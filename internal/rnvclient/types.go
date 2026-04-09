// Package rnvclient provides a typed GraphQL client for the RNV Datendrehscheibe API.
package rnvclient

import (
	"encoding/json"
	"fmt"
	"time"
)

type TokenResponse struct {
	AccessToken string      `json:"access_token"`
	ExpiresIn   json.Number `json:"expires_in"`
	TokenType   string      `json:"token_type"`
}

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

type Time struct {
	IsoString string `json:"isoString"`
	// Anzahl der Sekunden seit dem 1.1.1970T00:00:00 UTC+0
	X      int64 `json:"X"`
	OffSet int   `json:"offSet"`
}

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

type SearchResult struct {
	TotalCount int       `json:"totalCount"`
	Cursor     string    `json:"cursor"`
	Elements   []Element `json:"elements"`
}

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

type Load struct {
	Forecast float64  `json:"forecast"`
	Realtime *float64 `json:"realtime"` // nullable
	Adjusted float64  `json:"adjusted"`
	LoadType string   `json:"loadType"` // NA | I | II | III
	Ratio    float64  `json:"ratio"`
	Station  Station  `json:"station"`
}

type Stop struct {
	Station           Station `json:"station"`
	PlannedDeparture  *Time   `json:"plannedDeparture"`
	PlannedArrival    *Time   `json:"plannedArrival"`
	RealtimeDeparture *Time   `json:"realtimeDeparture"`
	RealtimeArrival   *Time   `json:"realtimeArrival"`
}

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
