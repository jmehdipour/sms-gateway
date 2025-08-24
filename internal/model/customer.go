package model

import "time"

type Customer struct {
	ID           int64     `db:"id"`
	Name         string    `db:"name"`
	APIKey       string    `db:"api_key"`
	Status       string    `db:"status"`         // active|suspended
	RateLimitRPS *int      `db:"rate_limit_rps"` // nullable
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
