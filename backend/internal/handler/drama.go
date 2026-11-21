package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/production"
)

func (h *Handler) handleListDramas(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Production.ListDramas(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createDramaReq struct {
	Title        string `json:"title"`
	RawScript    string `json:"rawScript"`
	PacingPreset string `json:"pacingPreset"`
}

func (h *Handler) handleCreateDrama(c *gin.Context) {
	user := currentUser(c)
	var req createDramaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	detail, err := h.app.Production.CreateDrama(c.Request.Context(), production.CreateDramaInput{
		OwnerID:      user.ID,
		Title:        req.Title,
		RawScript:    req.RawScript,
		PacingPreset: req.PacingPreset,
	})
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, detail)
}

func (h *Handler) handleGetDrama(c *gin.Context) {
	user := currentUser(c)
	detail, err := h.app.Production.GetDramaDetail(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyDramaErr(c, err)
		return
	}
	kernel.OK(c, detail)
}

func (h *Handler) handleStartDramaOrchestrator(c *gin.Context) {
	user := currentUser(c)
	detail, err := h.app.Production.RunOrchestrator(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		h.replyDramaErr(c, err)
		return
	}
	kernel.OK(c, detail)
}

func (h *Handler) handleExportJianyingDraft(c *gin.Context) {
	user := currentUser(c)
	draft, err := h.app.Production.ExportJianyingDraft(c.Request.Context(), user.ID, c.Param("id"), c.Param("episodeId"))
	if err != nil {
		h.replyDramaErr(c, err)
		return
	}
	kernel.OK(c, gin.H{"draft": draft})
}

func (h *Handler) replyDramaErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, production.ErrDramaNotFound), errors.Is(err, production.ErrEpisodeNotFound):
		kernel.Fail(c, kernel.NewError(http.StatusNotFound, 404, kernel.ReasonNotFound, err.Error()))
	case errors.Is(err, production.ErrForbidden):
		kernel.Fail(c, kernel.NewError(http.StatusForbidden, 403, kernel.ReasonForbidden, err.Error()))
	default:
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
	}
}
