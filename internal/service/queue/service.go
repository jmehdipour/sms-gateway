package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmehdipour/sms-gateway/internal/util"
	"github.com/jmoiron/sqlx"
)

const (
	NormalSMSKafkaTopic  = "sms.normal"
	ExpressSMSKafkaTopic = "sms.express"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

// Service atomically persists messages, wallet reserve, ledger events, and outbox events.
type Service struct {
	db     *sqlx.DB
	msgs   repository.MessagesRepository
	outbox repository.OutboxRepository
	wallet repository.WalletRepository
	ledger repository.LedgerRepository

	priceNormal  int64
	priceExpress int64
}

// New constructs the queue service.
func New(
	db *sqlx.DB,
	messagesRepo repository.MessagesRepository,
	outboxRepo repository.OutboxRepository,
	walletRepo repository.WalletRepository,
	ledgerRepo repository.LedgerRepository,
	priceNormal int64,
	priceExpress int64,
) *Service {
	return &Service{
		db:           db,
		msgs:         messagesRepo,
		outbox:       outboxRepo,
		wallet:       walletRepo,
		ledger:       ledgerRepo,
		priceNormal:  priceNormal,
		priceExpress: priceExpress,
	}
}

func (s *Service) priceOf(t model.SMSType) int64 {
	if t == model.SMSTypeExpress {
		return s.priceExpress
	}
	return s.priceNormal
}

// Enqueue validates the SMS, reserves wallet funds, generates a ULID,
// and writes into `wallet_ledger(reserve)`, `messages` and `outbox` within a single transaction.
// Returns the generated message ID.
func (s *Service) Enqueue(ctx context.Context, customerID int64, sms model.SMS) (string, error) {
	// Generate message ID (ULID)
	msgID := util.New()

	// Normalize and build the message row
	msg := model.Message{
		ID:         msgID,
		CustomerID: customerID,
		Phone:      sms.Phone,
		Text:       sms.Text,
		Type:       sms.Type,
		Status:     model.StatusQueued,
	}

	// Outbox envelope
	env := model.Envelope{
		ID:     msgID,
		UserID: customerID,
		SMS:    sms,
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("marshal envelope: %w", err)
	}

	price := s.priceOf(sms.Type)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.wallet.UpsertAccount(ctx, tx, customerID); err != nil {
		return "", fmt.Errorf("wallet upsert: %w", err)
	}

	bal, _, err := s.wallet.GetForUpdate(ctx, tx, customerID)
	if err != nil {
		return "", fmt.Errorf("wallet get for update: %w", err)
	}
	
	if bal < price {
		return "", ErrInsufficientFunds
	}

	if err := s.wallet.Adjust(ctx, tx, customerID, -price, +price); err != nil {
		return "", fmt.Errorf("wallet reserve adjust: %w", err)
	}

	if err := s.ledger.InsertReserve(ctx, tx, customerID, price, msgID, "reserve-"+msgID); err != nil {
		return "", fmt.Errorf("ledger reserve: %w", err)
	}

	if err := s.msgs.InsertQueued(ctx, tx, msg); err != nil {
		return "", fmt.Errorf("insert message queued: %w", err)
	}

	topic := NormalSMSKafkaTopic
	if sms.Type == model.SMSTypeExpress {
		topic = ExpressSMSKafkaTopic
	}

	if err := s.outbox.Insert(ctx, tx, "message", msgID, topic, payload); err != nil {
		return "", fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return msgID, nil
}
