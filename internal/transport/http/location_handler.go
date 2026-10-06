package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
	"fleet-management-system/internal/usecase"
)

type LocationHandler struct {
	uc  *usecase.LocationUsecase
	log zerolog.Logger
}

func NewLocationHandler(uc *usecase.LocationUsecase, log zerolog.Logger) *LocationHandler {
	return &LocationHandler{uc: uc, log: log}
}

func (h *LocationHandler) GetLatest(c *gin.Context) {
	id := c.Param("vehicle_id")
	log := LoggerFrom(c, h.log)

	loc, err := h.uc.GetLatest(c.Request.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "vehicle not found"})
	case errors.Is(err, domain.ErrValidation):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
	case err != nil:
		log.Error().Err(err).Str("vehicle", id).Msg("get latest failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	default:
		c.JSON(http.StatusOK, loc)
	}
}

type historyQuery struct {
	Start  int64 `form:"start"`
	End    int64 `form:"end"`
	Cursor int64 `form:"cursor"`
	Limit  int   `form:"limit"`
}

func (h *LocationHandler) GetHistory(c *gin.Context) {
	id := c.Param("vehicle_id")
	log := LoggerFrom(c, h.log)

	startStr := c.Query("start")
	endStr := c.Query("end")
	if startStr == "" || endStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "start and end query parameters are required",
		})
		return
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "start must be a valid unix timestamp",
		})
		return
	}
	end, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "end must be a valid unix timestamp",
		})
		return
	}

	// Optional cursor + limit.
	var cursor int64
	if s := c.Query("cursor"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			cursor = v
		}
	}
	var limit int
	if s := c.Query("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			limit = v
		}
	}

	page, err := h.uc.GetHistory(c.Request.Context(), id, start, end, cursor, limit)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Error().Err(err).Str("vehicle", id).Msg("get history failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, page)
}
