package dispatcher

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jmehdipour/sms-gateway/internal/model"
)

type Config struct {
	AttemptsExpress int // e.g. 3
	AttemptsNormal  int // e.g. 2
}

var (
	ErrNoHealthy = fmt.Errorf("no healthy providers")
	ErrNoAcquire = fmt.Errorf("provider not acquired")
)

type Dispatcher struct {
	providers          []Provider
	roundRobinCounter  atomic.Uint64
	maxAttemptsNormal  int
	maxAttemptsExpress int
}

func NewDispatcher(provs []Provider, maxAttemptsExpress, maxAttemptsNormal int) *Dispatcher {
	if maxAttemptsExpress < 1 {
		maxAttemptsExpress = 3
	}

	if maxAttemptsNormal < 1 {
		maxAttemptsNormal = 2
	}

	return &Dispatcher{providers: provs, maxAttemptsExpress: maxAttemptsExpress, maxAttemptsNormal: maxAttemptsNormal}
}

func (d *Dispatcher) selectProvider() (Provider, error) {
	healthy := make([]Provider, 0, len(d.providers))
	for _, p := range d.providers {
		if p.Ready() {
			healthy = append(healthy, p)
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthy
	}

	x := d.roundRobinCounter.Add(1)
	idx := int((x - 1) % uint64(len(healthy)))

	return healthy[idx], nil
}

func (d *Dispatcher) tryOnce(ctx context.Context, sms model.SMS, express bool) error {
	p, err := d.selectProvider()
	if err != nil {
		return err
	}

	if !p.Acquire() {
		return ErrNoAcquire
	}

	if express {
		return p.SendExpress(ctx, sms)
	}

	return p.SendNormal(ctx, sms)
}

func (d *Dispatcher) SendExpress(ctx context.Context, sms model.SMS) error {
	var last error
	for i := 0; i < d.maxAttemptsExpress; i++ {
		if err := d.tryOnce(ctx, sms, true); err == nil {
			return nil
		} else {
			last = err
		}
	}

	if last == nil {
		last = fmt.Errorf("send express failed")
	}

	return last
}

func (d *Dispatcher) SendNormal(ctx context.Context, sms model.SMS) error {
	var last error
	for i := 0; i < d.maxAttemptsNormal; i++ {
		if err := d.tryOnce(ctx, sms, false); err == nil {
			return nil
		} else {
			last = err
		}
	}
	
	if last == nil {
		last = fmt.Errorf("send normal failed")
	}

	return last
}
