package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/dispatcher"
	"github.com/jmehdipour/sms-gateway/internal/kafka"
	"github.com/jmehdipour/sms-gateway/internal/metrics"
	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmoiron/sqlx"
)

// SenderKafka:
// - fetches envelopes from Kafka,
// - dispatches SMS via providers,
// - batches wallet/ledger/messages updates atomically (Ledger-first).
type SenderKafka struct {
	// Dependencies
	DB       *sqlx.DB
	Consumer *kafka.Consumer
	Messages repository.MessagesRepository
	Wallet   repository.WalletRepository
	Ledger   repository.LedgerRepository
	Dispatch *dispatcher.Dispatcher

	// Behavior
	Type         model.SMSType // normal | express (topic-bound worker)
	Workers      int           // number of goroutines processing messages
	BatchSize    int           // max buffered updates per flush (items)
	BatchWait    time.Duration // max time to wait before flush
	PriceNormal  int64
	PriceExpress int64
}

// NewSenderKafka builds a worker with sane defaults.
func NewSenderKafka(
	db *sqlx.DB,
	consumer *kafka.Consumer,
	msgRepo repository.MessagesRepository,
	walletRepo repository.WalletRepository,
	ledgerRepo repository.LedgerRepository,
	dispatch *dispatcher.Dispatcher,
	lane model.SMSType,
	priceNormal, priceExpress int64,
) *SenderKafka {
	return &SenderKafka{
		DB:           db,
		Consumer:     consumer,
		Messages:     msgRepo,
		Wallet:       walletRepo,
		Ledger:       ledgerRepo,
		Dispatch:     dispatch,
		Type:         lane,
		Workers:      64,
		BatchSize:    200,
		BatchWait:    300 * time.Millisecond,
		PriceNormal:  priceNormal,
		PriceExpress: priceExpress,
	}
}

func (w *SenderKafka) priceOf(t model.SMSType) int64 {
	if t == model.SMSTypeExpress {
		return w.PriceExpress
	}
	return w.PriceNormal
}

// Run starts the worker and blocks until ctx is cancelled.
func (w *SenderKafka) Run(ctx context.Context) error {
	if !w.Type.Valid() {
		return errors.New("sender-kafka: invalid SMS type")
	}
	if w.Workers <= 0 {
		w.Workers = 64
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 200
	}
	if w.BatchWait <= 0 {
		w.BatchWait = 300 * time.Millisecond
	}
	if w.PriceNormal <= 0 || w.PriceExpress <= 0 {
		return errors.New("sender-kafka: invalid pricing")
	}

	// Channel for worker results → batch writer
	updates := make(chan updateItem, w.BatchSize*2)
	defer close(updates)

	// Start batch writer
	go w.runBatchWriter(ctx, updates)

	// Fetch loop → fan-out to processors
	msgCh := make(chan kafka.Message, w.Workers*2)

	// Fetcher goroutine
	go func() {
		defer close(msgCh)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				m, err := w.Consumer.Fetch(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[sender] kafka fetch err: %v", err)
					time.Sleep(200 * time.Millisecond)
					continue
				}
				msgCh <- m
			}
		}
	}()

	// Start processors
	for i := 0; i < w.Workers; i++ {
		go w.runProcessor(ctx, i, msgCh, updates)
	}

	// Block until shutdown
	<-ctx.Done()
	return nil
}

type updateItem struct {
	id         string
	customerID int64
	amount     int64
	status     model.MessageStatus // sent | failed
}

// runProcessor parses envelope, dispatches SMS, emits update, commits Kafka.
func (w *SenderKafka) runProcessor(ctx context.Context, id int, in <-chan kafka.Message, out chan<- updateItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-in:
			if !ok {
				return
			}
			w.processOne(ctx, m, out)
		}
	}
}

func (w *SenderKafka) processOne(ctx context.Context, m kafka.Message, out chan<- updateItem) {
	// Parse envelope: { id, user_id, sms:{phone,text,type} }
	var env model.Envelope
	if err := json.Unmarshal(m.Value, &env); err != nil || env.ID == "" {
		_ = w.Consumer.Commit(ctx, m) // poison → commit, skip
		if err != nil {
			log.Printf("[sender] bad envelope json: %v", err)
		} else {
			log.Printf("[sender] envelope missing id")
		}
		return
	}

	// Normalize type with worker lane if missing
	if !env.SMS.Type.Valid() {
		env.SMS.Type = w.Type
	}
	// Compute price
	price := w.priceOf(env.SMS.Type)

	// Dispatch (providers handle their own internal strategy)
	var derr error
	switch env.SMS.Type {
	case model.SMSTypeExpress:
		derr = w.Dispatch.SendExpress(ctx, env.SMS)
	default:
		derr = w.Dispatch.SendNormal(ctx, env.SMS)
	}

	if derr == nil {
		metrics.MessagesTotal.WithLabelValues("sent", env.SMS.Type.String()).Inc()
		out <- updateItem{id: env.ID, customerID: env.UserID, amount: price, status: model.StatusSent}
	} else {
		metrics.MessagesTotal.WithLabelValues("failed", env.SMS.Type.String()).Inc()
		out <- updateItem{id: env.ID, customerID: env.UserID, amount: price, status: model.StatusFailed}
	}

	// Always commit (at-least-once; idempotency is handled in DB layer)
	if err := w.Consumer.Commit(ctx, m); err != nil {
		log.Printf("[sender] commit err: %v", err)
	}
}

