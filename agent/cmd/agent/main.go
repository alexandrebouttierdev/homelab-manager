package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"homelab-manager/agent/internal/collector"
	"homelab-manager/agent/internal/config"
	"homelab-manager/agent/internal/transport"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	managerURL, err := url.Parse(cfg.ManagerURL)
	if err != nil {
		logger.Fatal("invalid Manager URL", zap.Error(err))
	}
	managerURL.Path = "/api/v1/agent/stream"

	interval := time.Duration(cfg.CollectIntervalSeconds) * time.Second
	err = transport.Run(ctx, managerURL.String(), cfg.AgentToken, func(conn *websocket.Conn) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				snapshot, err := collector.Collect(ctx)
				if err != nil {
					logger.Warn("metric collection failed", zap.Error(err))
					continue
				}
				payload, err := json.Marshal(snapshot)
				if err != nil {
					logger.Warn("metric encoding failed", zap.Error(err))
					continue
				}
				if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
					return err
				}
			}
		}
	})
	if err != nil && ctx.Err() == nil {
		logger.Error("Agent transport stopped", zap.Error(err))
	}
}
