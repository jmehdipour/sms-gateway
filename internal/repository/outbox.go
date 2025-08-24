package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// OutboxRepository defines persistence methods for the outbox table.
type OutboxRepository interface {
	// Insert writes a single outbox event. If tx is nil, it will open/commit
	// an internal transaction; otherwise it uses the given tx.
	Insert(ctx context.Context, tx *sqlx.Tx, aggregate, aggregateID, topic string, payload []byte) error
}

// OutboxRepositoryImpl is a sqlx-backed implementation.
type OutboxRepositoryImpl struct {
	db *sqlx.DB
}

// NewOutboxRepository constructs an OutboxRepositoryImpl.
func NewOutboxRepository(db *sqlx.DB) *OutboxRepositoryImpl {
	return &OutboxRepositoryImpl{db: db}
}

// withTx runs fn in the provided tx, or starts a new transaction when tx is nil.
func (r *OutboxRepositoryImpl) withTx(ctx context.Context, tx *sqlx.Tx, fn func(*sqlx.Tx) error) error {
	if tx != nil {
		return fn(tx)
	}

	t, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = t.Rollback() }()
	if err := fn(t); err != nil {
		return err
	}

	return t.Commit()
}

// Insert adds an event row to outbox. Debezium Outbox SMT will pick it up and
// publish to Kafka based on the `topic` column.
func (r *OutboxRepositoryImpl) Insert(ctx context.Context, tx *sqlx.Tx, aggregate, aggregateID, topic string, payload []byte) error {
	const q = `
		INSERT INTO outbox (aggregate, aggregate_id, topic, payload, created_at)
		VALUES (?, ?, ?, ?, NOW())
	`
	return r.withTx(ctx, tx, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, q, aggregate, aggregateID, topic, payload)

		return err
	})
}
