package util

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// New generates a new ULID string
func New() string {
	t := time.Now()
	entropy := ulid.Monotonic(rand.Reader, 0)

	return ulid.MustNew(ulid.Timestamp(t), entropy).String()
}
