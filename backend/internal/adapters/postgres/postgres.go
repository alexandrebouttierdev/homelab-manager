package postgres

import (
	"context"
	"database/sql"
	"time"

	"homelab-manager/backend/ent"

	_ "github.com/lib/pq"
)

func CheckConnection(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func Open(databaseURL string) (*ent.Client, error) {
	return ent.Open("postgres", databaseURL)
}
