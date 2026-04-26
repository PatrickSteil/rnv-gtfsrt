// Package rnvclient provides a typed GraphQL client for the RNV Datendrehscheibe API.
// It handles OAuth2 token acquisition and renewal transparently, and exposes
// higher-level methods such as ActiveJourneys that manage pagination internally.
package rnvclient

import (
	"encoding/json"
	"fmt"
	"time"
)

// TokenResponse is the JSON body returned by the OAuth2 token endpoint.
type TokenResponse struct {
	AccessToken string      `json:"access_token"`
	ExpiresIn   json.Number `json:"expires_in"`
	TokenType   string      `json:"token_type"`
}

// graphqlRequest is the JSON body sent to the GraphQL endpoint.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlError is a single error object in a GraphQL error response.
type graphqlError struct {
	Message string `json:"message"`
}

// GeoPoint is a geographic coordinate returned by the RNV API.
type GeoPoint struct {
	Lat  float32 `json:"lat"`
	Lon  float32 `json:"long"`
	Hash string  `json:"hash"`
}

// Station represents a transit stop or station in the RNV network.
type Station struct {
	Id        string   `json:"id"`
	HafasId   string   `json:"hafasID"`
	GlobalID  string   `json:"globalID"`
	ShortName string   `json:"shortName"`
	LongName  string   `json:"longName"`
	Location  GeoPoint `json:"geopoint"`
}

// Time is a timestamp as returned by the RNV API. It carries both a Unix
// epoch value (X) and an ISO 8601 string (IsoString); use GoTime to obtain
// a standard time.Time value.
type Time struct {
	IsoString string `json:"isoString"`
	// X is the number of seconds since 1970-01-01T00:00:00 UTC.
	X      int64 `json:"X"`
	OffSet int   `json:"offSet"`
}

// GoTime converts the RNV timestamp to a standard time.Time. It prefers the
// Unix epoch field X (with the UTC offset applied) over IsoString. Returns a
// zero time.Time and nil error if both fields are empty.
func (t *Time) GoTime() (time.Time, error) {
	if t == nil || (t.X == 0 && t.IsoString == "") {
		return time.Time{}, nil
	}

	if t.X != 0 {
		zone := time.FixedZone("Local", t.OffSet*60)
		return time.Unix(t.X, 0).In(zone), nil
	}

	if t.IsoString != "" {
		parsed, err := time.Parse(time.RFC3339, t.IsoString)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse isoString %q: %w", t.IsoString, err)
		}
		return parsed, nil
	}

	return time.Time{}, fmt.Errorf("time object contains no valid X or IsoString data")
}

// SearchResult is the paginated wrapper returned by list queries.
type SearchResult struct {
	TotalCount int       `json:"totalCount"`
	Cursor     string    `json:"cursor"`
	Elements   []Element `json:"elements"`
}

// Element represents a single journey returned by the RNV API.
type Element struct {
	ID                string   `json:"id"`
	LoadsForecastType string   `json:"loadsForecastType"`
	Loads             []Load   `json:"loads"`
	Stops             []Stop   `json:"stops"`
	Vehicles          []string `json:"vehicles"`
	Line              *Line    `json:"line"`
	// Canceled and Cancelled are both present because the API has used both
	// spellings historically. Check either field.
	Canceled  bool `json:"canceled"`
	Cancelled bool `json:"cancelled"`
}

// Load describes the occupancy at a specific station for a journey.
// Realtime is nil when no realtime data is available for that stop.
type Load struct {
	Forecast float64  `json:"forecast"`
	Realtime *float64 `json:"realtime"` // nil when no realtime data is available
	Adjusted float64  `json:"adjusted"`
	// LoadType is the coarse occupancy band reported by the API: "NA", "I",
	// "II", or "III". See MapLoadTypeToOccupancy for the mapping to GTFS-RT.
	LoadType string  `json:"loadType"`
	Ratio    float64 `json:"ratio"`
	Station  Station `json:"station"`
}

// Stop represents a single scheduled stop within a journey, including both
// planned and realtime departure/arrival times. Any of the time pointers may
// be nil if that data was not provided by the API.
type Stop struct {
	Station           Station `json:"station"`
	PlannedDeparture  *Time   `json:"plannedDeparture"`
	PlannedArrival    *Time   `json:"plannedArrival"`
	RealtimeDeparture *Time   `json:"realtimeDeparture"`
	RealtimeArrival   *Time   `json:"realtimeArrival"`
}

// Line holds the route identifier for a journey.
type Line struct {
	ID string `json:"id"`
}

type StationsData struct {
	Stations SearchResult `json:"stations"`
}

type StationJourneysData struct {
	Station Station `json:"station"`
}

type ActiveJourneysData struct {
	Journeys SearchResult `json:"journeys"`
}
