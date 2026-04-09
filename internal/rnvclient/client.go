package rnvclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a thread-safe GraphQL client for the RNV API with integrated OAuth2.
type Client struct {
	oauthURL     string
	clientID     string
	clientSecret string
	resourceID   string
	apiURL       string

	httpClient *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// New creates a new Client.
func New(oauthURL, clientID, clientSecret, resourceID, apiURL string) *Client {
	return &Client{
		oauthURL:     oauthURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		resourceID:   resourceID,
		apiURL:       apiURL,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// -----------------------------------------------------------------------
// Token management
// -----------------------------------------------------------------------

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	slog.InfoContext(ctx, "refreshing OAuth2 access token")

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("resource", c.resourceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	c.accessToken = tr.AccessToken
	expiry := int64(3600)
	if n, err := tr.ExpiresIn.Int64(); err == nil && n > 0 {
		expiry = n
	}
	c.tokenExpiry = time.Now().Add(time.Duration(expiry-60) * time.Second)

	slog.InfoContext(ctx, "obtained access token", "expires_in", expiry)
	return c.accessToken, nil
}

// -----------------------------------------------------------------------
// GraphQL execution
// -----------------------------------------------------------------------

func (c *Client) query(ctx context.Context, gqlQuery string, variables map[string]any, dest any) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(graphqlRequest{Query: gqlQuery, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshaling query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing graphql query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graphql endpoint returned %d: %s", resp.StatusCode, body)
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("decoding graphql envelope: %w", err)
	}

	if errField, ok := raw["errors"]; ok {
		var gqlErrs []graphqlError
		_ = json.Unmarshal(errField, &gqlErrs)
		if len(gqlErrs) > 0 {
			msgs := make([]string, len(gqlErrs))
			for i, e := range gqlErrs {
				msgs[i] = e.Message
			}
			return fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
		}
	}

	dataField, ok := raw["data"]
	if !ok {
		return fmt.Errorf("graphql response missing 'data' field")
	}

	if err := json.Unmarshal(dataField, dest); err != nil {
		return fmt.Errorf("unmarshaling graphql data: %w", err)
	}
	return nil
}

// -----------------------------------------------------------------------
const activeJourneysQuery = `
query ActiveJourneys($startTime: String!, $endTime: String!, $after: String, $source: SourceType) {
  journeys(startTime: $startTime, endTime: $endTime, after: $after, source: $source) {
    totalCount
    cursor
    elements {
      ... on Journey {
        id
        canceled
        cancelled
        loadsForecastType
        line {
          id
        }
		loads {
          station {
			globalID
		  }
          forecast
          realtime
          adjusted
          ratio
          loadType
        }
		vehicles
		stops {
			station {
				globalID
			}
			plannedDeparture { isoString}
			plannedArrival { isoString }
			realtimeDeparture { isoString }
			realtimeArrival { isoString }
		}
      }
    }
  }
}`

// ActiveJourneysData is the top-level response for the active journeys query.
type ActiveJourneysData struct {
	Journeys SearchResult `json:"journeys"`
}

// ActiveJourneys fetches all currently-running journeys with their occupancy,
// paging through the full result set automatically.
//
// windowBack/windowForward control how far before/after now to look.
// For "only currently active" use windowBack=2min, windowForward=1min.
func (c *Client) ActiveJourneys(ctx context.Context, now time.Time, windowBack, windowForward time.Duration, pageSize int) ([]Element, error) {
	startTime := now.Add(-windowBack).UTC().Format(time.RFC3339)
	endTime := now.Add(windowForward).UTC().Format(time.RFC3339)

	var all []Element
	var cursor string
	page := 0

	for {
		vars := map[string]any{
			"startTime": startTime,
			"endTime":   endTime,
			"source":    "REALTIMEONLY",
		}
		if cursor != "" {
			vars["after"] = cursor
		}

		var data ActiveJourneysData
		if err := c.query(ctx, activeJourneysQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("fetching active journeys (page %d): %w", page, err)
		}

		all = append(all, data.Journeys.Elements...)
		page++

		slog.DebugContext(ctx, "active journeys page fetched",
			"page", page,
			"fetched", len(all),
			"total", data.Journeys.TotalCount,
			"cursor", data.Journeys.Cursor,
		)

		if data.Journeys.Cursor == "" || len(data.Journeys.Elements) == 0 {
			break
		}
		if len(all) >= data.Journeys.TotalCount {
			break
		}
		cursor = data.Journeys.Cursor
	}

	slog.InfoContext(ctx, "fetched active journeys",
		"count", len(all),
		"window_start", startTime,
		"window_end", endTime,
	)
	return all, nil
}
