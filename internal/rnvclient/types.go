// Package rnvclient provides a typed GraphQL client for the RNV Datendrehscheibe API.
package rnvclient

import (
	"encoding/json"
	"time"
)

// TokenResponse is the JSON body returned by the OAuth2 token endpoint.
// Azure AD returns expires_in as a string (e.g. "3600"), not an int.
type TokenResponse struct {
	AccessToken string      `json:"access_token"`
	ExpiresIn   json.Number `json:"expires_in"`
	TokenType   string      `json:"token_type"`
}

// graphqlRequest is the POST body sent to the GraphQL endpoint.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlError struct {
	Message string `json:"message"`
}

type GeoPoint struct {
	Lat  float32 `json:"lat"`
	Lon  float32 `json:"long"`
	Hash string  `json:"hash"`
}

type Station struct {
	Id        string   `json:"id"`
	HafasId   string   `json:"hafasID"`
	GlobalID  string   `json:"globalID"`
	ShortName string   `json:"shortName"`
	LongName  string   `json:"longName"`
	Location  GeoPoint `json:"geopoint"`
}

// Time mirrors the GraphQL Time type.
type Time struct {
	IsoString string `json:"isoString"`
	X         int64  `json:"X"` // Unix seconds UTC
}

// GoTime parses the ISO-8601 string into a stdlib time.Time.
func (t *Time) GoTime() (time.Time, error) {
	if t == nil || t.IsoString == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, t.IsoString)
}

// SearchResult mirrors the GraphQL SearchResult type.
type SearchResult struct {
	TotalCount int       `json:"totalCount"`
	Cursor     string    `json:"cursor"`
	Elements   []Element `json:"elements"`
}

// Element is the union type returned inside SearchResult.
// Only the fields we care about are populated; GraphQL inline fragments
// are used to fill them selectively.
type Element struct {
	ID                string   `json:"id"`
	LoadsForecastType string   `json:"loadsForecastType"`
	Loads             []Load   `json:"loads"`
	Stops             []Stop   `json:"stops"`
	Vehicles          []string `json:"vehicles"`
	Line              *Line    `json:"line"`
	Canceled          bool     `json:"canceled"`
	Cancelled         bool     `json:"cancelled"`
}

// Load mirrors the GraphQL Load type.
type Load struct {
	Forecast float64  `json:"forecast"`
	Realtime *float64 `json:"realtime"` // nullable
	Adjusted float64  `json:"adjusted"`
	LoadType string   `json:"loadType"` // NA | I | II | III
	Ratio    float64  `json:"ratio"`
	Station  Station  `json:"station"`
}

// Stop mirrors the GraphQL Stop type.
type Stop struct {
	Station           Station `json:"station"`
	PlannedDeparture  *Time   `json:"plannedDeparture"`
	PlannedArrival    *Time   `json:"plannedArrival"`
	RealtimeDeparture *Time   `json:"realtimeDeparture"`
	RealtimeArrival   *Time   `json:"realtimeArrival"`
}

// Line mirrors the relevant fields of the GraphQL Line type.
type Line struct {
	ID string `json:"id"`
}

// -----------------------------------------------------------------------
// Query-specific response types
// -----------------------------------------------------------------------

// StationsData is the top-level data for a stations query.
type StationsData struct {
	Stations SearchResult `json:"stations"`
}

// StationJourneysData is the top-level data for a station + journeys query.
type StationJourneysData struct {
	Station Station `json:"station"`
}
