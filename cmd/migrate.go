package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmehdipour/sms-gateway/internal/config"
	"github.com/jmehdipour/sms-gateway/internal/db"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations (dev: DROP & CREATE tables)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		opts := db.MySQLOpts{
			MaxOpenConns:    cfg.MySQL.MaxOpenConns,
			MaxIdleConns:    cfg.MySQL.MaxIdleConns,
			ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.MySQL.ConnMaxIdleTime,
			PingTimeout:     cfg.MySQL.PingTimeout,
		}
		sqlDB, err := db.NewMySQLConnection(cfg.MySQL.DSN, opts)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer sqlDB.Close()

		sqlPath := filepath.Join("migrations", "001_init.sql")
		sqlBytes, err := os.ReadFile(sqlPath)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", sqlPath, err)
		}

		if _, err := sqlDB.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
			return fmt.Errorf("disable fk checks: %w", err)
		}
		if _, err := sqlDB.Exec(string(sqlBytes)); err != nil {
			_, _ = sqlDB.Exec("SET FOREIGN_KEY_CHECKS = 1")
			return fmt.Errorf("exec migration: %w", err)
		}
		if _, err := sqlDB.Exec("SET FOREIGN_KEY_CHECKS = 1"); err != nil {
			return fmt.Errorf("enable fk checks: %w", err)
		}

		fmt.Println(">> Migration complete ✅")
		return nil
	},
}
