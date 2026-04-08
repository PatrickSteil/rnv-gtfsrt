// Package poller orchestrates periodic fetching of occupancy data from the
// RNV API and converts it into a GTFS-RT FeedMessage.
package poller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/PatrickSteil/rnv-gtfsrt/internal/gtfsrt"
	"github.com/PatrickSteil/rnv-gtfsrt/internal/rnvclient"
)

// How far before/after "now" to consider a journey as currently active.
// Back: catch trips that started slightly before the poll tick.
// Forward: a small buffer for clock skew, but NOT future trips.
const (
	windowBack    = 5 * time.Minute
	windowForward = 10 * time.Minute
)

// JourneySnapshot is one active journey as returned by the API, stored for
// the /data JSON endpoint.
type JourneySnapshot struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Journey   rnvclient.Element `json:"journey"`
}

// Poller manages the polling lifecycle and exposes the latest FeedMessage.
type Poller struct {
	client   *rnvclient.Client
	pageSize int

	mu       sync.RWMutex
	feed     []byte            // latest serialised GTFS-RT feed
	feedTime time.Time         // when the feed was last built
	rawData  []JourneySnapshot // latest raw API data for /data endpoint
}

// New creates a new Poller.
func New(client *rnvclient.Client, pageSize int, _ []string) *Poller {
	// stationFilter parameter kept for API compatibility but no longer needed:
	// we query all active journeys globally in one call.
	return &Poller{
		client:   client,
		pageSize: pageSize,
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context, interval time.Duration) {
	slog.InfoContext(ctx, "poller starting",
		"interval", interval,
		"strategy", "global-active-journeys",
		"window_back", windowBack,
		"window_forward", windowForward,
	)

	if err := p.poll(ctx); err != nil {
		slog.ErrorContext(ctx, "initial poll failed", "err", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "poller stopping")
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				slog.ErrorContext(ctx, "poll failed", "err", err)
			}
		}
	}
}

// FeedBytes returns the latest serialised GTFS-RT protobuf feed and its age.
func (p *Poller) FeedBytes() ([]byte, time.Time) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.feed, p.feedTime
}

// RawData returns the latest journey snapshots from the RNV API.
func (p *Poller) RawData() []JourneySnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]JourneySnapshot, len(p.rawData))
	copy(out, p.rawData)
	return out
}

// -----------------------------------------------------------------------
// Internal
// -----------------------------------------------------------------------

func (p *Poller) poll(ctx context.Context) error {
	start := time.Now()
	slog.InfoContext(ctx, "poll started")

	journeys, err := p.client.ActiveJourneys(ctx, start, windowBack, windowForward, p.pageSize)
	if err != nil {
		return fmt.Errorf("fetching active journeys: %w", err)
	}

	msg := gtfsrt.NewFeedMessage()
	snapshots := make([]JourneySnapshot, 0, len(journeys))
	fetchedAt := time.Now()

	for _, j := range journeys {
		entity, ok := buildEntity(j)
		if !ok {
			continue
		}
		msg.Entities = append(msg.Entities, entity)
		snapshots = append(snapshots, JourneySnapshot{
			FetchedAt: fetchedAt,
			Journey:   j,
		})
	}

	slog.InfoContext(ctx, "poll finished",
		"active_journeys", len(journeys),
		"gtfsrt_entities", len(msg.Entities),
		"duration", time.Since(start).Round(time.Millisecond),
	)

	encoded := msg.MarshalBinary()

	p.mu.Lock()
	p.feed = encoded
	p.feedTime = fetchedAt
	p.rawData = snapshots
	p.mu.Unlock()

	return nil
}

