// Package server provides the HTTP server that exposes the GTFS-RT feed and
// supporting endpoints. All handlers are read-only and stateless; they delegate
// to the Poller for the current data.
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PatrickSteil/rnv-gtfsrt/internal/poller"
)

// Server wraps a Poller and exposes its data over HTTP.
type Server struct {
	p   *poller.Poller
	mux *http.ServeMux
}

// New creates a Server backed by the given Poller and registers all routes.
func New(p *poller.Poller) *Server {
	s := &Server{p: p, mux: http.NewServeMux()}
	s.routes()
	return s
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{w, http.StatusOK}

		next.ServeHTTP(wrapped, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("/gtfs-rt", s.handleFeed)
	s.mux.HandleFunc("/data", s.handleData)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
}

// Handler returns the root HTTP handler with logging middleware applied.
// Pass this to http.Server.Handler.
func (s *Server) Handler() http.Handler {
	return loggingMiddleware(s.mux)
}

// handleFeed serves the GTFS-RT protobuf feed at GET /gtfs-rt.
// Returns 503 if no poll has succeeded yet.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	feed, feedTime := s.p.FeedBytes()
	if feed == nil {
		slog.Debug("feed not yet available")
		http.Error(w, "feed not yet available, please retry", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("X-Feed-Timestamp", feedTime.UTC().Format(time.RFC3339))
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(feed); err != nil {
		slog.Error("writing feed response", "err", err)
	}
}

// handleData serves the raw journey snapshots as JSON at GET /data.
// Accepts an optional ?pretty query parameter for indented output.
// Returns 503 if no poll has succeeded yet.
func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	snapshots := s.p.RawData()
	if len(snapshots) == 0 {
		slog.Debug("data not yet available")
		http.Error(w, "data not yet available, please retry", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	_, feedTime := s.p.FeedBytes()
	response := map[string]any{
		"feed_timestamp": feedTime.UTC().Format(time.RFC3339),
		"journey_count":  len(snapshots),
		"journeys":       snapshots,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	if q.Get("pretty") != "" {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(response); err != nil {
		slog.Error("writing data response", "err", err)
	}
}

// handleStatus serves a JSON summary of the feed state at GET /status,
// including feed age, entity count, and a breakdown of load types seen.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	feed, feedTime := s.p.FeedBytes()
	snapshots := s.p.RawData()

	loadTypeCounts := map[string]int{}
	for _, snap := range snapshots {
		for _, l := range snap.Journey.Loads {
			loadTypeCounts[l.LoadType]++
		}
	}

	status := map[string]any{
		"feed_available": feed != nil,
		"feed_bytes":     len(feed),
		"feed_age_seconds": func() float64 {
			if feed == nil {
				return -1
			}
			return time.Since(feedTime).Seconds()
		}(),
		"feed_timestamp":   feedTime.UTC().Format(time.RFC3339),
		"active_journeys":  len(snapshots),
		"load_type_counts": loadTypeCounts,
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(status)
}
