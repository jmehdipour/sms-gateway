package http

import (
	"net/http"
	"strings"

	"github.com/jmehdipour/sms-gateway/internal/http/middleware"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmoiron/sqlx"
	echo "github.com/labstack/echo/v4"
)

type topupReq struct {
	Amount    int64  `json:"amount"`
	RequestID string `json:"request_id"`
}

// TopupHandler : wallet topup endpoint (idempotent).
func TopupHandler(db *sqlx.DB, wallet repository.WalletRepository, ledger repository.LedgerRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		customerID, ok := middleware.CustomerIDFromCtx(c)
		if !ok || customerID <= 0 {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		var req topupReq
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
		}

		req.RequestID = strings.TrimSpace(req.RequestID)
		if req.Amount <= 0 || req.RequestID == "" || len(req.RequestID) > 128 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		}

		idem := "topup-" + req.RequestID

		tx, err := db.BeginTxx(c.Request().Context(), nil)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}
		defer func() { _ = tx.Rollback() }()

		if err := wallet.UpsertAccount(c.Request().Context(), tx, customerID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		exists, err := ledger.ExistsByIdem(c.Request().Context(), tx, idem)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		if exists {
			if err := tx.Commit(); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
			}

			return c.JSON(http.StatusOK, map[string]any{
				"topup":       true,
				"idempotent":  true,
				"amount":      req.Amount,
				"customer_id": customerID,
				"request_id":  req.RequestID,
			})
		}

		if err := ledger.InsertTopup(c.Request().Context(), tx, customerID, req.Amount, idem); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		if err := wallet.Topup(c.Request().Context(), tx, customerID, req.Amount); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		if err := tx.Commit(); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"topup":       true,
			"idempotent":  false,
			"amount":      req.Amount,
			"customer_id": customerID,
			"request_id":  req.RequestID,
		})
	}
}
