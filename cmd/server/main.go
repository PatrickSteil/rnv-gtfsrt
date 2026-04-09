// Command server polls the RNV Datendrehscheibe for occupancy data and serves
// it as a GTFS-Realtime feed.
//
// Usage:
//
//	export RNV_OAUTH_URL=https://login.microsoftonline.com/<tenantID>/oauth2/token
//	export RNV_CLIENT_ID=<your-client-id>
//	export RNV_CLIENT_SECRET=<your-client-secret>
//	export RNV_RESOURCE_ID=<your-resource-id>
//	export RNV_API_URL=https://graphql-sandbox-dds.rnv-online.de/
//	go run ./cmd/server
//
// The GTFS-RT protobuf feed is then available at http://localhost:8080/gtfs-rt
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PatrickSteil/rnv-gtfsrt/internal/config"
	"github.com/PatrickSteil/rnv-gtfsrt/internal/poller"
	"github.com/PatrickSteil/rnv-gtfsrt/internal/rnvclient"
	"github.com/PatrickSteil/rnv-gtfsrt/internal/server"
)

func main() {
	debug := flag.Bool("debug", false, "enable verbose debug logging")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	slog.Info("starting rnv-gtfsrt",
		"api_url", cfg.ClientAPIURL,
		"poll_interval", cfg.PollInterval,
		"listen", cfg.ListenAddr,
	)

	apiClient := rnvclient.New(
		cfg.OAuthURL,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.ResourceID,
		cfg.ClientAPIURL,
	)

	poll := poller.New(apiClient)
	srv := server.New(poll)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go poll.Run(ctx, cfg.PollInterval)

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "err", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	case <-ctx.Done():
		slog.Info("context cancelled, shutting down")
	}

	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := httpSrv.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}

	slog.Info("shutdown complete")
}
