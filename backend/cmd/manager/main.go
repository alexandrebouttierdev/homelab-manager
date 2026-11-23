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
	"homelab-manager/backend/internal/adapters/journalctl"
	logindadapter "homelab-manager/backend/internal/adapters/logind"
	"homelab-manager/backend/internal/adapters/qrcode"
	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	systemcollector "homelab-manager/backend/internal/collector/system"
	httpdelivery "homelab-manager/backend/internal/delivery/http"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

// version is set at build time by scripts/release/build.sh
// (-ldflags "-X main.version=..."); "dev" for a local build.
var version = "dev"

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
	logger.Info("Starting Homelab Manager", zap.String("version", version))

	database, databaseErr := sqlite.Open(context.Background(), config.DatabasePath)
	if databaseErr != nil {
		logger.Warn("SQLite database is unavailable or migrations failed; the HTTP health service can still start",
			zap.String("path", config.DatabasePath), zap.Error(databaseErr))
	}

	healthUsecase := usecase.NewHealthUsecase(config.StartedAt)
	var authUsecase *usecase.AuthUsecase
	var accountUsecase *usecase.AccountUsecase
	var powerUsecase *usecase.PowerUsecase
	var securityUsecase *usecase.SecurityUsecase
	var auditUsecase *usecase.AuditUsecase
	var secondFactorUsecase *usecase.SecondFactorUsecase
	var monitoringUsecase *usecase.MonitoringUsecase
	var clientCloser interface{ Close() error }
	var dockerClientCloser interface{ CloseIdleConnections() }
	var schedulerDone chan struct{}
	schedulerCtx, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()
	if database != nil {
		clientCloser = database
		client := database.Client()
		authRepository := sqlite.NewAuthRepository(client)
		hasher := argon2adapter.New()
		auth := usecase.NewAuthUsecase(authRepository, authRepository, hasher)
		bootstrapPassword, isDefault := config.BootstrapAdminPassword()
		if created, forced, err := auth.EnsureAdministrator(context.Background(), config.AdminUsername, bootstrapPassword, isDefault); err != nil {
			logger.Error("Could not bootstrap administrator", zap.Error(err))
		} else if forced {
			logger.Warn("Administrator uses the default password; it must be changed at the next login",
				zap.String("username", config.AdminUsername), zap.Bool("created", created))
		} else if created {
			logger.Info("Administrator created", zap.String("username", config.AdminUsername))
		}
		auditRepository := sqlite.NewAuditRepository(client)
		securityRepository := sqlite.NewSecurityRepository(client)
		factorRepository := sqlite.NewSecondFactorRepository(client)
		box := loadSecretBox(logger, config.SecretsKeyPath(os.Getenv("CREDENTIALS_DIRECTORY")))
		security := usecase.NewSecurityUsecase(securityRepository).WithSecondFactors(factorRepository)
		// Without its policy the Manager cannot tell which networks may use
		// the interface: authentication stays unavailable rather than open.
		if err := security.Load(context.Background()); err != nil {
			logger.Error("Could not load the security policy; authentication is unavailable", zap.Error(err))
		} else {
			securityUsecase = security
			auditUsecase = usecase.NewAuditUsecase(auditRepository, auditRepository)
			authUsecase = auth.WithSecurity(securityRepository, auditRepository, security).
				WithSecondFactor(factorRepository, box)
			secondFactorUsecase = usecase.NewSecondFactorUsecase(authRepository, factorRepository, box, qrcode.New(), hasher)
			accountUsecase = usecase.NewAccountUsecase(sqlite.NewAccountRepository(client), authRepository, hasher).
				WithAudit(auditRepository)
			powerUsecase = usecase.NewPowerUsecase(logindadapter.New(), auditRepository, os.Hostname, config.PowerActionsEnabled)
			if config.PowerActionsEnabled {
				logger.Info("Reboot and power off are enabled; they also require the polkit rule")
			}
		}
		monitoringRepository := sqlite.NewMonitoringRepository(database)
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
		).WithSettings(sqlite.NewSettingsRepository(database))
		schedulerDone = make(chan struct{})
		go func() {
			defer close(schedulerDone)
			monitoringUsecase.Run(schedulerCtx)
		}()
	}
	router := httpdelivery.NewRouter(logger, httpdelivery.Services{
		Health: healthUsecase, Auth: authUsecase, Monitoring: monitoringUsecase,
		Accounts: accountUsecase, Power: powerUsecase, Security: securityUsecase, Audit: auditUsecase,
		SecondFactors: secondFactorUsecase, Journal: loadJournal(logger),
	}, config.SessionSecure, config.WebRoot)

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

// loadSecretBox opens the secrets box with the master key. Without a key the
// Manager keeps running: TOTP is unavailable, recovery codes still work
// (SPEC_ADMINISTRATION.md § 2.5).
func loadSecretBox(logger *zap.Logger, path string) ports.SecretBox {
	if path == "" {
		logger.Warn("No secrets key configured: TOTP is unavailable, recovery codes still work")
		return nil
	}
	key, err := secretbox.LoadKeyFile(path)
	if err != nil {
		logger.Error("Could not load the secrets key: TOTP is unavailable", zap.String("path", path), zap.Error(err))
		return nil
	}
	box, err := secretbox.New(key)
	if err != nil {
		logger.Error("Invalid secrets key: TOTP is unavailable", zap.Error(err))
		return nil
	}
	logger.Info("Secrets key loaded")
	return box
}

// loadJournal finds journalctl; without it the journal routes answer
// JOURNAL_UNAVAILABLE and the rest of the Manager works.
func loadJournal(logger *zap.Logger) *usecase.JournalUsecase {
	path, err := journalctl.Find()
	if err != nil {
		logger.Warn("journalctl not found: the system journal is unavailable", zap.Error(err))
		return usecase.NewJournalUsecase(nil)
	}
	reader := journalctl.New(path)
	if !reader.FullAccess() {
		logger.Warn("The Manager is not in the systemd-journal group: the journal shows its own entries only")
	}
	return usecase.NewJournalUsecase(reader)
}
