package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/config"
	"github.com/jmehdipour/sms-gateway/internal/db"
	httpSrv "github.com/jmehdipour/sms-gateway/internal/http"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run HTTP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		mysqlDB, err := db.NewMySQLConnection(cfg.MySQL.DSN, db.MySQLOpts{
			MaxOpenConns:    cfg.MySQL.MaxOpenConns,
			MaxIdleConns:    cfg.MySQL.MaxIdleConns,
			ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.MySQL.ConnMaxIdleTime,
			PingTimeout:     cfg.MySQL.PingTimeout,
		})
		if err != nil {
			return fmt.Errorf("mysql connect: %w", err)
		}
		defer mysqlDB.Close()

		redisClient, err := db.NewRedisClient(db.RedisOpts{
			Addr:        cfg.Redis.Addr,
			Password:    cfg.Redis.Password,
			DB:          cfg.Redis.DB,
			DialTimeout: cfg.Redis.DialTimeout,
		})
		if err != nil {
			return fmt.Errorf("redis connect: %w", err)
		}
		defer func() { _ = redisClient.Close() }()

		chDB, err := db.NewClickHouseConnection(db.ClickHouseOpts{
			DSN:             cfg.ClickHouse.DSN,
			MaxOpenConns:    cfg.ClickHouse.MaxOpenConns,
			MaxIdleConns:    cfg.ClickHouse.MaxIdleConns,
			ConnMaxLifetime: cfg.ClickHouse.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.ClickHouse.ConnMaxIdleTime,
			PingTimeout:     cfg.ClickHouse.PingTimeout,
		})
		if err != nil {
			return fmt.Errorf("clickhouse connect: %w", err)
		}
		defer func() {
			_ = chDB.Close()
		}()

		server := httpSrv.NewServer(cfg, mysqlDB, chDB, redisClient)

		errCh := make(chan error, 1)
		go func() {
			log.Printf("starting http on %s", cfg.HTTP.Addr)
			errCh <- server.Start(cfg.HTTP.Addr)
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case sig := <-sigCh:
			log.Printf("signal received: %s, shutting down...", sig)
		case err := <-errCh:
			if err != nil {
				log.Printf("http server exited: %v", err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)

		return nil
	},
}
