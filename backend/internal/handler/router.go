package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/assets"
	"github.com/smartinbox17-droid/canvas/backend/internal/auth"
	"github.com/smartinbox17-droid/canvas/backend/internal/canvas"
	"github.com/smartinbox17-droid/canvas/backend/internal/generation"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/payment"
	"github.com/smartinbox17-droid/canvas/backend/internal/protocol"
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

		mockUp := api.Group("/mock-upstream")
		{
			mockUp.POST("/v1/generations", h.handleMockUpstreamCreate)
			mockUp.GET("/v1/generations/:taskId", h.handleMockUpstreamPoll)
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

			v1.GET("/models", h.handleListLogicalModels)
			v1.GET("/payments/products", h.handleListPaymentProducts)
			v1.GET("/payments/providers", h.handleListPaymentProviders)
			v1.POST("/payments/notify/:providerId/:configId", h.handlePaymentNotify)
			v1.GET("/canvas-projects/shared/:token", h.handleGetSharedCanvas)

			authed := v1.Group("")
			authed.Use(h.requireAuth())
			{
				authed.GET("/assets", h.handleListAssets)
				authed.POST("/assets", h.handleUploadAsset)
				authed.GET("/assets/:id/file", h.handleDownloadAssetFile)
				authed.DELETE("/assets/:id", h.handleDeleteAsset)
				authed.GET("/asset-folders", h.handleListAssetFolders)
				authed.POST("/asset-folders", h.handleCreateAssetFolder)

				authed.POST("/tasks/quote", h.handleTaskQuote)
				authed.POST("/tasks", h.handleCreateTask)
				authed.GET("/tasks", h.handleListTasks)
				authed.GET("/tasks/:id", h.handleGetTask)
				authed.POST("/tasks/:id/cancel", h.handleCancelTask)
				authed.GET("/tasks/:id/text-events", h.handleTaskTextEventsSSE)

				authed.GET("/payments/orders", h.handleListPaymentOrders)
				authed.POST("/payments/orders", h.handleCreatePaymentOrder)
				authed.POST("/credits/redeem", h.handleRedeemCode)
				authed.GET("/credits/ledger", h.handleListUserLedger)

				authed.GET("/canvas-projects", h.handleListCanvasProjects)
				authed.POST("/canvas-projects", h.handleCreateCanvasProject)
				authed.GET("/canvas-projects/:id", h.handleGetCanvasProject)
				authed.DELETE("/canvas-projects/:id", h.handleDeleteCanvasProject)
				authed.PATCH("/canvas-projects/:id/operations", h.handlePatchCanvasOperations)
				authed.POST("/canvas-projects/:id/edit-lease", h.handleAcquireCanvasLease)
				authed.GET("/canvas-projects/:id/sync-status", h.handleGetCanvasSyncStatus)
				authed.GET("/canvas-projects/:id/realtime", h.handleCanvasRealtimeStream)
				authed.POST("/canvas-projects/:id/share", h.handleEnableCanvasShare)
				authed.DELETE("/canvas-projects/:id/share", h.handleDisableCanvasShare)

				authed.GET("/skills", h.handleListSkills)
				authed.POST("/skills", h.handleCreateSkill)

				authed.GET("/agent/capabilities", h.handleAgentCapabilities)
				authed.GET("/agent/runs", h.handleListAgentRuns)
				authed.POST("/agent/runs", h.handleStartAgentRun)
				authed.GET("/agent/runs/:id", h.handleGetAgentRun)
				authed.GET("/agent/runs/:id/events", h.handleAgentRunEventsSSE)
				authed.POST("/agent/runs/:id/approvals/:approvalId/decision", h.handleAgentApprovalDecision)
				authed.POST("/agent/runs/:id/undo", h.handleUndoAgentRun)
				authed.GET("/agent/memories", h.handleListAgentMemories)
				authed.POST("/agent/memories", h.handleCreateAgentMemory)

				authed.GET("/mcp/servers", h.handleListMCPServers)
				authed.POST("/mcp/servers", h.handleCreateMCPServer)

				authed.GET("/dramas", h.handleListDramas)
				authed.POST("/dramas", h.handleCreateDrama)
				authed.GET("/dramas/:id", h.handleGetDrama)
				authed.POST("/dramas/:id/start", h.handleStartDramaOrchestrator)
				authed.POST("/dramas/:id/episodes/:episodeId/jianying-draft", h.handleExportJianyingDraft)
			}

			admin := v1.Group("/admin")
			admin.Use(h.requireAuth(), h.requireAdmin())
			{
				admin.GET("/analytics/overview", h.handleAdminOverview)
				admin.GET("/users", h.handleAdminListUsers)
				admin.GET("/billing-orders", h.handleAdminListBillingOrders)
				admin.GET("/redeem-batches", h.handleAdminListRedeemBatches)
				admin.POST("/redeem-batches", h.handleAdminCreateRedeemBatch)
			}
		}
	}

	r.NoRoute(func(c *gin.Context) {
		kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, "route not found"))
	})
	return r
}

