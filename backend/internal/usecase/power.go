package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const auditTargetHost = "host"

// PowerUsecase reboots or powers off the machine on an administrator's
// request (SPEC_ADMINISTRATION.md § 8 bis). Callers must hold
// domain.PermManagePower; the HTTP layer enforces it.
type PowerUsecase struct {
	controller ports.PowerController
	audit      ports.AuditRecorder
	hostname   func() (string, error)
	enabled    bool

	mu sync.Mutex
	// accepted is set once the system accepted a request: the machine is
	// going down and any further request is refused.
	accepted bool
}

func NewPowerUsecase(controller ports.PowerController, audit ports.AuditRecorder, hostname func() (string, error), enabled bool) *PowerUsecase {
	return &PowerUsecase{controller: controller, audit: audit, hostname: hostname, enabled: enabled}
}

// Enabled reports whether the service configuration allows power actions.
func (u *PowerUsecase) Enabled() bool { return u.enabled }

func (u *PowerUsecase) Request(ctx context.Context, actor domain.User, action domain.PowerAction, confirmation, requestID string) error {
	if !action.Valid() {
		return domain.ErrInvalidPowerAction
	}
	if !u.enabled {
		return domain.ErrPowerActionsDisabled
	}
	hostname, err := u.hostname()
	if err != nil {
		return fmt.Errorf("read host name: %w", err)
	}
	if !domain.ConfirmsHostname(confirmation, hostname) {
		return domain.ErrHostnameMismatch
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if u.accepted {
		return domain.ErrActionInProgress
	}
	entry := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetHost, TargetID: hostname,
		Action: string(action), RequestID: requestID, Outcome: "requested",
	}
	// No trace, no action: the machine may stop before anything else is
	// written, so the entry is committed first.
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	if err := u.controller.Request(ctx, action); err != nil {
		entry.Outcome = "failure"
		if auditErr := u.audit.RecordAudit(ctx, entry); auditErr != nil {
			return errors.Join(err, fmt.Errorf("record audit: %w", auditErr))
		}
		return err
	}
	u.accepted = true
	return nil
}
