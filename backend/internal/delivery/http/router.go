package httpdelivery

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type healthResponse struct {
	Status     string         `json:"status"`
	Service    string         `json:"service"`
	Timestamp  time.Time      `json:"timestamp"`
	Uptime     string         `json:"uptime"`
	Database   string         `json:"database"`
	Collectors map[string]any `json:"collectors"`
}

// Services groups the usecases served over HTTP; a nil usecase leaves its
// routes unavailable (for example when the database could not be opened).
type Services struct {
	Health     *usecase.HealthUsecase
	Auth       *usecase.AuthUsecase
	Monitoring *usecase.MonitoringUsecase
	Accounts   *usecase.AccountUsecase
	Power      *usecase.PowerUsecase
	Security   *usecase.SecurityUsecase
	Audit      *usecase.AuditUsecase
	// SecondFactors manages the second factor of the signed-in account.
	SecondFactors *usecase.SecondFactorUsecase
	// Journal reads the systemd journal; nil means unavailable.
	Journal *usecase.JournalUsecase
	// Units serves the systemd units inventory; nil means unavailable.
	Units *usecase.ServiceUsecase
	// UnitActions starts, stops, restarts and reloads the units of the
	// allow list; nil (no database, hence no audit) answers 503.
	UnitActions *usecase.UnitActionUsecase
	// Alerts evaluates the alert rules; nil (no database) answers 503.
	Alerts *usecase.AlertUsecase
	// Notifications sends the alerts and manages silences and channels;
	// nil (no database) answers 503.
	Notifications *usecase.NotificationUsecase
	// Vulnerabilities scans the system packages; nil (no database) answers
	// 503.
	Vulnerabilities *usecase.VulnerabilityUsecase
	// Backups configures and runs restic; nil (no database) answers 503.
	Backups *usecase.BackupUsecase
	// Containers reads the container logs and starts, stops and restarts
	// containers; nil (no database, hence no audit) answers 503.
	Containers *usecase.ContainerUsecase
	// SystemPreferences reads and changes the host name, time zone and
	// language; nil (no database, hence no audit) answers 503.
	SystemPreferences *usecase.SystemPreferencesUsecase
	// Maintenance schedules the tasks of the maintenance catalog (D-1);
	// nil (no database) answers 503.
	Maintenance *usecase.MaintenanceUsecase
	// Cron reads the scheduled tasks; nil answers 503.
	Cron *usecase.CronUsecase
	// Tokens manages and authenticates the API tokens; nil (no database)
	// refuses every bearer token.
	Tokens *usecase.APITokenUsecase
}

