package model

// Envelope is the payload published to Kafka (via Debezium outbox SMT).
type Envelope struct {
	ID     string `json:"id"`      // message ULID
	UserID int64  `json:"user_id"` // customer id
	SMS    SMS    `json:"sms"`
}
