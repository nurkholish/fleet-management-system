package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
)

type GeofenceHandler struct {
	fences []domain.Geofence
	log    zerolog.Logger
}

func NewGeofenceHandler(fences []domain.Geofence, log zerolog.Logger) *GeofenceHandler {
	return &GeofenceHandler{fences: fences, log: log}
}

func (h *GeofenceHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.fences)
}
