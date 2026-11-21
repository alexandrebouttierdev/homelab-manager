package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"homelab-manager/backend/internal/adapters/postgres"
	httpdelivery "homelab-manager/backend/internal/delivery/http"
	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

func main() {
	config, err := LoadConfig()
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

	if err := postgres.CheckConnection(context.Background(), config.DatabaseURL); err != nil {
		logger.Warn("PostgreSQL is unavailable; the HTTP health service can still start", zap.Error(err))
	}

	healthUsecase := usecase.NewHealthUsecase(config.StartedAt)
	router := httpdelivery.NewRouter(logger, healthUsecase)

	server := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           router,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("Homelab Manager listening", zap.String("address", config.HTTPAddress))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server stopped", zap.Error(err))
			stop <- os.Interrupt
		}
	}()

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("HTTP shutdown failed", zap.Error(err))
	}
}