func (h *Handler) handleListCanvasProjects(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Canvas.ListProjects(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createCanvasProjectReq struct {
	Title string `json:"title"`
}

func (h *Handler) handleCreateCanvasProject(c *gin.Context) {
	user := currentUser(c)
	var req createCanvasProjectReq
	_ = c.ShouldBindJSON(&req)
	proj, err := h.app.Canvas.CreateProject(c.Request.Context(), user.ID, req.Title)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"project": proj})
}

func (h *Handler) handleGetCanvasProject(c *gin.Context) {
	user := currentUser(c)
	proj, err := h.app.Canvas.GetProject(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"project": proj})
}

func (h *Handler) handleDeleteCanvasProject(c *gin.Context) {
	user := currentUser(c)
	if err := h.app.Canvas.DeleteProject(c.Request.Context(), user.ID, c.Param("id")); err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"deleted": true})
}

type patchCanvasOpsReq struct {
	BaseRevision int64              `json:"baseRevision"`
	ClientID     string             `json:"clientId"`
	LeaseToken   string             `json:"leaseToken"`
	Operations   []canvas.Operation `json:"operations"`
}

func (h *Handler) handlePatchCanvasOperations(c *gin.Context) {
	user := currentUser(c)
	var req patchCanvasOpsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	res, err := h.app.Canvas.ApplyOperations(c.Request.Context(), canvas.ApplyOperationsInput{
		UserID:       user.ID,
		ProjectID:    c.Param("id"),
		BaseRevision: req.BaseRevision,
		ClientID:     req.ClientID,
		LeaseToken:   req.LeaseToken,
		Operations:   req.Operations,
	})
	if err != nil {
		if errors.Is(err, canvas.ErrRevisionConflict) {
			c.JSON(http.StatusConflict, kernel.Envelope[*canvas.ApplyOperationsResult]{
				Code:   409,
				Data:   res,
				Msg:    err.Error(),
				Reason: kernel.ReasonRevisionConflict,
			})
			return
		}
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, res)
}

type acquireLeaseReq struct {
	ClientID string `json:"clientId"`
}

func (h *Handler) handleAcquireCanvasLease(c *gin.Context) {
	user := currentUser(c)
	var req acquireLeaseReq
	_ = c.ShouldBindJSON(&req)
	owner, exp, err := h.app.Canvas.AcquireEditLease(c.Request.Context(), user.ID, c.Param("id"), req.ClientID)
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"leaseToken": owner, "expiresAt": exp})
}

func (h *Handler) handleGetCanvasSyncStatus(c *gin.Context) {
	user := currentUser(c)
	proj, err := h.app.Canvas.GetProject(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{
		"projectId":      proj.ID,
		"revision":       proj.Revision,
		"editLeaseOwner": proj.EditLeaseOwner,
		"updatedAt":      proj.UpdatedAt,
	})
}

func (h *Handler) handleCanvasRealtimeStream(c *gin.Context) {
	user := currentUser(c)
	proj, err := h.app.Canvas.GetProject(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	initPayload, _ := json.Marshal(gin.H{"type": "connected", "projectId": proj.ID, "revision": proj.Revision})
	_, _ = fmt.Fprintf(c.Writer, "event: presence\ndata: %s\n\n", string(initPayload))
	c.Writer.Flush()

	if c.Query("once") == "1" {
		return
	}

	ch, unsub := h.app.Canvas.Subscribe(proj.ID)
	defer unsub()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(msg)
			_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", msg.Type, string(b))
			c.Writer.Flush()
		}
	}
}

func (h *Handler) handleEnableCanvasShare(c *gin.Context) {
	user := currentUser(c)
	tok, err := h.app.Canvas.EnableShare(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"shareToken": tok})
}

func (h *Handler) handleDisableCanvasShare(c *gin.Context) {
	user := currentUser(c)
	if err := h.app.Canvas.DisableShare(c.Request.Context(), user.ID, c.Param("id")); err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"disabled": true})
}

