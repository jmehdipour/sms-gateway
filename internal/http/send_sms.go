package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jmehdipour/sms-gateway/internal/http/middleware"
	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmehdipour/sms-gateway/internal/service/queue"
	"github.com/jmehdipour/sms-gateway/internal/util"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

type sendReq struct {
	Phone string `json:"phone"`
	Text  string `json:"text"`
	Type  string `json:"type"` // "normal" | "express"
}

func sendSMSHandler(queueSvc *queue.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req sendReq
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
		}

		// Normalize
		req.Phone = util.NormalizePhone(strings.TrimSpace(req.Phone))
		req.Text = strings.TrimSpace(req.Text)

		// Basic validation
		if req.Phone == "" || req.Text == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
		}

		if utf8.RuneCountInString(req.Text) > 300 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "text too long"})
		}

		typ, ok := model.ParseSMSType(req.Type)
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid type"})
		}

		// auth (set by APIKeyMiddleware)
		custID, ok := middleware.CustomerIDFromCtx(c)
		if !ok || custID <= 0 {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		// enqueue (wallet reserve + ledger(reserve) + messages + outbox in one TX)
		idStr, err := queueSvc.Enqueue(c.Request().Context(), custID, model.SMS{
			Phone: req.Phone,
			Text:  req.Text,
			Type:  typ,
		})
		if err != nil {
			if errors.Is(err, queue.ErrInsufficientFunds) {
				return c.JSON(http.StatusPaymentRequired, map[string]any{
					"error":       "insufficient_funds",
					"description": "wallet balance is not enough to reserve the message cost",
					"type":        typ.String(),
					"customer_id": strconv.FormatInt(custID, 10),
				})
			}

			log.Errorf("enqueue failed: %v", err)

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "db error"})
		}

		return c.JSON(http.StatusAccepted, map[string]any{
			"enqueued":    true,
			"id":          idStr,
			"type":        typ.String(),
			"customer_id": strconv.FormatInt(custID, 10),
		})
	}
}