func NewRouter(logger *zap.Logger, services Services, sessionSecure bool, webRoot string) http.Handler {
	health, auth, monitoring := services.Health, services.Auth, services.Monitoring
	accounts, power, security, audit := services.Accounts, services.Power, services.Security, services.Audit
	factors := services.SecondFactors
	journal := services.Journal
	if journal == nil {
		journal = usecase.NewJournalUsecase(nil)
	}
	units, alerts, notifications := services.Units, services.Alerts, services.Notifications
	vulnerabilities := services.Vulnerabilities
	unitActions := services.UnitActions
	backups := services.Backups
	cron := services.Cron
	containers := services.Containers
	systemPreferences := services.SystemPreferences
	maintenance := services.Maintenance
	tokens := services.Tokens
	if units == nil {
		// Never run: its inventory stays pending and its detail unavailable.
		units = usecase.NewServiceUsecase(nil, nil, nil, logger)
	}
	deny := func(r *http.Request, user domain.User) {
		if audit == nil {
			return
		}
		if err := audit.RecordDenied(r.Context(), user, r.Method, r.URL.Path, middleware.GetReqID(r.Context())); err != nil {
			logger.Error("Could not audit a denied request",
				zap.String("request_id", middleware.GetReqID(r.Context())), zap.Error(err))
		}
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestSize(1 << 20))
	router.Use(securityHeaders)
	router.Use(requestLogger(logger))
	router.Use(allowedNetworks(security))

	router.Get("/health", healthHandler(health, monitoring, false))
	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthHandler(health, monitoring, true))
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The requested API route does not exist.")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The HTTP method is not allowed on this route.")
		})
		r.With(sameOrigin, loginRateLimit).Post("/auth/login", loginHandler(auth, sessionSecure))
		r.With(sameOrigin, loginRateLimit).Post("/auth/login/second-factor", completeSecondFactorHandler(auth, sessionSecure))
		r.With(sameOrigin, loginRateLimit).Post("/auth/login/second-factor/webauthn", beginWebAuthnLoginHandler(auth, sessionSecure))
		r.Get("/auth/login/passkey", passkeyLoginStatusHandler(auth))
		r.With(sameOrigin, loginRateLimit).Post("/auth/login/passkey", beginPasskeyLoginHandler(auth, sessionSecure))
		r.With(sameOrigin, loginRateLimit).Post("/auth/login/passkey/finish", completePasskeyLoginHandler(auth, sessionSecure))
		r.Get("/auth/session", sessionHandler(auth))
		r.Group(func(protected chi.Router) {
			protected.Use(requireSession(auth, tokens))
			protected.Use(tokenScopeGate(audit, logger))
			protected.Use(sameOrigin)
			protected.Post("/auth/logout", logoutHandler(auth, sessionSecure))
			protected.Post("/auth/password", changePasswordHandler(auth, sessionSecure))
			protected.Group(func(changed chi.Router) {
				changed.Use(requirePasswordChanged)
				changed.Get("/auth/second-factor", secondFactorStatusHandler(factors))
				changed.Post("/auth/second-factor/totp", beginTOTPHandler(factors))
				changed.Post("/auth/second-factor/totp/confirm", confirmTOTPHandler(factors))
				changed.Post("/auth/second-factor/recovery-codes", regenerateRecoveryCodesHandler(factors))
				changed.Post("/auth/second-factor/disable", disableSecondFactorHandler(factors))
				changed.Post("/auth/second-factor/webauthn", beginWebAuthnHandler(factors))
				changed.Post("/auth/second-factor/webauthn/confirm", confirmWebAuthnHandler(factors))
				changed.Post("/auth/second-factor/webauthn/{keyID}/delete", deleteWebAuthnHandler(factors))
				changed.Group(func(active chi.Router) {
					active.Use(requireSecondFactorEnrolled)
					active.Get("/dashboard", dashboardHandler(monitoring))
					active.Get("/host", hostHandler(monitoring))
					active.Get("/host/metrics/current", currentMetricsHandler(monitoring, false))
					active.Get("/host/metrics/{metricName}", metricHistoryHandler(monitoring, false))
					active.Get("/host/temperatures", hostTemperaturesHandler(monitoring))
					active.Get("/containers", listContainersHandler(monitoring))
					active.Get("/containers/{containerID}", getContainerHandler(monitoring))
					active.Get("/containers/{containerID}/metrics/current", currentMetricsHandler(monitoring, true))
					active.Get("/containers/{containerID}/metrics/{metricName}", metricHistoryHandler(monitoring, true))
					active.Get("/containers/{containerID}/events", listEventsHandler(monitoring, true))
					active.With(requireContainers(containers)).Get("/containers/{containerID}/logs", containerLogsHandler(containers))
					active.Route("/containers/{containerID}/actions", func(actions chi.Router) {
						actions.Use(requireContainers(containers))
						actions.With(requirePermission(deny, domain.PermManageContainers)).Post("/", requestContainerActionHandler(containers))
						actions.Get("/last", lastContainerActionHandler(containers))
					})
					active.With(requireContainers(containers)).Get("/docker/container-actions", containerActionsSettingsHandler(containers))
					active.Get("/events", listEventsHandler(monitoring, false))
					active.Get("/docker/inventory", dockerInventoryHandler(monitoring))
					active.Get("/settings", settingsHandler(monitoring))
					active.With(requirePermission(deny, domain.PermManageSettings)).Put("/settings/retention", updateRetentionHandler(monitoring))
					active.With(requireVulnerabilities(vulnerabilities), requirePermission(deny, domain.PermManageSettings)).
						Put("/settings/vulnerability-scans", updateScanIntervalHandler(vulnerabilities))
					active.Route("/users", func(users chi.Router) {
						users.Use(requirePermission(deny, domain.PermManageUsers))
						users.Get("/", listUsersHandler(accounts))
						users.Post("/", createUserHandler(accounts))
						users.Patch("/{userID}", updateUserHandler(accounts))
						users.Delete("/{userID}", deleteUserHandler(accounts))
						users.Post("/{userID}/password", resetPasswordHandler(accounts))
					})
					active.Get("/auth/sessions", listSessionsHandler(auth, deny))
					active.Delete("/auth/sessions/{sessionID}", revokeSessionHandler(auth, sessionSecure))
					active.Post("/auth/sessions/revoke-others", revokeOtherSessionsHandler(auth))
					active.Route("/security/policy", func(policy chi.Router) {
						policy.Use(requirePermission(deny, domain.PermManageUsers))
						policy.Get("/", securityPolicyHandler(security))
						policy.Put("/", updateSecurityPolicyHandler(security))
					})
					active.Route("/audit", func(log chi.Router) {
						log.Use(requirePermission(deny, domain.PermViewAudit))
						log.Get("/", listAuditHandler(audit))
						log.Get("/export", exportAuditHandler(audit))
					})
					active.Route("/journal", func(system chi.Router) {
						system.Get("/entries", listJournalHandler(logger, journal))
						system.Get("/entry", journalEntryHandler(logger, journal))
						system.Get("/boots", journalBootsHandler(logger, journal))
						system.Get("/units", journalUnitsHandler(logger, journal))
						system.Get("/summary", journalSummaryHandler(logger, journal))
						system.Get("/export", exportJournalHandler(logger, journal))
					})
					active.Route("/system/preferences", func(system chi.Router) {
						system.Use(requireSystemPreferences(systemPreferences))
						system.Get("/", systemPreferencesHandler(systemPreferences))
						system.With(requirePermission(deny, domain.PermManageSystem)).Put("/", updateSystemPreferencesHandler(systemPreferences))
					})
					active.Route("/system/units", func(system chi.Router) {
						system.Get("/", listUnitsHandler(units, unitActions))
						system.Get("/{unitName}", unitDetailHandler(logger, units))
						system.Route("/{unitName}/actions", func(actions chi.Router) {
							actions.Use(requireUnitActions(unitActions))
							actions.With(requirePermission(deny, domain.PermManageServices)).Post("/", requestUnitActionHandler(unitActions))
							actions.Get("/last", lastUnitActionHandler(unitActions))
						})
					})
					active.Route("/alerts", func(list chi.Router) {
						list.Use(requireAlerts(alerts))
						list.Get("/", listAlertsHandler(alerts))
						list.Get("/summary", alertSummaryHandler(alerts))
						list.Get("/{alertID}", alertHandler(alerts))
						list.With(requirePermission(deny, domain.PermOperateAlerts)).Post("/{alertID}/acknowledge", acknowledgeAlertHandler(alerts))
					})
					active.Route("/alert-rules", func(rules chi.Router) {
						rules.Use(requireAlerts(alerts))
						rules.Get("/", listAlertRulesHandler(alerts))
						rules.Get("/{ruleID}", alertRuleHandler(alerts))
						rules.Group(func(manage chi.Router) {
							manage.Use(requirePermission(deny, domain.PermManageAlerts))
							manage.Post("/", createAlertRuleHandler(alerts))
							manage.Put("/{ruleID}", updateAlertRuleHandler(alerts))
							manage.Delete("/{ruleID}", deleteAlertRuleHandler(alerts))
						})
					})
					active.Route("/alert-silences", func(silences chi.Router) {
						silences.Use(requireNotifications(notifications))
						silences.Get("/", listSilencesHandler(notifications))
						silences.Group(func(operate chi.Router) {
							operate.Use(requirePermission(deny, domain.PermOperateAlerts))
							operate.Post("/", createSilenceHandler(notifications))
							operate.Delete("/{silenceID}", liftSilenceHandler(notifications))
						})
					})
					active.Route("/notification-channels", func(channels chi.Router) {
						channels.Use(requireNotifications(notifications))
						channels.Use(requirePermission(deny, domain.PermManageAlerts))
						channels.Get("/", listChannelsHandler(notifications))
						channels.Post("/", createChannelHandler(notifications))
						channels.Put("/{channelID}", updateChannelHandler(notifications))
						channels.Delete("/{channelID}", deleteChannelHandler(notifications))
						channels.Post("/{channelID}/test", testChannelHandler(notifications))
					})
					active.Route("/vulnerabilities", func(vulns chi.Router) {
						vulns.Use(requireVulnerabilities(vulnerabilities))
						vulns.Get("/", listFindingsHandler(vulnerabilities))
						vulns.Get("/summary", vulnerabilitySummaryHandler(vulnerabilities))
						vulns.Get("/scans", listScansHandler(vulnerabilities))
						vulns.With(requirePermission(deny, domain.PermScanVulnerabilities)).Post("/scans", requestScanHandler(vulnerabilities))
						vulns.Get("/{findingID}", findingHandler(vulnerabilities))
					})
					active.Route("/backups", func(backup chi.Router) {
						backup.Use(requireBackups(backups))
						backup.Get("/summary", backupSummaryHandler(backups))
						backup.Get("/config", backupConfigHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Put("/config", updateBackupConfigHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/repository/init", initBackupRepositoryHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/repository/connect", connectBackupRepositoryHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/operations", requestBackupOperationHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Put("/copy", setBackupCopyHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/copy/remove", removeBackupCopyHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/copy/init", initBackupCopyHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/copy/connect", connectBackupCopyHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Post("/ssh-key", generateBackupSSHKeyHandler(backups))
						backup.Get("/runs", listBackupRunsHandler(backups))
						backup.With(requirePermission(deny, domain.PermRunBackups)).Post("/runs", requestBackupHandler(backups))
						backup.Get("/runs/{runID}", backupRunHandler(backups))
						backup.With(requirePermission(deny, domain.PermManageBackups)).Get("/runs/{runID}/archive", backupArchiveHandler(backups))
						backup.Get("/snapshots", listBackupSnapshotsHandler(backups))
					})
					active.Route("/auth/tokens", func(apiTokens chi.Router) {
						apiTokens.Use(requireAPITokens(tokens))
						apiTokens.Get("/", listAPITokensHandler(tokens, deny))
						apiTokens.Post("/", createAPITokenHandler(tokens))
						apiTokens.Delete("/{tokenID}", revokeAPITokenHandler(tokens))
					})
					active.Route("/maintenance/tasks", func(tasks chi.Router) {
						tasks.Use(requireMaintenance(maintenance))
						tasks.Get("/", listMaintenanceTasksHandler(maintenance))
						tasks.With(requirePermission(deny, domain.PermManageSettings)).Put("/{kind}", updateMaintenanceTaskHandler(maintenance))
						tasks.With(requirePermission(deny, domain.PermManageSettings)).Post("/{kind}/run", runMaintenanceTaskHandler(maintenance))
					})
					active.Route("/cron", func(tasks chi.Router) {
						tasks.Use(func(next http.Handler) http.Handler {
							return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								if cron == nil {
									writeError(w, r, http.StatusServiceUnavailable, "CRON_UNAVAILABLE", "Scheduled tasks are unavailable.")
									return
								}
								next.ServeHTTP(w, r)
							})
						})
						tasks.Get("/", cronHandler(logger, cron))
						tasks.Get("/inspect", cronInspectHandler(cron))
					})
					active.Route("/machine/power", func(machine chi.Router) {
						machine.Use(requirePermission(deny, domain.PermManagePower))
						machine.Get("/", powerSettingsHandler(power))
						machine.Post("/{action}", powerActionHandler(power))
					})
				})
			})
		})
	})
	if webRoot != "" {
		router.Handle("/*", spaHandler(webRoot))
	}
	return router
}

