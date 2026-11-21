package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smartinbox17-droid/canvas/backend/internal/agent"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/skills"
)

func (h *Handler) handleListSkills(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Skills.ListSkills(c.Request.Context(), user.ID, c.Query("category"))
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createSkillReq struct {
	Name         string            `json:"name"`
	Category     string            `json:"category"`
	OutputMode   string            `json:"outputMode"`
	Description  string            `json:"description"`
	SystemPrompt string            `json:"systemPrompt"`
	Files        map[string]string `json:"files"`
}

func (h *Handler) handleCreateSkill(c *gin.Context) {
	user := currentUser(c)
	var req createSkillReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	sk, err := h.app.Skills.CreateCustomSkill(c.Request.Context(), skills.CreateSkillInput{
		OwnerID:      user.ID,
		Name:         req.Name,
		Category:     req.Category,
		OutputMode:   req.OutputMode,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
		Files:        req.Files,
	})
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"skill": sk})
}

func (h *Handler) handleAgentCapabilities(c *gin.Context) {
	kernel.OK(c, gin.H{"tools": h.app.Agent.ListCapabilities()})
}

type startAgentRunReq struct {
	ConversationID  string `json:"conversationId"`
	ProjectID       string `json:"projectId"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	RequireApproval bool   `json:"requireApproval"`
}

func (h *Handler) handleStartAgentRun(c *gin.Context) {
	user := currentUser(c)
	var req startAgentRunReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "prompt is required"))
		return
	}
	run, approval, err := h.app.Agent.StartRun(c.Request.Context(), agent.StartRunInput{
		UserID:          user.ID,
		ConversationID:  req.ConversationID,
		ProjectID:       req.ProjectID,
		Prompt:          req.Prompt,
		Model:           req.Model,
		RequireApproval: req.RequireApproval,
	})
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"run": run, "approval": approval})
}

func (h *Handler) handleListAgentRuns(c *gin.Context) {
	user := currentUser(c)
	runs, err := h.app.Agent.ListRuns(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	approvals, _ := h.app.Agent.ListPendingApprovals(c.Request.Context(), user.ID, "")
	kernel.OK(c, gin.H{"items": runs, "pendingApprovals": approvals})
}

func (h *Handler) handleGetAgentRun(c *gin.Context) {
	user := currentUser(c)
	run, err := h.app.Agent.GetRun(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyAgentErr(c, err)
		return
	}
	approvals, _ := h.app.Agent.ListPendingApprovals(c.Request.Context(), user.ID, run.ID)
	kernel.OK(c, gin.H{"run": run, "pendingApprovals": approvals})
}

func (h *Handler) handleAgentRunEventsSSE(c *gin.Context) {
	user := currentUser(c)
	afterSeq := 0
	if raw := strings.TrimSpace(c.GetHeader("Last-Event-ID")); raw != "" {
		afterSeq, _ = strconv.Atoi(raw)
	} else if q := strings.TrimSpace(c.Query("afterSeq")); q != "" {
		afterSeq, _ = strconv.Atoi(q)
	}
	events, err := h.app.Agent.ListRunEvents(c.Request.Context(), user.ID, c.Param("id"), afterSeq)
	if err != nil {
		h.replyAgentErr(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for _, ev := range events {
		_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, ev.PayloadJSON)
	}
	c.Writer.Flush()
}

type approvalDecisionReq struct {
	Decision   string         `json:"decision"` // approve | reject
	Remember   string         `json:"remember,omitempty"`
	EditedArgs map[string]any `json:"editedArgs,omitempty"`
}

func (h *Handler) handleAgentApprovalDecision(c *gin.Context) {
	user := currentUser(c)
	var req approvalDecisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	run, err := h.app.Agent.ResolveApproval(c.Request.Context(), user.ID, c.Param("id"), c.Param("approvalId"), req.Decision, req.EditedArgs)
	if err != nil {
		h.replyAgentErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"run": run})
}

func (h *Handler) handleUndoAgentRun(c *gin.Context) {
	user := currentUser(c)
	run, err := h.app.Agent.UndoRun(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyAgentErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"run": run})
}

type saveMemoryReq struct {
	Scope   string `json:"scope"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *Handler) handleListAgentMemories(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Agent.ListMemories(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleCreateAgentMemory(c *gin.Context) {
	user := currentUser(c)
	var req saveMemoryReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "content is required"))
		return
	}
	doc, err := h.app.Agent.SaveMemory(c.Request.Context(), user.ID, req.Scope, req.Title, req.Content)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"memory": doc})
}

type createMCPServerReq struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Endpoint  string `json:"endpoint"`
}

func (h *Handler) handleListMCPServers(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Agent.ListMCPServers(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleCreateMCPServer(c *gin.Context) {
	user := currentUser(c)
	var req createMCPServerReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Endpoint) == "" {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "endpoint is required"))
		return
	}
	srv, err := h.app.Agent.RegisterMCPServer(c.Request.Context(), user.ID, req.Name, req.Transport, req.Endpoint)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"server": srv})
}

func (h *Handler) replyAgentErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrRunNotFound), errors.Is(err, agent.ErrApprovalNotFound):
		kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
	case errors.Is(err, agent.ErrForbidden):
		kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
	default:
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
	}
}
