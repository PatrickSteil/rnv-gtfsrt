// cmd/inspect reads a GTFS-RT binary protobuf file produced by rnv-occupancy
// and prints a human-readable validation report to stdout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	gtfs "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

func main() {
	asJSON := flag.Bool("json", false, "output as JSON instead of text")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/inspect [flags] <file.pb>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	path := flag.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("reading %q: %v", path, err)
	}

	feed := &gtfs.FeedMessage{}
	if err := proto.Unmarshal(data, feed); err != nil {
		fatalf("unmarshalling protobuf: %v\n\nThe file may not be a valid GTFS-RT binary protobuf.", err)
	}

	if *asJSON {
		printJSON(feed)
	} else {
		printText(feed)
	}
}

func printText(feed *gtfs.FeedMessage) {
	h := feed.GetHeader()
	ts := time.Unix(int64(h.GetTimestamp()), 0)

	errs := validate(feed)

	fmt.Println(separator("=", 70))
	fmt.Println("  GTFS-RT Feed Inspection Report")
	fmt.Println(separator("=", 70))
	fmt.Printf("  File parsed:     OK  (%d bytes decoded)\n", proto.Size(feed))
	fmt.Printf("  Spec version:    %s\n", h.GetGtfsRealtimeVersion())
	fmt.Printf("  Incrementality:  %s\n", h.GetIncrementality())
	fmt.Printf("  Timestamp:       %d  (%s)\n", h.GetTimestamp(), ts.Format(time.RFC3339))
	fmt.Printf("  Entities:        %d\n", len(feed.Entity))
	if len(errs) == 0 {
		fmt.Println("  Validation:      PASS")
	} else {
		fmt.Printf("  Validation:      FAIL (%d issue(s))\n", len(errs))
	}
	fmt.Println(separator("-", 70))

	// Validation issues
	if len(errs) > 0 {
		fmt.Println("  VALIDATION ISSUES")
		for _, e := range errs {
			fmt.Printf("    [!] %s\n", e)
		}
		fmt.Println(separator("-", 70))
	}

	// Per-entity detail
	fmt.Println("  ENTITIES")
	fmt.Println()
	for i, e := range feed.Entity {
		fmt.Printf("  [%d] entity_id = %q\n", i+1, e.GetId())

		vp := e.GetVehicle()
		if vp == nil {
			fmt.Println("      (no vehicle_position – skipped)")
			continue
		}

		trip := vp.GetTrip()
		fmt.Printf("      trip_id            = %q\n", trip.GetTripId())
		fmt.Printf("      route_id           = %q\n", trip.GetRouteId())
		fmt.Printf("      start_date         = %q\n", trip.GetStartDate())
		fmt.Printf("      start_time         = %q\n", trip.GetStartTime())
		fmt.Printf("      current_stop_seq   = %d\n", vp.GetCurrentStopSequence())
		fmt.Printf("      occupancy_status   = %s (%d)\n",
			vp.GetOccupancyStatus().String(),
			vp.GetOccupancyStatus().Number())

		if vp.OccupancyPercentage != nil {
			fmt.Printf("      occupancy_pct      = %d%%\n", vp.GetOccupancyPercentage())
		}
		if vp.GetVehicle() != nil {
			fmt.Printf("      vehicle_label      = %q\n", vp.GetVehicle().GetLabel())
		}
		fmt.Println()
	}

	fmt.Println(separator("=", 70))

	// Summary table
	fmt.Println("  OCCUPANCY SUMMARY")
	fmt.Println()
	counts := map[string]int{}
	for _, e := range feed.Entity {
		vp := e.GetVehicle()
		if vp == nil {
			continue
		}
		counts[vp.GetOccupancyStatus().String()]++
	}
	for status, n := range counts {
		bar := strings.Repeat("█", min(n, 100))
		fmt.Printf("  %-35s %s (%d)\n", status, bar, n)
	}
	fmt.Println()
	fmt.Println(separator("=", 70))

	if len(errs) > 0 {
		os.Exit(2)
	}
}

type jsonEntity struct {
	EntityID         string  `json:"entity_id"`
	TripID           string  `json:"trip_id"`
	RouteID          string  `json:"route_id"`
	StartDate        string  `json:"start_date"`
	StartTime        string  `json:"start_time,omitempty"`
	CurrentStopSeq   uint32  `json:"current_stop_sequence"`
	OccupancyStatus  string  `json:"occupancy_status"`
	OccupancyStatusN int32   `json:"occupancy_status_number"`
	OccupancyPercent *uint32 `json:"occupancy_percentage,omitempty"`
	VehicleLabel     string  `json:"vehicle_label,omitempty"`
}

