package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/config"
	"github.com/jmehdipour/sms-gateway/internal/db"
	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed the database with demo customers",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1) load config
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// 2) connect MySQL
		sqlDB, err := db.NewMySQLConnection(cfg.MySQL.DSN, db.MySQLOpts{
			MaxOpenConns:    cfg.MySQL.MaxOpenConns,
			MaxIdleConns:    cfg.MySQL.MaxIdleConns,
			ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.MySQL.ConnMaxIdleTime,
			PingTimeout:     cfg.MySQL.PingTimeout,
		})
		if err != nil {
			return fmt.Errorf("mysql connect: %w", err)
		}
		defer sqlDB.Close()

		log.Println(">> Seeding demo customers...")

		if err := seedCustomers(sqlDB); err != nil {
			return err
		}
		if err := ensureWallets(sqlDB); err != nil {
			return err
		}

		log.Println(">> Seed completed ✅")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(seedCmd)
}

// seedCustomers inserts 5 deterministic demo customers (idempotent).
func seedCustomers(dbx *sqlx.DB) error {
	customers := []model.Customer{
		{
			Name:         "Acme Corp",
			APIKey:       "11111111111111111111111111111111",
			Status:       "active",
			RateLimitRPS: intptr(20),
		},
		{
			Name:         "Foobar LLC",
			APIKey:       "22222222222222222222222222222222",
			Status:       "active",
			RateLimitRPS: intptr(50),
		},
		{
			Name:         "Beta Testers",
			APIKey:       "33333333333333333333333333333333",
			Status:       "active",
			RateLimitRPS: intptr(5),
		},
		{
			Name:         "Suspended Inc",
			APIKey:       "44444444444444444444444444444444",
			Status:       "suspended",
			RateLimitRPS: nil,
		},
		{
			Name:         "Express Partner",
			APIKey:       "55555555555555555555555555555555",
			Status:       "active",
			RateLimitRPS: intptr(100),
		},
	}

	// idempotent upsert based on api_key (UNIQUE)
	const q = `
INSERT INTO customers
    (name, api_key, status, rate_limit_rps, created_at, updated_at)
VALUES
    (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    name        = VALUES(name),
    status      = VALUES(status),
    rate_limit_rps = VALUES(rate_limit_rps),
    updated_at  = VALUES(updated_at)
`
	tx, err := dbx.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()
	for _, c := range customers {
		if _, err := tx.Exec(q, c.Name, c.APIKey, c.Status, c.RateLimitRPS, now, now); err != nil {
			return fmt.Errorf("insert customer %q: %w", c.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit customers: %w", err)
	}
	return nil
}

// ensureWallets creates wallet_accounts for customers who don't have one yet.
func ensureWallets(dbx *sqlx.DB) error {
	const q = `
INSERT INTO wallet_accounts (customer_id, balance, reserved, updated_at)
SELECT c.id, 0, 0, NOW()
FROM customers c
LEFT JOIN wallet_accounts w ON w.customer_id = c.id
WHERE w.customer_id IS NULL
`
	if _, err := dbx.Exec(q); err != nil {
		return fmt.Errorf("ensure wallets: %w", err)
	}
	return nil
}

func intptr(i int) *int { return &i }
