package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/smartinbox17-droid/canvas/backend/internal/ecommerce"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
)

func (h *Handler) handleListEcommerceKnowledge(c *gin.Context) {
	items, err := h.app.Ecommerce.ListKnowledge(c.Request.Context(), c.Query("platform"))
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

func (h *Handler) handleListEcommerceProjects(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Ecommerce.ListEcommerceProjects(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createEcommerceReq struct {
	ProductName    string `json:"productName"`
	CategoryCode   string `json:"categoryCode"`
	TargetPlatform string `json:"targetPlatform"`
	RegionCasting  string `json:"regionCasting"`
}

func (h *Handler) handleCreateEcommerceProject(c *gin.Context) {
	user := currentUser(c)
	var req createEcommerceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	proj, err := h.app.Ecommerce.CreateEcommerceProject(c.Request.Context(), ecommerce.CreateEcommerceInput{
		OwnerID:        user.ID,
		ProductName:    req.ProductName,
		CategoryCode:   req.CategoryCode,
		TargetPlatform: req.TargetPlatform,
		RegionCasting:  req.RegionCasting,
	})
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"project": proj})
}

func (h *Handler) handleListPrevisScenes(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Ecommerce.ListPrevisScenes(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createPrevisReq struct {
	Title           string `json:"title"`
	CityPreset      string `json:"cityPreset"`
	Weather         string `json:"weather"`
	CameraRecipe    string `json:"cameraRecipe"`
	CharacterPose   string `json:"characterPose"`
	CanvasProjectID string `json:"canvasProjectId"`
}

func (h *Handler) handleCreatePrevisScene(c *gin.Context) {
	user := currentUser(c)
	var req createPrevisReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	scene, err := h.app.Ecommerce.CreatePrevisScene(c.Request.Context(), ecommerce.CreatePrevisInput{
		OwnerID:         user.ID,
		Title:           req.Title,
		CityPreset:      req.CityPreset,
		Weather:         req.Weather,
		CameraRecipe:    req.CameraRecipe,
		CharacterPose:   req.CharacterPose,
		CanvasProjectID: req.CanvasProjectID,
	})
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"scene": scene})
}

func (h *Handler) handleListTeams(c *gin.Context) {
	user := currentUser(c)
	items, err := h.app.Ecommerce.ListTeams(c.Request.Context(), user.ID)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"items": items})
}

type createTeamReq struct {
	Name             string  `json:"name"`
	ParentTeamID     *string `json:"parentTeamId"`
	PoolMicrocredits int64   `json:"poolMicrocredits"`
}

func (h *Handler) handleCreateTeam(c *gin.Context) {
	user := currentUser(c)
	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusBadRequest, 400, kernel.ReasonInvalidArgument, "invalid json payload"))
		return
	}
	team, err := h.app.Ecommerce.CreateTeam(c.Request.Context(), user.ID, req.Name, req.ParentTeamID, req.PoolMicrocredits)
	if err != nil {
		kernel.Fail(c, kernel.NewError(http.StatusInternalServerError, 500, kernel.ReasonInternalError, err.Error()))
		return
	}
	kernel.OK(c, gin.H{"team": team})
}
