package db

import (
	"context"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jmoiron/sqlx"
)

type ClickHouseOpts struct {
	DSN             string // e.g. clickhouse://default:@localhost:9000/smsgw?dial_timeout=5s&compress=true
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration // default 3s
}

func NewClickHouseConnection(opts ClickHouseOpts) (*sqlx.DB, error) {
	if opts.PingTimeout <= 0 {
		opts.PingTimeout = 3 * time.Second
	}
	db, err := sqlx.Open("clickhouse", opts.DSN)
	if err != nil {
		return nil, err
	}

	if opts.MaxOpenConns > 0 {
		db.SetMaxOpenConns(opts.MaxOpenConns)
	}
	if opts.MaxIdleConns > 0 {
		db.SetMaxIdleConns(opts.MaxIdleConns)
	}
	if opts.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}
	if opts.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(opts.ConnMaxIdleTime)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.PingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