func (h *Handler) handleGetSharedCanvas(c *gin.Context) {
	proj, err := h.app.Canvas.GetSharedProject(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.replyCanvasErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"project": proj})
}

func (h *Handler) replyCanvasErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, canvas.ErrProjectNotFound):
		kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
	case errors.Is(err, canvas.ErrForbidden):
		kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
	case errors.Is(err, canvas.ErrLeaseConflict):
		kernel.Fail(c, kernel.NewError(http.StatusConflict, 409, kernel.ReasonConflict, err.Error()))
	default:
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
	}
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

func (h *Handler) handleListLogicalModels(c *gin.Context) {
	models, err := h.app.Generation.ListLogicalModels(c.Request.Context())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": models})
}

func (h *Handler) handleListPaymentProducts(c *gin.Context) {
	items, err := h.app.Payment.ListProducts(c.Request.Context())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleListPaymentProviders(c *gin.Context) {
	items, err := h.app.Payment.ListProviders(c.Request.Context())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleListPaymentOrders(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Payment.ListUserOrders(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createPaymentOrderReq struct {
	ProductID string `json:"productId"`
	ConfigID  string `json:"configId"`
}

func (h *Handler) handleCreatePaymentOrder(c *gin.Context) {
	user := currentUser(c)
	var req createPaymentOrderReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ProductID) == "" {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "productId is required"))
		return
	}
	notifyURL := h.app.Config.PublicBaseURL + "/api/v1/payments/notify"
	returnURL := h.app.Config.PublicBaseURL + "/billing"
	order, checkout, err := h.app.Payment.CreateOrder(c.Request.Context(), user.ID, req.ProductID, req.ConfigID, notifyURL, returnURL, c.ClientIP())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"order": order, "checkout": checkout})
}

func (h *Handler) handlePaymentNotify(c *gin.Context) {
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	ack, err := h.app.Payment.HandleNotification(c.Request.Context(), c.Param("providerId"), c.Param("configId"), c.Request.Header, rawBody)
	if err != nil {
		c.String(http.StatusBadRequest, "fail: "+err.Error())
		return
	}
	c.String(http.StatusOK, ack)
}

type redeemReq struct {
	Code string `json:"code"`
}

func (h *Handler) handleRedeemCode(c *gin.Context) {
	user := currentUser(c)
	var req redeemReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "code is required"))
		return
	}
	acc, err := h.app.Payment.Redeem(c.Request.Context(), user.ID, req.Code)
	if err != nil {
		if errors.Is(err, payment.ErrInvalidRedeem) {
			kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, err.Error()))
			return
		}
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"credits": acc})
}

func (h *Handler) handleListUserLedger(c *gin.Context) {
	user := currentUser(c)
	var items []model.CreditLedger
	_ = h.app.DB.WithContext(c.Request.Context()).
		Where("user_id = ?", user.ID).
		Order("created_at DESC").
		Limit(50).
		Find(&items).Error
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleAdminOverview(c *gin.Context) {
	ov, err := h.app.Payment.GetAdminOverview(c.Request.Context())
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, ov)
}

func (h *Handler) handleAdminListUsers(c *gin.Context) {
	var users []model.User
	_ = h.app.DB.WithContext(c.Request.Context()).Order("created_at DESC").Limit(100).Find(&users).Error
	kernel.OK(c, gin.H{"items": users})
}

func (h *Handler) handleAdminListBillingOrders(c *gin.Context) {
	var orders []model.BillingOrder
	_ = h.app.DB.WithContext(c.Request.Context()).Order("created_at DESC").Limit(100).Find(&orders).Error
	kernel.OK(c, gin.H{"items": orders})
}

func (h *Handler) handleAdminListRedeemBatches(c *gin.Context) {
	var batches []model.RedeemBatch
	_ = h.app.DB.WithContext(c.Request.Context()).Order("created_at DESC").Limit(50).Find(&batches).Error
	kernel.OK(c, gin.H{"items": batches})
}

type createRedeemBatchReq struct {
	Name               string `json:"name"`
	CreditMicrocredits int64  `json:"creditMicrocredits"`
	Count              int    `json:"count"`
}

func (h *Handler) handleAdminCreateRedeemBatch(c *gin.Context) {
	user := currentUser(c)
	var req createRedeemBatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	batch, codes, err := h.app.Payment.CreateRedeemBatch(c.Request.Context(), user.ID, req.Name, req.CreditMicrocredits, req.Count)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"batch": batch, "codes": codes})
}

