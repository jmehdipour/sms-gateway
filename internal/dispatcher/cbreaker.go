package dispatcher

import (
	"sync"
	"time"
)

type state int

const (
	closed state = iota
	open
	halfOpen
)

type MicroBreaker struct {
	mu               sync.Mutex
	st               state
	consecutiveFails int
	failThreshold    int
	openFor          time.Duration
	nextTryAt        time.Time
	probeInFlight    bool
}

func NewMicroBreaker(threshold int, openFor time.Duration) *MicroBreaker {
	return &MicroBreaker{failThreshold: threshold, openFor: openFor}
}

func (b *MicroBreaker) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.st {
	case closed:
		return true
	case open:
		return time.Now().After(b.nextTryAt) && !b.probeInFlight
	case halfOpen:
		return !b.probeInFlight
	default:
		return true
	}
}

func (b *MicroBreaker) TryAcquire() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	switch b.st {
	case closed:
		return true
	case open:
		if now.After(b.nextTryAt) && !b.probeInFlight {
			b.st = halfOpen
			b.probeInFlight = true
			return true
		}
		return false
	case halfOpen:
		if !b.probeInFlight {
			b.probeInFlight = true
			return true
		}
		return false
	default:
		return true
	}
}

func (b *MicroBreaker) OnSuccess() {
	b.mu.Lock()
	b.consecutiveFails = 0
	b.st = closed
	b.probeInFlight = false
	b.mu.Unlock()
}

func (b *MicroBreaker) OnFailure() {
	b.mu.Lock()
	if b.st == halfOpen {
		b.st = open
		b.nextTryAt = time.Now().Add(b.openFor)
		b.probeInFlight = false
		b.mu.Unlock()
		return
	}

	b.consecutiveFails++
	if b.consecutiveFails >= b.failThreshold {
		b.st = open
		b.nextTryAt = time.Now().Add(b.openFor)
	}

	b.mu.Unlock()
}
