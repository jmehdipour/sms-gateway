package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jmehdipour/sms-gateway/internal/http/middleware"
	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmehdipour/sms-gateway/internal/repository"
	"github.com/jmehdipour/sms-gateway/internal/util"
	echo "github.com/labstack/echo/v4"
)

func listMessagesHandler(chRepo repository.CHMessagesRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		custID, ok := middleware.CustomerIDFromCtx(c)
		if !ok || custID <= 0 {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		limit := 50
		offset := 0
		if v := c.QueryParam("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}
		if v := c.QueryParam("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		var st model.MessageStatus
		if raw := strings.TrimSpace(c.QueryParam("status")); raw != "" {
			tmp := model.MessageStatus(raw)
			if tmp.Valid() {
				st = tmp
			}
		}

		phone := util.NormalizePhone(strings.TrimSpace(c.QueryParam("phone")))

		msgs, err := chRepo.ListByCustomer(
			c.Request().Context(),
			custID,
			phone,
			st,
			limit,
			offset,
		)
		if err != nil {
			c.Logger().Errorf("clickhouse list failed: %v", err)

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "query failed"})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"limit":   limit,
			"offset":  offset,
			"count":   len(msgs),
			"results": msgs,
		})
	}
}
