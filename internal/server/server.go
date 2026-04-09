package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PatrickSteil/rnv-gtfsrt/internal/poller"
)

type Server struct {
	p   *poller.Poller
	mux *http.ServeMux
}

func New(p *poller.Poller) *Server {
	s := &Server{p: p, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/gtfs-rt", s.handleFeed)
	s.mux.HandleFunc("/data", s.handleData)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("serving /health")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	slog.Debug("serving /gtfs-rt")
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

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	slog.Debug("serving /data")
	snapshots := s.p.RawData()
	if len(snapshots) == 0 {
		http.Error(w, "data not yet available, please retry", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	if len(snapshots) == 0 {
		http.Error(w, "no journeys match the given filter", http.StatusNotFound)
		return
	}

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

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	slog.Debug("serving /status")
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