type taskSubmitPayload struct {
	CreationSubmissionID string                     `json:"creationSubmissionId"`
	ProjectID            string                     `json:"projectId"`
	LogicalModelCode     string                     `json:"logicalModelId"`
	Operation            string                     `json:"operation"`
	Request              protocol.GenerationRequest `json:"request"`
}

func (h *Handler) handleTaskQuote(c *gin.Context) {
	var body taskSubmitPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	modelCode := body.LogicalModelCode
	if modelCode == "" {
		modelCode = body.Request.Model
	}
	quote, err := h.app.Generation.Quote(c.Request.Context(), modelCode, body.Request)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusServiceUnavailable, 503, kernel.ReasonRouteUnavailable, err.Error()))
		return
	}
	kernel.OK(c, quote)
}

func (h *Handler) handleCreateTask(c *gin.Context) {
	user := currentUser(c)
	var body taskSubmitPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	subID := body.CreationSubmissionID
	if subID == "" {
		subID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	task, err := h.app.Generation.SubmitTask(c.Request.Context(), generation.SubmitTaskInput{
		UserID:               user.ID,
		ProjectID:            body.ProjectID,
		CreationSubmissionID: subID,
		LogicalModelCode:     body.LogicalModelCode,
		Operation:            body.Operation,
		Request:              body.Request,
	})
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrInsufficientCredits):
			kernel.Fail(c, kernel.NewError(http.StatusPaymentRequired, 402, kernel.ReasonInsufficientCredits, err.Error()))
		case errors.Is(err, generation.ErrRouteUnavailable):
			kernel.Fail(c, kernel.NewError(http.StatusServiceUnavailable, 503, kernel.ReasonRouteUnavailable, err.Error()))
		case errors.Is(err, generation.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}
	kernel.OK(c, gin.H{"task": task})
}

func (h *Handler) handleListTasks(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Generation.ListTasks(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleGetTask(c *gin.Context) {
	user := currentUser(c)
	task, err := h.app.Generation.GetTask(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		case errors.Is(err, generation.ErrTaskNotFound):
			kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}
	kernel.OK(c, gin.H{"task": task})
}

func (h *Handler) handleCancelTask(c *gin.Context) {
	user := currentUser(c)
	task, err := h.app.Generation.CancelTask(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		case errors.Is(err, generation.ErrTaskNotFound):
			kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}
	kernel.OK(c, gin.H{"task": task})
}

func (h *Handler) handleTaskTextEventsSSE(c *gin.Context) {
	user := currentUser(c)
	afterSeq := 0
	if raw := strings.TrimSpace(c.GetHeader("Last-Event-ID")); raw != "" {
		afterSeq, _ = strconv.Atoi(raw)
	} else if q := strings.TrimSpace(c.Query("afterSeq")); q != "" {
		afterSeq, _ = strconv.Atoi(q)
	}

	events, err := h.app.Generation.ListTextEventsAfter(c.Request.Context(), user.ID, c.Param("id"), afterSeq)
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrForbidden):
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
		case errors.Is(err, generation.ErrTaskNotFound):
			kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
		default:
			kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		}
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for _, ev := range events {
		_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Event, ev.DataJSON)
	}
	c.Writer.Flush()
}

func (h *Handler) handleMockUpstreamCreate(c *gin.Context) {
	var body map[string]any
	_ = c.ShouldBindJSON(&body)
	capStr, _ := body["capability"].(string)
	prompt, _ := body["prompt"].(string)
	id := "mock_up_" + uuid.NewString()[:8]

	switch capStr {
	case "image":
		c.JSON(http.StatusOK, gin.H{
			"id":     id,
			"status": "succeeded",
			"images": []string{"https://placehold.co/1024x1024/2563eb/ffffff.png?text=Mock+AI+Image"},
		})
	case "video":
		c.JSON(http.StatusOK, gin.H{
			"id":     id,
			"status": "succeeded",
			"videos": []string{"https://www.w3schools.com/html/mov_bbb.mp4"},
		})
	default:
		c.JSON(http.StatusOK, gin.H{
			"id":     id,
			"status": "succeeded",
			"text":   fmt.Sprintf("【Mock AI 创作结果】已根据提示词生成：%s", prompt),
		})
	}
}

func (h *Handler) handleMockUpstreamPoll(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":     c.Param("taskId"),
		"status": "succeeded",
	})
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

func (h *Handler) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := currentUser(c)
		if user == nil || user.Role != "admin" {
			kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, "admin role required"))
			c.Abort()
			return
		}
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
