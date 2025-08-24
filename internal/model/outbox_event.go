package model

import "time"

type OutboxEvent struct {
	ID          int64     `db:"id"`
	Aggregate   string    `db:"aggregate"`    // e.g. "message"
	AggregateID string    `db:"aggregate_id"` // message.ID
	Topic       string    `db:"topic"`
	Payload     []byte    `db:"payload"`
	Attempts    int       `db:"attempts"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
