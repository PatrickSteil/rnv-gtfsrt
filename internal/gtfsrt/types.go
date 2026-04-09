// Package gtfsrt implements the GTFS-Realtime FeedMessage format as plain Go
// structs.  Rather than requiring a protoc code-generation step, we define
// the proto3-compatible types manually and serialise them using the
// google.golang.org/protobuf/encoding/protojson and protowire packages.
//
// The occupancy extension follows the official gtfs-realtime.proto:
//
//	https://github.com/google/transit/blob/master/gtfs-realtime/proto/gtfs-realtime.proto
//
// We implement the minimal subset needed to publish OccupancyStatus:
//
//	FeedMessage
//	  FeedHeader
//	  FeedEntity[]
//	    TripUpdate
//	      TripDescriptor
//	      StopTimeUpdate[]
//	        StopTimeEvent (arrival/departure)
//	        OccupancyStatus
package gtfsrt

import "time"

// -----------------------------------------------------------------------
// Enum types
// -----------------------------------------------------------------------

// Incrementality mirrors the proto enum FeedHeader.Incrementality.
type Incrementality int32

const (
	FullDataset  Incrementality = 0
	Differential Incrementality = 1
)

// OccupancyStatus mirrors the proto enum OccupancyStatus.
type OccupancyStatus int32

const (
	OccupancyEmpty                  OccupancyStatus = 0
	OccupancyManySeatsAvailable     OccupancyStatus = 1
	OccupancyFewSeatsAvailable      OccupancyStatus = 2
	OccupancyStandingRoomOnly       OccupancyStatus = 3
	OccupancyCrushedStandingRoom    OccupancyStatus = 4
	OccupancyFull                   OccupancyStatus = 5
	OccupancyNotAcceptingPassengers OccupancyStatus = 6
	OccupancyNoDataAvailable        OccupancyStatus = 7
)

// ScheduleRelationship mirrors proto enum TripDescriptor.ScheduleRelationship.
type ScheduleRelationship int32

const (
	Scheduled   ScheduleRelationship = 0
	Added       ScheduleRelationship = 1
	Unscheduled ScheduleRelationship = 2
	Canceled    ScheduleRelationship = 3
)

// VehicleStopStatus mirrors the proto enum VehiclePosition.VehicleStopStatus.
type VehicleStopStatus int32

const (
	IncomingAt  VehicleStopStatus = 0
	StoppedAt   VehicleStopStatus = 1
	InTransitTo VehicleStopStatus = 2
)

// -----------------------------------------------------------------------
// Message types
// -----------------------------------------------------------------------

// FeedMessage is the top-level GTFS-RT message.
type FeedMessage struct {
	Header   FeedHeader
	Entities []FeedEntity
}

// FeedHeader contains feed metadata.
type FeedHeader struct {
	GtfsRealtimeVersion string
	Incrementality      Incrementality
	Timestamp           uint64
}

// FeedEntity wraps a single realtime update.
type FeedEntity struct {
	ID              string
	IsDeleted       bool
	VehiclePosition *VehiclePosition
}

// VehiclePosition carries realtime position/status for a vehicle.
type VehiclePosition struct {
	Trip                TripDescriptor
	Vehicle             *VehicleDescriptor
	CurrentStopSequence *uint32
	StopID              string
	CurrentStatus       *VehicleStopStatus // INCOMING_AT, STOPPED_AT, IN_TRANSIT_TO
	OccupancyStatus     *OccupancyStatus
}

// VehicleDescriptor identifies a vehicle.
type VehicleDescriptor struct {
	ID string
}

// TripDescriptor identifies a GTFS trip.
type TripDescriptor struct {
	TripID               string
	RouteID              string
	DirectionID          *uint32
	StartTime            string
	StartDate            string
	ScheduleRelationship ScheduleRelationship
}

// -----------------------------------------------------------------------
// Helper constructors
// -----------------------------------------------------------------------

// NewFeedMessage initialises a FeedMessage with a correct header timestamp.
func NewFeedMessage() *FeedMessage {
	ts := uint64(time.Now().Unix())
	return &FeedMessage{
		Header: FeedHeader{
			GtfsRealtimeVersion: "2.0",
			Incrementality:      FullDataset,
			Timestamp:           ts,
		},
	}
}

// MapLoadTypeToOccupancy converts an RNV loadType string to a GTFS-RT OccupancyStatus.
//
// RNV loadType levels:
//
//	NA  – no data
//	I   – empty / few passengers
//	II  – medium / many seats available
//	III – full / standing room only
func MapLoadTypeToOccupancy(loadType string, ratio float64) OccupancyStatus {
	switch loadType {
	case "I":
		if ratio < 0.25 {
			return OccupancyEmpty
		}
		return OccupancyManySeatsAvailable
	case "II":
		if ratio < 0.6 {
			return OccupancyFewSeatsAvailable
		}
		return OccupancyStandingRoomOnly
	case "III":
		if ratio >= 0.9 {
			return OccupancyFull
		}
		return OccupancyCrushedStandingRoom
	default:
		return OccupancyNoDataAvailable
	}
}
