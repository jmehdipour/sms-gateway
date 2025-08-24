package middleware

import (
	"net/http"
	"strings"

	"github.com/jmehdipour/sms-gateway/internal/repository"
	echo "github.com/labstack/echo/v4"
)

type ctxKey int

const ctxCustomerID ctxKey = 1

// CustomerIDFromCtx extracts authenticated customer_id set by APIKeyMiddleware.
func CustomerIDFromCtx(c echo.Context) (int64, bool) {
	v := c.Get("customer_id")
	id, ok := v.(int64)
	return id, ok
}

// APIKeyMiddleware authenticates requests using X-API-Key header.
// On success it stores customer_id in context and optionally blocks suspended accounts.
func APIKeyMiddleware(customers repository.CustomersRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := strings.TrimSpace(c.Request().Header.Get("X-API-Key"))
			if key == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing api key"})
			}
			cu, err := customers.GetByAPIKey(c.Request().Context(), key)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "auth error"})
			}
			if cu == nil || cu.Status != "active" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid api key"})
			}
			c.Set("customer_id", cu.ID)
			if cu.RateLimitRPS != nil {
				c.Set("customer_rps", *cu.RateLimitRPS)
			}
			return next(c)
		}
	}
}