// buildEntity converts a single RNV Journey element into a GTFS-RT FeedEntity
// with a VehiclePosition message.
//
// current_stop_sequence and stop_id are derived by iterating the journey's
// stops and finding the first stop whose planned departure is in the future
// relative to now (i.e. the vehicle has not yet departed that stop).
// If all stops have already passed we use the last stop.
func buildEntity(j rnvclient.Element) (gtfsrt.FeedEntity, bool) {
	if j.ID == "" {
		return gtfsrt.FeedEntity{}, false
	}

	// No load data at all → skip, nothing useful to emit.
	if len(j.Loads) == 0 {
		return gtfsrt.FeedEntity{}, false
	}

	// Pick the best load entry: prefer one with realtime data.
	load := bestLoad(j.Loads)
	occ := gtfsrt.MapLoadTypeToOccupancy(load.LoadType, load.Ratio)

	routeID := ""
	if j.Line != nil {
		routeID = j.Line.ID
	}

	schedRel := gtfsrt.Scheduled
	if j.Canceled || j.Cancelled {
		schedRel = gtfsrt.Canceled
	}

	// Derive start_time and start_date from the plannedDeparture of the first stop.
	var startTime, startDate string
	if len(j.Stops) > 0 && j.Stops[0].PlannedDeparture != nil {
		cet, _ := time.LoadLocation("Europe/Berlin")
		if t, err := j.Stops[0].PlannedDeparture.GoTime(); err == nil {
			local := t.In(cet)
			startTime = local.Format("15:04:05")
			startDate = local.Format("20060102")
		}
	}

	td := gtfsrt.TripDescriptor{
		// No trip_id available from the RNV API.
		RouteID:              routeID,
		StartTime:            startTime,
		StartDate:            startDate,
		ScheduleRelationship: schedRel,
	}

	// Determine current stop: iterate stops and find the first whose
	// planned departure (or arrival) is still in the future.
	now := time.Now()
	currentSeq, currentStopID := currentStop(j.Stops, now)

	// VehicleDescriptor: use the first vehicle ID if available.
	var vehicleDesc *gtfsrt.VehicleDescriptor
	if len(j.Vehicles) > 0 && j.Vehicles[0] != "" {
		vehicleDesc = &gtfsrt.VehicleDescriptor{ID: j.Vehicles[0]}
	}

	seqVal := uint32(currentSeq)
	vp := &gtfsrt.VehiclePosition{
		Trip:                td,
		Vehicle:             vehicleDesc,
		CurrentStopSequence: &seqVal,
		StopID:              currentStopID,
		OccupancyStatus:     &occ,
	}

	return gtfsrt.FeedEntity{
		ID:              fmt.Sprintf("rnv-%s", j.ID),
		VehiclePosition: vp,
	}, true
}

// currentStop returns the 1-based stop sequence index and stop ID of the
// current stop for the vehicle at time now.
//
// We iterate the stops in order. The "current" stop is the first stop where
// the vehicle has not yet departed, i.e. plannedDeparture > now (falling back
// to plannedArrival if departure is absent). If the vehicle has passed all
// stops, the last stop is returned.
func currentStop(stops []rnvclient.Stop, now time.Time) (seq int, stopID string) {
	if len(stops) == 0 {
		return 1, ""
	}
	for i, s := range stops {
		var ref *rnvclient.Time
		switch {
		case s.RealtimeDeparture != nil && s.RealtimeDeparture.IsoString != "":
			ref = s.RealtimeDeparture
		case s.PlannedDeparture != nil && s.PlannedDeparture.IsoString != "":
			ref = s.PlannedDeparture
		case s.RealtimeArrival != nil && s.RealtimeArrival.IsoString != "":
			ref = s.RealtimeArrival
		case s.PlannedArrival != nil && s.PlannedArrival.IsoString != "":
			ref = s.PlannedArrival
		}
		if ref != nil {
			if t, err := ref.GoTime(); err == nil && t.After(now) {
				return i + 1, s.Station.Id
			}
		}
	}
	// All stops passed – return last stop.
	last := stops[len(stops)-1]
	return len(stops), last.Station.Id
}

// bestLoad picks the most informative load entry from a slice:
// prefers realtime over statistical, highest ratio breaks ties.
func bestLoad(loads []rnvclient.Load) rnvclient.Load {
	best := loads[0]
	for _, l := range loads[1:] {
		// Prefer entries that have realtime data.
		bestHasRT := best.Realtime != nil
		lHasRT := l.Realtime != nil
		if lHasRT && !bestHasRT {
			best = l
			continue
		}
		// Among equal realtime availability, pick higher ratio (more informative).
		if lHasRT == bestHasRT && l.Ratio > best.Ratio {
			best = l
		}
	}
	return best
}
