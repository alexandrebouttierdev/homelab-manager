package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	argon2adapter "monoops/backend/internal/adapters/argon2"
	"monoops/backend/internal/adapters/backupunit"
	"monoops/backend/internal/adapters/crontab"
	"monoops/backend/internal/adapters/dockerengine"
	"monoops/backend/internal/adapters/journalctl"
	logindadapter "monoops/backend/internal/adapters/logind"
	"monoops/backend/internal/adapters/notify"
	"monoops/backend/internal/adapters/packages"
	"monoops/backend/internal/adapters/qrcode"
	"monoops/backend/internal/adapters/secretbox"
	"monoops/backend/internal/adapters/sqlite"
	"monoops/backend/internal/adapters/sshkey"
	"monoops/backend/internal/adapters/systemd"
	"monoops/backend/internal/adapters/systemsettings"
	"monoops/backend/internal/adapters/vulnscan"
	webauthnadapter "monoops/backend/internal/adapters/webauthn"
	systemcollector "monoops/backend/internal/collector/system"
	httpdelivery "monoops/backend/internal/delivery/http"
	"monoops/backend/internal/ports"
	"monoops/backend/internal/usecase"

	"go.uber.org/zap"
)

// version is set at build time by scripts/release/build.sh
// (-ldflags "-X main.version=..."); "dev" for a local build.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:], os.Stdout, os.Stderr, LoadConfig))
	}
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
	logger.Info("Starting MonoOps", zap.String("version", version))

	database, databaseErr := sqlite.Open(context.Background(), config.DatabasePath)
	if databaseErr != nil {
		logger.Warn("SQLite database is unavailable or migrations failed; the HTTP health service can still start",
			zap.String("path", config.DatabasePath), zap.Error(databaseErr))
	}

	healthUsecase := usecase.NewHealthUsecase(config.StartedAt)
	var authUsecase *usecase.AuthUsecase
	var accountUsecase *usecase.AccountUsecase
	var powerUsecase *usecase.PowerUsecase
	var apiTokenUsecase *usecase.APITokenUsecase
	var securityUsecase *usecase.SecurityUsecase
	var auditUsecase *usecase.AuditUsecase
	var secondFactorUsecase *usecase.SecondFactorUsecase
	var monitoringUsecase *usecase.MonitoringUsecase
	var clientCloser interface{ Close() error }
	var dockerClientCloser interface{ CloseIdleConnections() }
	var schedulerDone chan struct{}
	// failedUnits stays a nil interface without a database: the service
	// inventory is then served without events.
	var failedUnits ports.FailedUnitRepository
	// secretBox seals the TOTP secrets and the notification secrets; nil
	// without a master key.
	var secretBox ports.SecretBox
	var monitoringRepository *sqlite.MonitoringRepository
	var containerUsecase *usecase.ContainerUsecase
	var containersDone chan struct{}
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
		secretBox = box
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
			if verifier := loadWebAuthn(logger, config); verifier != nil {
				authUsecase.WithWebAuthn(verifier, factorRepository)
				secondFactorUsecase.WithWebAuthn(verifier, factorRepository)
			}
			accountUsecase = usecase.NewAccountUsecase(sqlite.NewAccountRepository(client), authRepository, hasher).
				WithAudit(auditRepository)
			apiTokenUsecase = usecase.NewAPITokenUsecase(sqlite.NewAPITokenRepository(client), authRepository, logger)
			powerUsecase = usecase.NewPowerUsecase(logindadapter.New(), auditRepository, os.Hostname, config.PowerActionsEnabled)
			if config.PowerActionsEnabled {
				logger.Info("Reboot and power off are enabled; they also require the polkit rule")
			}
		}
		failedUnits = sqlite.NewSystemUnitRepository(client)
		monitoringRepository = sqlite.NewMonitoringRepository(database)
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
		// Container logs and actions (D-5): the actions are audited and
		// also need the setting of the proxy.
		containerUsecase = usecase.NewContainerUsecase(monitoringRepository, dockerClient, dockerClient,
			sqlite.NewAuditRepository(client), config.DockerActionsEnabled, logger)
		containersDone = make(chan struct{})
		go func() {
			defer close(containersDone)
			containerUsecase.Run(schedulerCtx)
		}()
		if config.DockerActionsEnabled {
			logger.Info("Container start, stop and restart are enabled; they also require MONOOPS_DOCKER_PROXY_CONTAINER_ACTIONS in the proxy")
		}
	}
	journalUsecase := loadJournal(logger)
	serviceUsecase := usecase.NewServiceUsecase(systemd.New(systemd.SystemBus), failedUnits, journalUsecase, logger)
	servicesDone := make(chan struct{})
	go func() {
		defer close(servicesDone)
		serviceUsecase.Run(schedulerCtx)
	}()
	// The alert engine evaluates the data stored by the collections: it
	// needs the database.
	var alertUsecase *usecase.AlertUsecase
	var notificationUsecase *usecase.NotificationUsecase
	var alertsDone, notificationsDone chan struct{}
	var notifier *notify.Notifier
	if database != nil {
		alertRepository := sqlite.NewAlertRepository(database.Client())
		notificationRepository := sqlite.NewNotificationRepository(database.Client())
		notifier = notify.New()
		notificationUsecase = usecase.NewNotificationUsecase(notificationRepository, alertRepository, secretBox, notifier,
			sqlite.NewAuditRepository(database.Client()), monitoringUsecase, logger)
		alertUsecase = usecase.NewAlertUsecase(alertRepository, usecase.AlertSources{
			Metrics: monitoringRepository, Collectors: monitoringUsecase, Units: serviceUsecase, Retention: monitoringUsecase,
			Vulnerabilities: sqlite.NewVulnerabilityRepository(database.Client()),
			Backups:         sqlite.NewBackupRepository(database.Client()),
		}, logger).WithNotifications(notificationUsecase, notificationRepository)
		alertsDone = make(chan struct{})
		go func() {
			defer close(alertsDone)
			alertUsecase.Run(schedulerCtx)
		}()
		notificationsDone = make(chan struct{})
		go func() {
			defer close(notificationsDone)
			notificationUsecase.Run(schedulerCtx)
		}()
	}
	// Package vulnerabilities: the Manager writes the inventory, starts the
	// monoops-vulnscan.service unit and reads its result; without Grype or
	// the unit, scans fail with vulnscan_not_installed.
	var vulnerabilityUsecase *usecase.VulnerabilityUsecase
	var vulnerabilitiesDone chan struct{}
	if database != nil {
		vulnerabilityUsecase = usecase.NewVulnerabilityUsecase(
			sqlite.NewVulnerabilityRepository(database.Client()), packages.New("/"),
			vulnscan.NewScanner(vulnscan.Config{
				Unit: vulnscan.DefaultUnit, InventoryPath: config.VulnscanInventoryPath, ResultPath: config.VulnscanResultPath,
				Timeout: vulnscan.DefaultTimeout, Version: version,
			}, vulnscan.NewUnitStarter(vulnscan.SystemBus)),
			time.Duration(config.VulnerabilityScanIntervalHours)*time.Hour, logger,
		).WithSettings(sqlite.NewSettingsRepository(database))
		vulnerabilitiesDone = make(chan struct{})
		go func() {
			defer close(vulnerabilitiesDone)
			vulnerabilityUsecase.Run(schedulerCtx)
		}()
	}
	// Service actions: they are audited, so they need the database.
	var unitActionUsecase *usecase.UnitActionUsecase
	var unitActionsDone chan struct{}
	if database != nil {
		unitActionUsecase = usecase.NewUnitActionUsecase(systemd.NewController(systemd.SystemBus),
			sqlite.NewAuditRepository(database.Client()), config.ManageableUnits, logger)
		unitActionsDone = make(chan struct{})
		go func() {
			defer close(unitActionsDone)
			unitActionUsecase.Run(schedulerCtx)
		}()
		if names := config.ManageableUnits.Names(); len(names) > 0 {
			logger.Info("Service actions are allowed on the listed units; they also require the rule of \"monoops polkit-rule\"",
				zap.Strings("units", names))
		}
	}
	// Backups (phase 6): restic runs in monoops-backup@.service only; the
	// Manager writes the job, starts the unit and reads the result. Without
	// the secrets key the repository password cannot be kept: backups are
	// then unavailable.
	var backupUsecase *usecase.BackupUsecase
	var backupsDone chan struct{}
	if database != nil {
		backupUsecase = usecase.NewBackupUsecase(sqlite.NewBackupRepository(database.Client()),
			backupunit.NewExecutor(backupunit.Config{
				JobsDir: config.BackupJobsDir, RunsDir: config.BackupRunsDir,
				InitTimeout: backupunit.DefaultInitTimeout, BackupTimeout: backupunit.DefaultBackupTimeout,
			}, systemd.NewController(systemd.SystemBus)),
			secretBox, sshkey.Keys{}, logger)
		backupsDone = make(chan struct{})
		go func() {
			defer close(backupsDone)
			backupUsecase.Run(schedulerCtx)
		}()
	}
	// System preferences (D-6): host name, time zone and language through
	// hostnamed, timedated and localed; changes are audited and also need
	// the polkit rule.
	var systemPreferencesUsecase *usecase.SystemPreferencesUsecase
	if database != nil {
		systemPreferencesUsecase = usecase.NewSystemPreferencesUsecase(systemsettings.New(systemsettings.SystemBus),
			sqlite.NewAuditRepository(database.Client()), config.SystemPreferencesEnabled, logger)
		if config.SystemPreferencesEnabled {
			logger.Info("Host name, time zone and language changes are enabled; they also require the polkit rule")
		}
	}
	// Maintenance catalog (D-1): fixed tasks scheduled by the Manager, run
	// through units shipped with the project and the polkit rule.
	var maintenanceUsecase *usecase.MaintenanceUsecase
	var maintenanceDone chan struct{}
	if database != nil {
		maintenanceUsecase = usecase.NewMaintenanceUsecase(sqlite.NewMaintenanceRepository(database.Client()),
			systemd.NewController(systemd.SystemBus), sqlite.NewAuditRepository(database.Client()), logger)
		maintenanceDone = make(chan struct{})
		go func() {
			defer close(maintenanceDone)
			maintenanceUsecase.Run(schedulerCtx)
		}()
	}
	// Scheduled tasks (phase 7): system crontabs read-only, the timers of
	// the units inventory and the CRON lines of the journal.
	cronUsecase := usecase.NewCronUsecase(crontab.New("/"), serviceUsecase, journalUsecase, logger)
	router := httpdelivery.NewRouter(logger, httpdelivery.Services{
		Health: healthUsecase, Auth: authUsecase, Monitoring: monitoringUsecase,
		Accounts: accountUsecase, Power: powerUsecase, Security: securityUsecase, Audit: auditUsecase,
		SecondFactors: secondFactorUsecase, Journal: journalUsecase, Units: serviceUsecase, UnitActions: unitActionUsecase,
		Alerts:        alertUsecase,
		Notifications: notificationUsecase, Vulnerabilities: vulnerabilityUsecase, Backups: backupUsecase,
		Cron: cronUsecase, Tokens: apiTokenUsecase, Containers: containerUsecase,
		SystemPreferences: systemPreferencesUsecase, Maintenance: maintenanceUsecase,
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
		logger.Info("MonoOps listening", zap.String("address", config.HTTPAddress))
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
	select {
	case <-servicesDone:
	case <-time.After(10 * time.Second):
		logger.Error("system units collection did not stop before deadline")
	}
	if alertsDone != nil {
		select {
		case <-alertsDone:
		case <-time.After(10 * time.Second):
			logger.Error("alert evaluation did not stop before deadline")
		}
	}
	if notificationsDone != nil {
		// A send in progress ends within its 10 s timeout.
		select {
		case <-notificationsDone:
		case <-time.After(15 * time.Second):
			logger.Error("alert notifications did not stop before deadline")
		}
		notifier.CloseIdleConnections()
	}
	if backupsDone != nil {
		// A running backup is stopped by its context and recorded as
		// interrupted; systemd keeps the unit until restic exits.
		select {
		case <-backupsDone:
		case <-time.After(15 * time.Second):
			logger.Error("backups did not stop before deadline")
		}
	}
	if unitActionsDone != nil {
		// An action being followed ends as interrupted at once.
		select {
		case <-unitActionsDone:
		case <-time.After(10 * time.Second):
			logger.Error("unit actions did not stop before deadline")
		}
	}
	if maintenanceDone != nil {
		// A running task ends with its context.
		select {
		case <-maintenanceDone:
		case <-time.After(15 * time.Second):
			logger.Error("maintenance tasks did not stop before deadline")
		}
	}
	if containersDone != nil {
		// An action being followed ends as interrupted at once.
		select {
		case <-containersDone:
		case <-time.After(10 * time.Second):
			logger.Error("container actions did not stop before deadline")
		}
	}
	if vulnerabilitiesDone != nil {
		// A running scan is recorded as interrupted, within a few seconds.
		select {
		case <-vulnerabilitiesDone:
		case <-time.After(15 * time.Second):
			logger.Error("vulnerability scans did not stop before deadline")
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

// loadWebAuthn returns the verifier of the passkeys and security keys, or
// nil when webauthn_rp_id is empty or the library refuses the settings.
func loadWebAuthn(logger *zap.Logger, config Config) ports.WebAuthnVerifier {
	if config.WebAuthnRPID == "" {
		logger.Info("Passkeys and security keys are off; set webauthn_rp_id and webauthn_origins to enable them")
		return nil
	}
	verifier, err := webauthnadapter.New(webauthnadapter.Config{
		RPID: config.WebAuthnRPID, RPDisplayName: config.WebAuthnRPName, Origins: config.WebAuthnOrigins,
	})
	if err != nil {
		logger.Error("Could not enable passkeys and security keys", zap.Error(err))
		return nil
	}
	logger.Info("Passkeys and security keys are on", zap.String("rp_id", config.WebAuthnRPID), zap.Strings("origins", config.WebAuthnOrigins))
	return verifier
}

// loadSecretBox opens the secrets box with the master key. Without a key the
// Manager keeps running: TOTP and the notification channels with secrets
// are unavailable, recovery codes still work (SPEC_ADMINISTRATION.md § 2.5).
func loadSecretBox(logger *zap.Logger, path string) ports.SecretBox {
	if path == "" {
		logger.Warn("No secrets key configured: TOTP and notification secrets are unavailable, recovery codes still work")
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
