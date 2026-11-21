package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/service"
)

type Handler struct {
	app *service.Application
}

func New(app *service.Application) *Handler {
	return &Handler{app: app}
}

func (h *Handler) NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), h.corsMiddleware())

	api := r.Group("/api")
	{
		health := api.Group("/health")
		{
			health.GET("/live", h.handleLive)
			health.GET("/startup", h.handleStartup)
			health.GET("/ready", h.handleReady)
		}

		v1 := api.Group("/v1")
		{
			v1.GET("/auth/settings", h.handleAuthSettings)
			v1.GET("/auth/session", h.handleAuthSession)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, "route not found"))
	})
	return r
}

func (h *Handler) handleLive(c *gin.Context) {
	kernel.OK(c, gin.H{"status": "alive"})
}

func (h *Handler) handleStartup(c *gin.Context) {
	if !h.app.IsReady() {
		kernel.Fail(c, kernel.NewError(http.StatusServiceUnavailable, 503, kernel.ReasonServiceNotReady, "application is starting"))
		return
	}
	kernel.OK(c, gin.H{"status": "started"})
}

func (h *Handler) handleReady(c *gin.Context) {
	status, err := h.app.CheckReadiness(c.Request.Context())
	if err != nil || !h.app.IsReady() {
		kernel.Fail(c, kernel.NewError(http.StatusServiceUnavailable, 503, kernel.ReasonServiceNotReady, status.Status))
		return
	}
	kernel.OK(c, status)
}

func (h *Handler) handleAuthSettings(c *gin.Context) {
	kernel.OK(c, gin.H{
		"registrationEnabled": h.app.Config.RegistrationEnabled,
		"oidcEnabled":         false,
		"turnstileEnabled":    false,
	})
}

func (h *Handler) handleAuthSession(c *gin.Context) {
	kernel.OK(c, gin.H{
		"authenticated": false,
		"user":          nil,
	})
}

func (h *Handler) corsMiddleware() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(h.app.Config.CORSOrigins))
	for _, o := range h.app.Config.CORSOrigins {
		allowed[strings.TrimRight(o, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}
