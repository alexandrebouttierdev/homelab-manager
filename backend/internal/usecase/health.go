package usecase

import (
	"time"

	"monoops/backend/internal/domain"
)

type HealthUsecase struct {
	startedAt time.Time
}

func NewHealthUsecase(startedAt time.Time) *HealthUsecase {
	return &HealthUsecase{startedAt: startedAt}
}

func (u *HealthUsecase) Status() domain.Health {
	now := time.Now().UTC()
	return domain.Health{
		Status:    "ok",
		Service:   "monoops",
		Timestamp: now,
		Uptime:    now.Sub(u.startedAt).Round(time.Second).String(),
	}
}
