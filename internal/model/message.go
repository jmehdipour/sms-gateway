package model

import "time"

type MessageStatus string

const (
	StatusQueued MessageStatus = "queued"
	StatusSent   MessageStatus = "sent"
	StatusFailed MessageStatus = "failed"
)

func (s MessageStatus) String() string {
	return string(s)
}

func (s MessageStatus) Valid() bool {
	return s == StatusQueued || s == StatusSent || s == StatusFailed
}

// Message is the DB entity persisted in messages table.
type Message struct {
	ID         string        `db:"id"`
	CustomerID int64         `db:"customer_id"`
	Phone      string        `db:"phone"`
	Text       string        `db:"text"`
	Type       SMSType       `db:"type"` // normal|express
	Status     MessageStatus `db:"status"`
	CreatedAt  time.Time     `db:"created_at"`
	UpdatedAt  time.Time     `db:"updated_at"`
}