// runBatchWriter does size/time-based flush of DB updates (wallet + ledger + messages) atomically.
func (w *SenderKafka) runBatchWriter(ctx context.Context, in <-chan updateItem) {
	tick := time.NewTicker(w.BatchWait)
	defer tick.Stop()

	var success, failed []updateItem

	reset := func() {
		success = success[:0]
		failed = failed[:0]
	}

	flush := func() {
		if len(success) == 0 && len(failed) == 0 {
			return
		}

		// Build per-customer deltas
		deltaMap := make(map[int64]repository.WalletDelta, 128)
		for _, it := range success {
			d := deltaMap[it.customerID]
			d.CustomerID = it.customerID
			d.DecReserved += it.amount // capture reduces reserved
			// no inc_balance for captures
			deltaMap[it.customerID] = d
		}
		for _, it := range failed {
			d := deltaMap[it.customerID]
			d.CustomerID = it.customerID
			d.DecReserved += it.amount // refund also reduces reserved
			d.IncBalance += it.amount  // refund increases balance
			deltaMap[it.customerID] = d
		}

		deltas := make([]repository.WalletDelta, 0, len(deltaMap))
		for _, d := range deltaMap {
			deltas = append(deltas, d)
		}

		// Build ledger rows
		toCap := make([]repository.LedgerRow, 0, len(success))
		for _, it := range success {
			toCap = append(toCap, repository.LedgerRow{
				CustomerID: it.customerID,
				Amount:     it.amount,
				MessageID:  it.id,
			})
		}
		toRef := make([]repository.LedgerRow, 0, len(failed))
		for _, it := range failed {
			toRef = append(toRef, repository.LedgerRow{
				CustomerID: it.customerID,
				Amount:     it.amount,
				MessageID:  it.id,
			})
		}

		// Gather message IDs
		sentIDs := make([]string, 0, len(success))
		for _, it := range success {
			sentIDs = append(sentIDs, it.id)
		}
		failedIDs := make([]string, 0, len(failed))
		for _, it := range failed {
			failedIDs = append(failedIDs, it.id)
		}

		// Single TX: ledger (cap/ref) + wallet deltas + messages status
		tx, err := w.DB.BeginTxx(ctx, nil)
		if err != nil {
			log.Printf("[sender] begin tx err: %v", err)
			reset()
			return
		}
		defer func() { _ = tx.Rollback() }()

		// 1) Ledger (idempotent inserts)
		if err := w.Ledger.InsertCaptureBatch(ctx, tx, toCap); err != nil {
			log.Printf("[sender] ledger capture batch err: %v", err)
			return
		}
		if err := w.Ledger.InsertRefundBatch(ctx, tx, toRef); err != nil {
			log.Printf("[sender] ledger refund batch err: %v", err)
			return
		}

		// 2) Wallet batch apply (reserved-=all, balance+=refunds)
		if err := w.Wallet.BatchApplySums(ctx, tx, deltas); err != nil {
			log.Printf("[sender] wallet batch apply err: %v", err)
			return
		}

		// 3) Messages status updates
		if len(sentIDs) > 0 {
			if err := w.Messages.BatchUpdateStatus(ctx, tx, sentIDs, model.StatusSent); err != nil {
				log.Printf("[sender] batch update sent err: %v", err)
				return
			}
		}
		if len(failedIDs) > 0 {
			if err := w.Messages.BatchUpdateStatus(ctx, tx, failedIDs, model.StatusFailed); err != nil {
				log.Printf("[sender] batch update failed err: %v", err)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[sender] tx commit err: %v", err)
			return
		}

		log.Printf("[sender:%s] flushed: sent=%d failed=%d customers=%d",
			w.Type, len(sentIDs), len(failedIDs), len(deltas))

		reset()
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return

		case u, ok := <-in:
			if !ok {
				flush()
				return
			}
			if u.status == model.StatusSent {
				success = append(success, u)
			} else if u.status == model.StatusFailed {
				failed = append(failed, u)
			}

			if len(success)+len(failed) >= w.BatchSize {
				flush()
			}

		case <-tick.C:
			flush()
		}
	}
}
