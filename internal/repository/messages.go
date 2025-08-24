package repository

import (
	"context"

	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmoiron/sqlx"
)

// MessagesRepository defines persistence for the messages table (no attempts, no provider).
type MessagesRepository interface {
	InsertQueued(ctx context.Context, tx *sqlx.Tx, m model.Message) error
	BatchUpdateStatus(ctx context.Context, tx *sqlx.Tx, ids []string, status model.MessageStatus) error
}

type MessagesRepositoryImpl struct {
	db *sqlx.DB
}

func NewMessagesRepository(db *sqlx.DB) *MessagesRepositoryImpl {
	return &MessagesRepositoryImpl{db: db}
}

func (r *MessagesRepositoryImpl) withTx(ctx context.Context, tx *sqlx.Tx, fn func(*sqlx.Tx) error) error {
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

// InsertQueued inserts a new message row with status=queued.
func (r *MessagesRepositoryImpl) InsertQueued(ctx context.Context, tx *sqlx.Tx, m model.Message) error {
	const q = `
		INSERT INTO messages
		    (id, customer_id, phone, text, type, status, created_at, updated_at)
		VALUES
		    (?,  ?,           ?,     ?,   ?,   'queued', NOW(),    NOW())
	`
	return r.withTx(ctx, tx, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, q,
			m.ID, m.CustomerID, m.Phone, m.Text, m.Type.String(),
		)
		return err
	})
}

// BatchUpdateStatus updates status for many messages using a single statement.
func (r *MessagesRepositoryImpl) BatchUpdateStatus(ctx context.Context, tx *sqlx.Tx, ids []string, status model.MessageStatus) error {
	if len(ids) == 0 {
		return nil
	}
	const base = `UPDATE messages SET status = ?, updated_at = NOW() WHERE id IN (?)`
	query, args, err := sqlx.In(base, status, ids)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)

	return r.withTx(ctx, tx, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}
