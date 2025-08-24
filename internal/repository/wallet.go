package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type WalletRepository interface {
	UpsertAccount(ctx context.Context, tx *sqlx.Tx, customerID int64) error
	GetForUpdate(ctx context.Context, tx *sqlx.Tx, customerID int64) (balance, reserved int64, err error)
	Adjust(ctx context.Context, tx *sqlx.Tx, customerID, deltaBalance, deltaReserved int64) error
	Topup(ctx context.Context, tx *sqlx.Tx, customerID, amount int64) error

	BatchApplySums(ctx context.Context, tx *sqlx.Tx, deltas []WalletDelta) error
}

type walletRepo struct{}

func NewWalletRepository() WalletRepository { return &walletRepo{} }

func (r *walletRepo) UpsertAccount(ctx context.Context, tx *sqlx.Tx, customerID int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_accounts (customer_id, balance, reserved, created_at, updated_at)
		VALUES (?, 0, 0, NOW(), NOW())
		ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)
	`, customerID)
	return err
}

func (r *walletRepo) GetForUpdate(ctx context.Context, tx *sqlx.Tx, customerID int64) (int64, int64, error) {
	var bal, rsv int64
	err := tx.QueryRowxContext(ctx, `
		SELECT balance, reserved
		FROM wallet_accounts
		WHERE customer_id = ?
		FOR UPDATE
	`, customerID).Scan(&bal, &rsv)
	return bal, rsv, err
}

func (r *walletRepo) Adjust(ctx context.Context, tx *sqlx.Tx, customerID, dBal, dRsv int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE wallet_accounts
		SET balance = balance + ?, reserved = reserved + ?, updated_at = NOW()
		WHERE customer_id = ?
	`, dBal, dRsv, customerID)
	return err
}

func (r *walletRepo) Topup(ctx context.Context, tx *sqlx.Tx, customerID, amount int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE wallet_accounts
		SET balance = balance + ?, updated_at = NOW()
		WHERE customer_id = ?
	`, amount, customerID)
	return err
}

type WalletDelta struct {
	CustomerID  int64
	DecReserved int64
	IncBalance  int64
}

func (r *walletRepo) BatchApplySums(ctx context.Context, tx *sqlx.Tx, deltas []WalletDelta) error {
	if len(deltas) == 0 {
		return nil
	}

	var sbRaw strings.Builder
	args := make([]any, 0, len(deltas)*3)

	sbRaw.WriteString("SELECT ? AS customer_id, ? AS dec_reserved, ? AS inc_balance")
	for i, d := range deltas {
		if i == 0 {
			args = append(args, d.CustomerID, d.DecReserved, d.IncBalance)
			continue
		}
		sbRaw.WriteString(" UNION ALL SELECT ?, ?, ?")
		args = append(args, d.CustomerID, d.DecReserved, d.IncBalance)
	}

	query := fmt.Sprintf(`
		UPDATE wallet_accounts w
		JOIN (
			SELECT customer_id,
			       SUM(dec_reserved) AS dec_reserved,
			       SUM(inc_balance)  AS inc_balance
			FROM (
				%s
			) raw
			GROUP BY customer_id
		) s ON s.customer_id = w.customer_id
		SET w.reserved  = w.reserved - s.dec_reserved,
		    w.balance   = w.balance  + s.inc_balance,
		    w.updated_at = NOW()
	`, sbRaw.String())

	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
