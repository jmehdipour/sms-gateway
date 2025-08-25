package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/config"
	"github.com/jmehdipour/sms-gateway/internal/db"
	"github.com/jmehdipour/sms-gateway/internal/dispatcher"
	"github.com/jmehdipour/sms-gateway/internal/kafka"
	"github.com/jmehdipour/sms-gateway/internal/metrics"
	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmehdipour/sms-gateway/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
)

var senderCmd = &cobra.Command{
	Use:   "sender",
	Short: "Start sender worker (normal | express)",
}

var senderNormalCmd = &cobra.Command{
	Use:   "normal",
	Short: "Run sender worker for normal SMS",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSender(cmd, model.SMSTypeNormal)
	},
}

var senderExpressCmd = &cobra.Command{
	Use:   "express",
	Short: "Run sender worker for express SMS",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSender(cmd, model.SMSTypeExpress)
	},
}

func init() {
	senderCmd.AddCommand(senderNormalCmd)
	senderCmd.AddCommand(senderExpressCmd)
}

func runSender(cmd *cobra.Command, smsType model.SMSType) error {
	// 1) load config
	cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	metrics.MustRegister(prometheus.DefaultRegisterer)

	// sanity on pricing
	if cfg.Pricing.Normal <= 0 || cfg.Pricing.Express <= 0 {
		return fmt.Errorf("invalid pricing: normal=%d express=%d", cfg.Pricing.Normal, cfg.Pricing.Express)
	}

	// 2) DB connection (MySQL)
	dbx, err := db.NewMySQLConnection(cfg.MySQL.DSN, db.MySQLOpts{
		MaxOpenConns:    cfg.MySQL.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.MaxIdleConns,
		ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.MySQL.ConnMaxIdleTime,
		PingTimeout:     cfg.MySQL.PingTimeout,
	})
	if err != nil {
		return fmt.Errorf("mysql connect: %w", err)
	}
	defer dbx.Close()

	// 3) repositories (MySQL)
	messagesRepo := repository.NewMessagesRepository(dbx)
	walletRepo := repository.NewWalletRepository()
	ledgerRepo := repository.NewLedgerRepository()

	// 4) providers → dispatcher
	var provs []dispatcher.Provider
	for _, pc := range cfg.Providers {
		if !pc.Enabled || strings.TrimSpace(pc.BaseURL) == "" {
			continue
		}
		provs = append(provs,
			dispatcher.NewHTTPProvider(
				pc.Name,
				strings.TrimRight(pc.BaseURL, "/"),
				pc.NormalPath,
				pc.ExpressPath,
				pc.TimeoutMs,
				pc.Breaker.FailThreshold,
				pc.Breaker.OpenForMs,
			),
		)
	}
	if len(provs) == 0 {
		return fmt.Errorf("no providers enabled in config")
	}
	disp := dispatcher.NewDispatcher(provs, cfg.Dispatcher.MaxRetryAttempts.Express, cfg.Dispatcher.MaxRetryAttempts.Normal)

	// 5) kafka consumer
	topic := "sms.normal"
	if smsType == model.SMSTypeExpress {
		topic = "sms.express"
	}
	groupID := cfg.Kafka.GroupID
	if groupID == "" {
		groupID = "smsgw-sender"
	}
	groupID = groupID + "-" + smsType.String()

	consumer := kafka.NewConsumerFromConfig(kafka.Config{
		Brokers:        cfg.Kafka.Brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       cfg.Kafka.MinBytes,
		MaxBytes:       cfg.Kafka.MaxBytes,
		CommitInterval: time.Duration(cfg.Kafka.CommitInterval) * time.Millisecond,
	})
	defer consumer.Close()

	w := worker.NewSenderKafka(
		dbx,
		consumer,
		messagesRepo,
		walletRepo,
		ledgerRepo,
		disp,
		smsType,
		cfg.Pricing.Normal,
		cfg.Pricing.Express,
	)

	// tune knobs
	if cfg.Dispatcher.WorkerCount > 0 {
		w.Workers = cfg.Dispatcher.WorkerCount
	}
	if cfg.Dispatcher.BatchSize > 0 {
		w.BatchSize = cfg.Dispatcher.BatchSize
	}
	if cfg.Dispatcher.BatchWait > 0 {
		w.BatchWait = cfg.Dispatcher.BatchWait
	}

	// 7) graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf(">> sender started type=%s topic=%s group=%s workers=%d batchSize=%d batchWait=%s",
		smsType, topic, groupID, w.Workers, w.BatchSize, w.BatchWait)

	return w.Run(ctx)
}
