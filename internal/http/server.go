package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/config"
	"github.com/jmehdipour/sms-gateway/internal/http/middleware"
	"github.com/jmehdipour/sms-gateway/internal/metrics"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmehdipour/sms-gateway/internal/service/queue"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	echoMid "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type Server struct{ e *echo.Echo }

func NewServer(cfg config.Config, mysqlDB, clickhouseDB *sqlx.DB, rds *redis.Client) *Server {
	// repos (MySQL)
	customersRepo := repository.NewCustomersRepository(mysqlDB)
	messagesRepo := repository.NewMessagesRepository(mysqlDB)
	outboxRepo := repository.NewOutboxRepository(mysqlDB)
	walletRepo := repository.NewWalletRepository()
	ledgerRepo := repository.NewLedgerRepository()

	// repos (ClickHouse)
	chMessagesRepo := repository.NewCHMessagesRepository(clickhouseDB)

	// services
	queueSvc := queue.New(
		mysqlDB,
		messagesRepo,
		outboxRepo,
		walletRepo,
		ledgerRepo,
		cfg.Pricing.Normal,
		cfg.Pricing.Express,
	)

	// echo
	e := echo.New()
	e.HideBanner = true
	e.Use(echoMid.Recover(), echoMid.Logger())

	metrics.MustRegister(prometheus.DefaultRegisterer)

	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// health
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	// middlewares
	authMW := middleware.APIKeyMiddleware(customersRepo)
	rlMW := middleware.RateLimitMiddleware(middleware.RateLimitConfig{
		Redis:          rds,
		DefaultRPS:     cfg.RateLimit.RPS,
		KeyPrefix:      "rl:cust:",
		Window:         time.Second,
		RetryAfterHint: true,
	})

	// routes
	v1 := e.Group("/v1", authMW, rlMW)
	v1.POST("/sms/send", sendSMSHandler(queueSvc))
	v1.GET("/reports/messages", listMessagesHandler(chMessagesRepo))
	v1.POST("/wallet/topup", TopupHandler(mysqlDB, walletRepo, ledgerRepo))

	return &Server{e: e}
}

func (s *Server) Start(addr string) error {
	log.Printf("http: listening on %s", addr)
	return s.e.Start(addr)
}
func (s *Server) Shutdown(ctx context.Context) error { return s.e.Shutdown(ctx) }
