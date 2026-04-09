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
func New(client *rnvclient.Client) *Poller {
	return &Poller{
		client: client,
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

// incomingAtThreshold is how close to a future arrival the vehicle must be
// to be considered INCOMING_AT rather than IN_TRANSIT_TO.
const incomingAtThreshold = 30 * time.Second

// StopMatch holds the result of the current-stop search.
type StopMatch struct {
	Seq    int    // 1-based stop sequence index
	StopID string // station GlobalID
	Status gtfsrt.VehicleStopStatus
}

func currentStop(stops []rnvclient.Stop, now time.Time) StopMatch {
	if len(stops) == 0 {
		return StopMatch{Seq: 1, Status: gtfsrt.InTransitTo}
	}

	bestIdx := -1
	bestDelta := time.Duration(1<<63 - 1)

	for i, s := range stops {
		refs := []*rnvclient.Time{
			s.RealtimeDeparture,
			s.PlannedDeparture,
			s.RealtimeArrival,
			s.PlannedArrival,
		}
		for _, ref := range refs {
			if ref == nil || ref.IsoString == "" {
				continue
			}
			t, err := ref.GoTime()
			if err != nil {
				continue
			}
			delta := t.Sub(now)
			if delta < 0 {
				delta = -delta
			}
			if delta < bestDelta {
				bestDelta = delta
				bestIdx = i
			}
		}
	}

	if bestIdx < 0 {
		return StopMatch{Seq: 1, StopID: stops[0].Station.GlobalID, Status: gtfsrt.InTransitTo}
	}

	s := stops[bestIdx]
	status := vehicleStopStatus(s, now)

	return StopMatch{
		Seq:    bestIdx + 1,
		StopID: s.Station.GlobalID,
		Status: status,
	}
}

// vehicleStopStatus derives the VehicleStopStatus for a single stop at time now.
//
// Priority of time references: realtime over planned, departure over arrival.
// The dwell window is defined as: arrival has passed AND departure has not.
func vehicleStopStatus(s rnvclient.Stop, now time.Time) gtfsrt.VehicleStopStatus {
	arrivalRef := firstValidTime(s.RealtimeArrival, s.PlannedArrival)
	departureRef := firstValidTime(s.RealtimeDeparture, s.PlannedDeparture)

	arrivalTime, hasArrival := resolveTime(arrivalRef)
	departureTime, hasDeparture := resolveTime(departureRef)

	switch {
	case hasArrival && hasDeparture:
		arrivalPassed := now.After(arrivalTime)
		departurePassed := now.After(departureTime)
		switch {
		case arrivalPassed && !departurePassed:
			// Inside the dwell window.
			return gtfsrt.StoppedAt
		case !arrivalPassed && departureTime.Sub(now) <= incomingAtThreshold:
			// Close enough to call it incoming (arrival is imminent).
			// This handles the edge case where arrival == departure (pass-through stop).
			return gtfsrt.IncomingAt
		case !arrivalPassed:
			return gtfsrt.InTransitTo
		default:
			// Both passed — vehicle has departed.
			return gtfsrt.InTransitTo
		}

	case hasDeparture && !hasArrival:
		// No arrival data — use departure as the sole reference.
		timeUntilDep := departureTime.Sub(now)
		switch {
		case timeUntilDep > 0 && timeUntilDep <= incomingAtThreshold:
			return gtfsrt.IncomingAt
		case timeUntilDep > 0:
			return gtfsrt.InTransitTo
		default:
			// Departure passed, no arrival data — assume departed.
			return gtfsrt.InTransitTo
		}

	case hasArrival && !hasDeparture:
		// No departure data — use arrival as the sole reference.
		if now.After(arrivalTime) {
			return gtfsrt.StoppedAt
		}
		if arrivalTime.Sub(now) <= incomingAtThreshold {
			return gtfsrt.IncomingAt
		}
		return gtfsrt.InTransitTo

	default:
		// No usable timestamps for this stop.
		return gtfsrt.InTransitTo
	}
}

// firstValidTime returns the first non-nil, non-empty Time from the arguments.
func firstValidTime(refs ...*rnvclient.Time) *rnvclient.Time {
	for _, r := range refs {
		if r != nil && r.IsoString != "" {
			return r
		}
	}
	return nil
}

// resolveTime parses a *rnvclient.Time and reports whether it succeeded.
func resolveTime(ref *rnvclient.Time) (time.Time, bool) {
	if ref == nil {
		return time.Time{}, false
	}
	t, err := ref.GoTime()
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func loadAtStop(loads []rnvclient.Load, stopID string) rnvclient.Load {
	if stopID != "" {
		for _, l := range loads {
			if l.Station.GlobalID == stopID { // ← adjust field name if needed
				return l
			}
		}
	}
	return bestLoad(loads)
}

// buildEntity converts a single RNV Journey element into a GTFS-RT FeedEntity
// with a VehiclePosition message.
//
// The occupancy status is derived from the load data at the current stop.
// "Current stop" is the stop the vehicle is temporally closest to — either
// the stop it is currently at, or the nearer of the two stops surrounding
// its current position. If no stop-specific load entry exists the best
// available load (preferring realtime) is used instead.
func buildEntity(j rnvclient.Element) (gtfsrt.FeedEntity, bool) {
	if j.ID == "" {
		return gtfsrt.FeedEntity{}, false
	}

	if len(j.Loads) == 0 {
		return gtfsrt.FeedEntity{}, false
	}

	// Find the current stop first so we can look up the matching load.
	now := time.Now()
	match := currentStop(j.Stops, now)

	// Load at the current stop — falls back to bestLoad if no match.
	load := loadAtStop(j.Loads, match.StopID)
	occ := gtfsrt.MapLoadTypeToOccupancy(load.LoadType, load.Ratio)

	routeID := ""
	if j.Line != nil {
		routeID = j.Line.ID
	}

	schedRel := gtfsrt.Scheduled
	if j.Canceled || j.Cancelled {
		schedRel = gtfsrt.Canceled
	}

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
		RouteID:              routeID,
		StartTime:            startTime,
		StartDate:            startDate,
		ScheduleRelationship: schedRel,
	}

	var vehicleDesc *gtfsrt.VehicleDescriptor
	if len(j.Vehicles) > 0 && j.Vehicles[0] != "" {
		vehicleDesc = &gtfsrt.VehicleDescriptor{ID: j.Vehicles[0]}
	}

	seqVal := uint32(match.Seq)
	vp := &gtfsrt.VehiclePosition{
		Trip:                td,
		Vehicle:             vehicleDesc,
		CurrentStopSequence: &seqVal,
		StopID:              match.StopID,
		CurrentStatus:       &match.Status,
		OccupancyStatus:     &occ,
	}

	return gtfsrt.FeedEntity{
		ID:              fmt.Sprintf("rnv-%s", j.ID),
		VehiclePosition: vp,
	}, true
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
