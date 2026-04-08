package gtfsrt

// encode.go – hand-written protobuf encoding for GTFS-RT FeedMessage.
//
// Field numbers follow the official gtfs-realtime.proto spec:
//   https://github.com/google/transit/blob/master/gtfs-realtime/proto/gtfs-realtime.proto
//
// We use google.golang.org/protobuf/encoding/protowire for low-level varint/
// length-delimited encoding so we don't depend on a generated pb.go file.

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// MarshalBinary serialises a FeedMessage to protobuf binary format.
func (fm *FeedMessage) MarshalBinary() []byte {
	var b []byte

	// field 1: FeedHeader (message)
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendBytes(b, marshalHeader(fm.Header))

	// field 2: FeedEntity[] (repeated message)
	for _, e := range fm.Entities {
		b = protowire.AppendTag(b, 2, protowire.BytesType)
		b = protowire.AppendBytes(b, marshalEntity(e))
	}
	return b
}

// -----------------------------------------------------------------------
// FeedHeader  (proto field numbers per spec)
//   1: gtfs_realtime_version (string)
//   2: incrementality        (enum/int32)
//   3: timestamp             (uint64)
// -----------------------------------------------------------------------

func marshalHeader(h FeedHeader) []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, h.GtfsRealtimeVersion)
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(h.Incrementality))
	b = protowire.AppendTag(b, 3, protowire.VarintType)
	b = protowire.AppendVarint(b, h.Timestamp)
	return b
}

// -----------------------------------------------------------------------
// FeedEntity
//   1: id               (string)
//   2: is_deleted       (bool)
//   3: trip_update      (message)
//   4: vehicle_position (message)
// -----------------------------------------------------------------------

func marshalEntity(e FeedEntity) []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, e.ID)
	if e.IsDeleted {
		b = protowire.AppendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	if e.VehiclePosition != nil {
		b = protowire.AppendTag(b, 4, protowire.BytesType)
		b = protowire.AppendBytes(b, marshalVehiclePosition(e.VehiclePosition))
	}
	return b
}

// -----------------------------------------------------------------------
// VehiclePosition
//   1: trip
//   2: position
//   3: current_stop_sequence
//   4: stop_id
//   5: current_status
//   6: timestamp
//   7: congestion_level
//   8: vehicle
//   9: occupancy_status
// -----------------------------------------------------------------------

func marshalVehiclePosition(vp *VehiclePosition) []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendBytes(b, marshalTripDescriptor(vp.Trip))
	if vp.CurrentStopSequence != nil {
		b = protowire.AppendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(*vp.CurrentStopSequence))
	}
	if vp.StopID != "" {
		b = protowire.AppendTag(b, 7, protowire.BytesType)
		b = protowire.AppendString(b, vp.StopID)
	}
	if vp.Vehicle != nil {
		b = protowire.AppendTag(b, 8, protowire.BytesType)
		b = protowire.AppendBytes(b, marshalVehicleDescriptor(vp.Vehicle))
	}
	if vp.OccupancyStatus != nil {
		b = protowire.AppendTag(b, 9, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(*vp.OccupancyStatus))
	}
	return b
}

// -----------------------------------------------------------------------
// VehicleDescriptor
//   1: id    (string)
//   2: label (string) – omitted
//   3: license_plate (string) – omitted
// -----------------------------------------------------------------------

func marshalVehicleDescriptor(vd *VehicleDescriptor) []byte {
	var b []byte
	if vd.ID != "" {
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, vd.ID)
	}
	return b
}

// -----------------------------------------------------------------------
// TripDescriptor
//   1: trip_id                  (string)
//   2: route_id                 (string)
//   3: direction_id             (uint32)
//   4: start_time               (string)
//   5: start_date               (string)
//   6: schedule_relationship    (enum)
// -----------------------------------------------------------------------

func marshalTripDescriptor(td TripDescriptor) []byte {
	var b []byte
	if td.TripID != "" {
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, td.TripID)
	}
	if td.StartTime != "" {
		b = protowire.AppendTag(b, 2, protowire.BytesType) // ← was 4
		b = protowire.AppendString(b, td.StartTime)
	}
	if td.StartDate != "" {
		b = protowire.AppendTag(b, 3, protowire.BytesType) // ← was 5
		b = protowire.AppendString(b, td.StartDate)
	}
	if td.ScheduleRelationship != 0 {
		b = protowire.AppendTag(b, 4, protowire.VarintType) // ← was 6
		b = protowire.AppendVarint(b, uint64(td.ScheduleRelationship))
	}
	if td.RouteID != "" {
		b = protowire.AppendTag(b, 5, protowire.BytesType) // ← was 2
		b = protowire.AppendString(b, td.RouteID)
	}
	if td.DirectionID != nil {
		b = protowire.AppendTag(b, 6, protowire.VarintType) // ← was 3
		b = protowire.AppendVarint(b, uint64(*td.DirectionID))
	}
	return b
}
