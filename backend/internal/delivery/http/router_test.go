package httpdelivery

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

func TestHealthEndpoints(t *testing.T) {
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()))

	for _, path := range []string{"/health", "/api/v1/health"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("got status %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("got content type %q", got)
			}
		})
	}
}