func spaHandler(webRoot string) http.Handler {
	files := http.FileServer(http.Dir(webRoot))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := filepath.Join(webRoot, filepath.Clean("/"+r.URL.Path))
		info, err := os.Stat(name)
		if err == nil && info.Mode().IsRegular() {
			files.ServeHTTP(w, r)
			return
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "could not read web asset", http.StatusInternalServerError)
			return
		}
		index := filepath.Join(webRoot, "index.html")
		if _, err := os.Stat(index); errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "could not read web application", http.StatusInternalServerError)
			return
		}
		http.ServeFile(w, r, index)
	})
}

func healthHandler(health *usecase.HealthUsecase, monitoring *usecase.MonitoringUsecase, detailed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		result := health.Status()
		if !detailed {
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "ok", "service": result.Service,
				"timestamp": result.Timestamp, "uptime": result.Uptime,
			})
			return
		}
		system := domain.CollectorStatus{Status: "unavailable"}
		docker := domain.CollectorStatus{Status: "unavailable"}
		database := domain.CollectorStatus{Status: "unavailable"}
		if monitoring != nil {
			database, system, docker = monitoring.HealthStatus()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		status := "ok"
		if database.Status != "ok" || system.Status != "ok" || docker.Status != "ok" {
			status = "degraded"
		}
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status: status, Service: result.Service, Timestamp: result.Timestamp,
			Uptime: result.Uptime, Database: database.Status,
			Collectors: map[string]any{"system": system, "docker": docker},
		})
	}
}

func requestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.Status()),
				zap.String("request_id", middleware.GetReqID(r.Context())),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
