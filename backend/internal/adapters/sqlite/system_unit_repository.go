package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/failedunit"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

var _ ports.FailedUnitRepository = (*SystemUnitRepository)(nil)

// SystemUnitRepository stores the systemd units recorded as failed and the
// service events their transitions produce.
type SystemUnitRepository struct {
	client *ent.Client
}

func NewSystemUnitRepository(client *ent.Client) *SystemUnitRepository {
	return &SystemUnitRepository{client: client}
}

func (r *SystemUnitRepository) FailedUnits(ctx context.Context) (map[string]struct{}, error) {
	names, err := r.client.FailedUnit.Query().IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read failed units: %w", err)
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set, nil
}

func (r *SystemUnitRepository) ApplyFailureTransitions(ctx context.Context, transitions domain.FailureTransitions, at time.Time) error {
	if len(transitions.Added) == 0 && len(transitions.Removed) == 0 && len(transitions.Events) == 0 {
		return nil
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		if len(transitions.Removed) > 0 {
			if _, err := tx.FailedUnit.Delete().Where(failedunit.IDIn(transitions.Removed...)).Exec(ctx); err != nil {
				return fmt.Errorf("remove failed units: %w", err)
			}
		}
		for _, name := range transitions.Added {
			if err := tx.FailedUnit.Create().SetID(name).SetFailedAt(at.UTC()).Exec(ctx); err != nil {
				return fmt.Errorf("record failed unit: %w", err)
			}
		}
		for _, event := range transitions.Events {
			if err := writeEvent(ctx, tx, event); err != nil {
				return err
			}
		}
		return nil
	})
}
