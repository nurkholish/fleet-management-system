package http

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func NewRouter(
	lh *LocationHandler,
	hh *HealthHandler,
	gh *GeofenceHandler,
	log zerolog.Logger,
	env string,
) *gin.Engine {
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestLogger(log))
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Next()
	})

	r.GET("/healthz", hh.Live)
	r.GET("/readyz", hh.Ready)
	r.GET("/geofences", gh.List)

	v := r.Group("/vehicles/:vehicle_id")
	{
		v.GET("/location", lh.GetLatest)
		v.GET("/history", lh.GetHistory)
	}
	return r
}
