package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type healthChecker interface {
	IsHealthy() bool
}

type HealthHandler struct {
	pg     pinger
	rabbit healthChecker
}

func NewHealthHandler(pg pinger, rabbit healthChecker) *HealthHandler {
	return &HealthHandler{pg: pg, rabbit: rabbit}
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.pg.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "degraded", "postgres": err.Error(),
		})
		return
	}
	if !h.rabbit.IsHealthy() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "degraded", "rabbitmq": "unhealthy",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
