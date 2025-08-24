package model

import "time"

// WalletAccount represents customer's SMS credits.
type WalletAccount struct {
	CustomerID int64     `db:"customer_id"`
	Balance    int64     `db:"balance"`
	Reserved   int64     `db:"reserved"`
	UpdatedAt  time.Time `db:"updated_at"`
	CreatedAt  time.Time `db:"created_at"`
}