type jsonFeed struct {
	Header struct {
		Version        string `json:"gtfs_realtime_version"`
		Incrementality string `json:"incrementality"`
		Timestamp      uint64 `json:"timestamp"`
		TimestampISO   string `json:"timestamp_iso"`
	} `json:"header"`
	EntityCount      int          `json:"entity_count"`
	ValidationErrors []string     `json:"validation_errors"`
	Entities         []jsonEntity `json:"entities"`
}

func printJSON(feed *gtfs.FeedMessage) {
	h := feed.GetHeader()
	out := jsonFeed{}
	out.Header.Version = h.GetGtfsRealtimeVersion()
	out.Header.Incrementality = h.GetIncrementality().String()
	out.Header.Timestamp = h.GetTimestamp()
	out.Header.TimestampISO = time.Unix(int64(h.GetTimestamp()), 0).Format(time.RFC3339)
	out.EntityCount = len(feed.Entity)
	out.ValidationErrors = validate(feed)

	for _, e := range feed.Entity {
		vp := e.GetVehicle()
		if vp == nil {
			continue
		}
		trip := vp.GetTrip()
		je := jsonEntity{
			EntityID:         e.GetId(),
			TripID:           trip.GetTripId(),
			RouteID:          trip.GetRouteId(),
			StartDate:        trip.GetStartDate(),
			StartTime:        trip.GetStartTime(),
			CurrentStopSeq:   vp.GetCurrentStopSequence(),
			OccupancyStatus:  vp.GetOccupancyStatus().String(),
			OccupancyStatusN: int32(vp.GetOccupancyStatus().Number()),
		}
		if vp.OccupancyPercentage != nil {
			v := vp.GetOccupancyPercentage()
			je.OccupancyPercent = &v
		}
		if vp.GetVehicle() != nil {
			je.VehicleLabel = vp.GetVehicle().GetLabel()
		}
		out.Entities = append(out.Entities, je)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// validate checks required GTFS-RT fields per the spec and returns a list
// of human-readable error strings.
func validate(feed *gtfs.FeedMessage) []string {
	var errs []string

	h := feed.GetHeader()
	if h == nil {
		errs = append(errs, "header is missing")
		return errs
	}
	if h.GtfsRealtimeVersion == nil || h.GetGtfsRealtimeVersion() == "" {
		errs = append(errs, "header.gtfs_realtime_version is required")
	}
	if h.Timestamp == nil || h.GetTimestamp() == 0 {
		errs = append(errs, "header.timestamp is required")
	} else {
		age := time.Since(time.Unix(int64(h.GetTimestamp()), 0))
		if age > 5*time.Minute {
			errs = append(errs, fmt.Sprintf("header.timestamp is %.0f minutes old (>5 min – consumers may reject)", age.Minutes()))
		}
	}

	idsSeen := map[string]bool{}
	for i, e := range feed.Entity {
		prefix := fmt.Sprintf("entity[%d]", i)

		if e.Id == nil || e.GetId() == "" {
			errs = append(errs, prefix+": id is required")
		} else if idsSeen[e.GetId()] {
			errs = append(errs, fmt.Sprintf("%s: duplicate id %q", prefix, e.GetId()))
		}
		idsSeen[e.GetId()] = true

		// Must have exactly one of: trip_update, vehicle_position, alert
		hasVP := e.Vehicle != nil
		hasTU := e.TripUpdate != nil
		hasAl := e.Alert != nil
		if !hasVP && !hasTU && !hasAl {
			errs = append(errs, prefix+": must have vehicle_position, trip_update, or alert")
			continue
		}

		if hasVP {
			vp := e.GetVehicle()
			trip := vp.GetTrip()
			if trip == nil {
				errs = append(errs, prefix+".vehicle_position: trip descriptor is required")
			} else {
				if trip.GetTripId() == "" && trip.GetRouteId() == "" {
					errs = append(errs, prefix+".vehicle_position.trip: at least trip_id or route_id is required")
				}
			}
			// if vp.GetOccupancyStatus() == gtfs.VehiclePosition_EMPTY {
			// 	errs = append(errs, fmt.Sprintf("%s.vehicle_position: occupancy_status is EMPTY (0) – no data for this trip", prefix))
			// }
			if vp.OccupancyPercentage != nil {
				pct := vp.GetOccupancyPercentage()
				if pct > 100 {
					errs = append(errs, fmt.Sprintf("%s.vehicle_position: occupancy_percentage %d > 100", prefix, pct))
				}
			}
		}
	}

	return errs
}

func separator(ch string, n int) string {
	return "  " + strings.Repeat(ch, n)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
