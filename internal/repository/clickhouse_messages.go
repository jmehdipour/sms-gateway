package repository

import (
	"context"

	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmoiron/sqlx"
)

// CHMessagesRepository lists messages from ClickHouse (final view).
type CHMessagesRepository interface {
	ListByCustomer(ctx context.Context, customerID int64, phone string, status model.MessageStatus, limit, offset int) ([]model.Message, error)
}

type chMessagesRepository struct {
	ch *sqlx.DB // ClickHouse connection
}

func NewCHMessagesRepository(ch *sqlx.DB) CHMessagesRepository {
	return &chMessagesRepository{ch: ch}
}

func (r *chMessagesRepository) ListByCustomer(ctx context.Context, customerID int64, phone string, status model.MessageStatus, limit, offset int) ([]model.Message, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	q := `
		SELECT id, customer_id, phone, text, type, status, created_at, updated_at
		FROM smsgw.messages_latest
		WHERE customer_id = ?
	`
	args := []any{customerID}
	
	if status != "" {
		q += " AND status = ?"
		args = append(args, status.String())
	}
	if phone != "" {
		q += " AND phone = ?"
		args = append(args, phone)
	}

	q += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []model.Message
	if err := r.ch.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, err
	}
	return rows, nil
}
