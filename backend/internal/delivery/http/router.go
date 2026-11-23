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

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

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

func NewRouter(logger *zap.Logger, health *usecase.HealthUsecase, auth *usecase.AuthUsecase, monitoring *usecase.MonitoringUsecase, accounts *usecase.AccountUsecase, sessionSecure bool, webRoot string) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestSize(1 << 20))
	router.Use(securityHeaders)
	router.Use(requestLogger(logger))

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
		r.Get("/auth/session", sessionHandler(auth))
		r.Group(func(protected chi.Router) {
			protected.Use(requireSession(auth))
			protected.Use(sameOrigin)
			protected.Post("/auth/logout", logoutHandler(auth, sessionSecure))
			protected.Post("/auth/password", changePasswordHandler(auth, sessionSecure))
			protected.Group(func(active chi.Router) {
				active.Use(requirePasswordChanged)
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
				active.Get("/events", listEventsHandler(monitoring, false))
				active.Get("/settings", settingsHandler(monitoring))
				active.With(requirePermission(domain.PermManageSettings)).Put("/settings/retention", updateRetentionHandler(monitoring))
				active.Route("/users", func(users chi.Router) {
					users.Use(requirePermission(domain.PermManageUsers))
					users.Get("/", listUsersHandler(accounts))
					users.Post("/", createUserHandler(accounts))
					users.Patch("/{userID}", updateUserHandler(accounts))
					users.Delete("/{userID}", deleteUserHandler(accounts))
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
