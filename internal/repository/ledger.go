package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type LedgerRepository interface {
	ExistsByIdem(ctx context.Context, tx *sqlx.Tx, idem string) (bool, error)
	InsertTopup(ctx context.Context, tx *sqlx.Tx, customerID int64, amount int64, idem string) error
	InsertReserve(ctx context.Context, tx *sqlx.Tx, customerID int64, amount int64, msgID, idem string) error
	InsertCaptureBatch(ctx context.Context, tx *sqlx.Tx, rows []LedgerRow) error
	InsertRefundBatch(ctx context.Context, tx *sqlx.Tx, rows []LedgerRow) error
}

type ledgerRepo struct{}

func NewLedgerRepository() LedgerRepository { return &ledgerRepo{} }

type LedgerRow struct {
	CustomerID int64
	Amount     int64
	MessageID  string
}

// ExistsByIdem checks if a ledger row with the given idempotency key already exists.
func (r *ledgerRepo) ExistsByIdem(ctx context.Context, tx *sqlx.Tx, idem string) (bool, error) {
	var one int
	err := tx.QueryRowxContext(ctx,
		`SELECT 1 FROM wallet_ledger WHERE idempotency_key = ? LIMIT 1`, idem,
	).Scan(&one)

	if err != nil {
		// no rows means false
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *ledgerRepo) InsertTopup(ctx context.Context, tx *sqlx.Tx, customerID int64, amount int64, idem string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_ledger (customer_id, op, amount, idempotency_key)
		VALUES (?, 'topup', ?, ?)
		ON DUPLICATE KEY UPDATE id = id
	`, customerID, amount, idem)
	return err
}

func (r *ledgerRepo) InsertReserve(ctx context.Context, tx *sqlx.Tx, customerID int64, amount int64, msgID, idem string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_ledger (customer_id, op, amount, idempotency_key, message_id)
		VALUES (?, 'reserve', ?, ?, ?)
		ON DUPLICATE KEY UPDATE id = id
	`, customerID, amount, idem, msgID)
	return err
}

func (r *ledgerRepo) InsertCaptureBatch(ctx context.Context, tx *sqlx.Tx, rows []LedgerRow) error {
	return r.insertBatch(ctx, tx, "capture", rows)
}

func (r *ledgerRepo) InsertRefundBatch(ctx context.Context, tx *sqlx.Tx, rows []LedgerRow) error {
	return r.insertBatch(ctx, tx, "refund", rows)
}

func (r *ledgerRepo) insertBatch(ctx context.Context, tx *sqlx.Tx, op string, rows []LedgerRow) error {
	if len(rows) == 0 {
		return nil
	}

	var sb strings.Builder
	args := make([]any, 0, len(rows)*5)

	sb.WriteString(`INSERT INTO wallet_ledger (customer_id, op, amount, idempotency_key, message_id) VALUES `)
	for i, rw := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?, ?, ?, ?)")
		// idempotency_key: cap-<msg> / ref-<msg>
		idem := fmt.Sprintf("%s-%s", op[:3], rw.MessageID)
		args = append(args, rw.CustomerID, op, rw.Amount, idem, rw.MessageID)
	}
	sb.WriteString(` ON DUPLICATE KEY UPDATE id = id`)

	_, err := tx.ExecContext(ctx, sb.String(), args...)
	return err
}
