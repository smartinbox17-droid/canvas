package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smartinbox17-droid/canvas/backend/internal/assets"
	"github.com/smartinbox17-droid/canvas/backend/internal/auth"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/service"
)

const ctxUserKey = "canvas_current_user"

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
			authGroup := v1.Group("/auth")
			{
				authGroup.GET("/settings", h.handleAuthSettings)
				authGroup.GET("/session", h.handleAuthSession)
				authGroup.POST("/register", h.handleRegister)
				authGroup.POST("/login", h.handleLogin)
				authGroup.POST("/logout", h.handleLogout)
			}

			authed := v1.Group("")
			authed.Use(h.requireAuth())
			{
				authed.GET("/assets", h.handleListAssets)
				authed.POST("/assets", h.handleUploadAsset)
				authed.GET("/assets/:id/file", h.handleDownloadAssetFile)
				authed.DELETE("/assets/:id", h.handleDeleteAsset)
				authed.GET("/asset-folders", h.handleListAssetFolders)
				authed.POST("/asset-folders", h.handleCreateAssetFolder)
			}
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
	tok := h.extractSessionToken(c)
	user, _, err := h.app.Auth.AuthenticateToken(c.Request.Context(), tok)
	if err != nil || user == nil {
		kernel.OK(c, gin.H{
			"authenticated": false,
			"user":          nil,
		})
		return
	}
	acc, _ := h.app.Auth.GetCreditAccount(c.Request.Context(), user.ID)
	kernel.OK(c, gin.H{
		"authenticated": true,
		"user":          user,
		"credits":       acc,
	})
}

type authCredentialsReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) handleRegister(c *gin.Context) {
	if !h.app.Config.RegistrationEnabled {
		kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, "registration is disabled"))
		return
	}
	var req authCredentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	user, rawToken, err := h.app.Auth.Register(c.Request.Context(), req.Email, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			kernel.Fail(c, kernel.NewError(http.StatusConflict, 409, kernel.ReasonConflict, err.Error()))
			return
		}
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, err.Error()))
		return
	}
	h.setSessionCookie(c, rawToken)
	acc, _ := h.app.Auth.GetCreditAccount(c.Request.Context(), user.ID)
	kernel.OK(c, gin.H{"user": user, "credits": acc})
}

func (h *Handler) handleLogin(c *gin.Context) {
	var req authCredentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	user, rawToken, err := h.app.Auth.Login(c.Request.Context(), req.Email, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusUnauthorized, 401, kernel.ReasonUnauthorized, err.Error()))
		return
	}
	h.setSessionCookie(c, rawToken)
	acc, _ := h.app.Auth.GetCreditAccount(c.Request.Context(), user.ID)
	kernel.OK(c, gin.H{"user": user, "credits": acc})
}

func (h *Handler) handleLogout(c *gin.Context) {
	tok := h.extractSessionToken(c)
	_ = h.app.Auth.Logout(c.Request.Context(), tok)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, "", -1, "/", "", false, true)
	kernel.OK(c, gin.H{"loggedOut": true})
}

func (h *Handler) handleListAssetFolders(c *gin.Context) {
	user := currentUser(c)
	folders, err := h.app.Assets.ListFolders(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": folders})
}

type createFolderReq struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
}

func (h *Handler) handleCreateAssetFolder(c *gin.Context) {
	user := currentUser(c)
	var req createFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	folder, err := h.app.Assets.CreateFolder(c.Request.Context(), user.ID, req.Name, req.ParentID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, folder)
}

func (h *Handler) handleListAssets(c *gin.Context) {
	user := currentUser(c)
	var folderID *string
	if f := strings.TrimSpace(c.Query("folderId")); f != "" {
		folderID = &f
	}
	kind := strings.TrimSpace(c.Query("kind"))
	items, err := h.app.Assets.ListAssets(c.Request.Context(), user.ID, folderID, kind)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleUploadAsset(c *gin.Context) {
	user := currentUser(c)
	fh, err := c.FormFile("file")
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "multipart file field is required"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, err.Error()))
		return
	}
	defer func() { _ = f.Close() }()

	var folderID *string
	if rawFolder := strings.TrimSpace(c.PostForm("folderId")); rawFolder != "" {
		folderID = &rawFolder
	}
	asset, err := h.app.Assets.Upload(c.Request.Context(), user.ID, fh.Filename, folderID, f)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, asset)
}

func (h *Handler) handleDownloadAssetFile(c *gin.Context) {
	user := currentUser(c)
	asset, rc, err := h.app.Assets.OpenAssetFile(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, assets.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		case errors.Is(err, assets.ErrAssetNotFound):
			kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}
	defer func() { _ = rc.Close() }()
	c.Header("Content-Type", asset.MimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

func (h *Handler) handleDeleteAsset(c *gin.Context) {
	user := currentUser(c)
	err := h.app.Assets.DeleteAsset(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, assets.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		case errors.Is(err, assets.ErrAssetNotFound):
			kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
		case errors.Is(err, assets.ErrAssetInUse):
			kernel.Fail(c, kernel.NewError(http.StatusConflict, 409, kernel.ReasonAssetInUse, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}
	kernel.OK(c, gin.H{"deleted": true})
}

func (h *Handler) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := h.extractSessionToken(c)
		user, _, err := h.app.Auth.AuthenticateToken(c.Request.Context(), tok)
		if err != nil || user == nil {
			kernel.Fail(c, kernel.NewError(http.StatusUnauthorized, 401, kernel.ReasonUnauthorized, "authentication required"))
			c.Abort()
			return
		}
		c.Set(ctxUserKey, user)
		c.Next()
	}
}

func currentUser(c *gin.Context) *model.User {
	val, _ := c.Get(ctxUserKey)
	u, _ := val.(*model.User)
	return u
}

func (h *Handler) extractSessionToken(c *gin.Context) string {
	if cookie, err := c.Cookie(auth.SessionCookieName); err == nil && strings.TrimSpace(cookie) != "" {
		return strings.TrimSpace(cookie)
	}
	return ""
}

func (h *Handler) setSessionCookie(c *gin.Context, rawToken string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, rawToken, int(auth.SessionTTL.Seconds()), "/", "", false, true)
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
