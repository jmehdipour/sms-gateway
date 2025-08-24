package middleware

import (
	"net/http"
	"strconv"
	"time"

	echo "github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig config for Redis-based RPS limiter.
type RateLimitConfig struct {
	Redis          *redis.Client
	DefaultRPS     int           // fallback if customer_rps not set
	KeyPrefix      string        // e.g. "rl:cust:"
	Window         time.Duration // usually 1s
	RetryAfterHint bool          // set Retry-After header when limited
}

// RateLimitMiddleware applies a simple fixed-window per-customer RPS limit.
// It expects customer_id in echo.Context (set by APIKeyMiddleware).
func RateLimitMiddleware(cfg RateLimitConfig) echo.MiddlewareFunc {
	if cfg.Window <= 0 {
		cfg.Window = time.Second
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "rl:cust:"
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			v := c.Get("customer_id")
			custID, ok := v.(int64)
			if !ok || custID <= 0 {
				return next(c)
			}

			max := cfg.DefaultRPS
			if vv := c.Get("customer_rps"); vv != nil {
				if m, ok := vv.(int); ok && m > 0 {
					max = m
				}
			}
			if max <= 0 || cfg.Redis == nil {
				// no limit configured or redis missing (dev): allow
				return next(c)
			}

			// fixed-window key: rl:cust:{id}:{unix_sec}
			now := time.Now()
			key := cfg.KeyPrefix + strconv.FormatInt(custID, 10) + ":" + strconv.FormatInt(now.Unix(), 10)

			// INCR and set expiry 2*window (safety)
			pipe := cfg.Redis.Pipeline()
			cnt := pipe.Incr(c.Request().Context(), key)
			pipe.Expire(c.Request().Context(), key, cfg.Window*2)
			_, err := pipe.Exec(c.Request().Context())
			if err != nil {
				return next(c)
			}

			if cnt.Val() > int64(max) {
				if cfg.RetryAfterHint {
					// seconds until next window
					remain := cfg.Window - time.Duration(now.UnixNano()%int64(cfg.Window))
					if remain > 0 {
						c.Response().Header().Set("Retry-After", strconv.Itoa(int(remain.Round(time.Second)/time.Second)))
					}
				}
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
			}
			return next(c)
		}
	}
}
