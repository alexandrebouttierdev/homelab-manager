package httpdelivery

import (
	"encoding/json"
	"net/http"
	"time"

	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type healthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

func NewRouter(logger *zap.Logger, health *usecase.HealthUsecase) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(10 * time.Second))
	router.Use(requestLogger(logger))

	router.Get("/health", healthHandler(health))
	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthHandler(health))
	})

	return router
}

func healthHandler(health *usecase.HealthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := health.Status()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status: result.Status, Service: result.Service,
			Timestamp: result.Timestamp, Uptime: result.Uptime,
		})
	}
}

func requestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("request_id", middleware.GetReqID(r.Context())),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
