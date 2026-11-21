package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	argon2adapter "homelab-manager/backend/internal/adapters/argon2"
	"homelab-manager/backend/internal/adapters/dockerengine"
	"homelab-manager/backend/internal/adapters/postgres"
	systemcollector "homelab-manager/backend/internal/collector/system"
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

	databaseErr := postgres.CheckConnection(context.Background(), config.DatabaseURL)
	databaseAvailable := databaseErr == nil
	if databaseAvailable {
		migrationCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		databaseErr = postgres.ApplyMigrations(migrationCtx, config.DatabaseURL)
		cancel()
		databaseAvailable = databaseErr == nil
	}
	if databaseErr != nil {
		logger.Warn("PostgreSQL is unavailable or migrations failed; the HTTP health service can still start", zap.Error(databaseErr))
	}

	healthUsecase := usecase.NewHealthUsecase(config.StartedAt)
	var authUsecase *usecase.AuthUsecase
	var monitoringUsecase *usecase.MonitoringUsecase
	var clientCloser interface{ Close() error }
	var dockerClientCloser interface{ CloseIdleConnections() }
	var schedulerDone chan struct{}
	schedulerCtx, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()
	if !databaseAvailable {
		authUsecase = nil
	} else if client, err := postgres.Open(config.DatabaseURL); err != nil {
		logger.Warn("PostgreSQL Ent client unavailable", zap.Error(err))
	} else {
		clientCloser = client
		authRepository := postgres.NewAuthRepository(client)
		hasher := argon2adapter.New()
		if config.AdminPassword != "" {
			hash, err := hasher.Hash(config.AdminPassword)
			if err != nil {
				logger.Error("Could not hash initial administrator password", zap.Error(err))
			} else if err := authRepository.EnsureAdmin(context.Background(), config.AdminUsername, hash); err != nil {
				logger.Error("Could not bootstrap administrator", zap.Error(err))
			}
		} else {
			logger.Warn("HOMELAB_ADMIN_PASSWORD is empty; administrator bootstrap was skipped")
		}
		authUsecase = usecase.NewAuthUsecase(authRepository, authRepository, hasher)
		monitoringRepository := postgres.NewMonitoringRepository(client)
		dockerClient := dockerengine.NewClient(config.DockerProxySocket)
		dockerClientCloser = dockerClient
		monitoringUsecase = usecase.NewMonitoringUsecase(
			systemcollector.New(),
			dockerClient,
			monitoringRepository,
			usecase.MonitoringConfig{
				SystemInterval:      time.Duration(config.SystemIntervalSeconds) * time.Second,
				DockerStatsInterval: time.Duration(config.DockerStatsIntervalSeconds) * time.Second,
				DockerScanInterval:  time.Duration(config.DockerDiscoveryIntervalSeconds) * time.Second,
				Retention:           time.Duration(config.MetricsRetentionDays) * 24 * time.Hour,
			},
			logger,
		)
		schedulerDone = make(chan struct{})
		go func() {
			defer close(schedulerDone)
			monitoringUsecase.Run(schedulerCtx)
		}()
	}
	router := httpdelivery.NewRouter(logger, healthUsecase, authUsecase, monitoringUsecase, config.SessionSecure, config.WebRoot)

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
		var serveErr error
		if config.TLSCertFile != "" {
			serveErr = server.ListenAndServeTLS(config.TLSCertFile, config.TLSKeyFile)
		} else {
			serveErr = server.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("HTTP server stopped", zap.Error(serveErr))
			stop <- os.Interrupt
		}
	}()

	<-stop
	cancelScheduler()
	if schedulerDone != nil {
		select {
		case <-schedulerDone:
		case <-time.After(10 * time.Second):
			logger.Error("monitoring workers did not stop before deadline")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("HTTP shutdown failed", zap.Error(err))
	}
	if clientCloser != nil {
		if err := clientCloser.Close(); err != nil {
			logger.Warn("database client close failed", zap.Error(err))
		}
	}
	if dockerClientCloser != nil {
		dockerClientCloser.CloseIdleConnections()
	}
}
