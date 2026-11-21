package ecommerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/canvas"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"gorm.io/gorm"
)

var (
	ErrNotFound  = errors.New("resource not found")
	ErrForbidden = errors.New("access is forbidden")
)

type Service struct {
	db     *gorm.DB
	canvas *canvas.Service
}

func NewService(db *gorm.DB, canvasSvc *canvas.Service) *Service {
	return &Service{db: db, canvas: canvasSvc}
}

func (s *Service) BootstrapKnowledge(ctx context.Context) error {
	entries := []model.EcommerceKnowledgeEntry{
		{
			ID:        "kb_amazon_main_image",
			Platform:  "amazon",
			Category:  "consumer_electronics",
			RuleTitle: "Amazon 主图纯白底与 85% 占比合规门禁",
			RuleBody:  "主图背景必须为纯白 RGB(255,255,255)，商品主体占画幅 85% 以上，严禁促销徽章与水印，文字排版锁定在 8%-92% 安全区内。",
			SafeZone:  "8%-92%",
		},
		{
			ID:        "kb_tiktok_shop_lifestyle",
			Platform:  "tiktok_shop",
			Category:  "fashion_apparel",
			RuleTitle: "TikTok Shop 北美/东南亚区域模特选角与痛点场景图规范",
			RuleBody:  "首帧场景图需匹配目标销售区本土模特形象（Region Casting），前 2 张副图突出使用前后对比，四周预留 8%-92% UI 贴纸安全区。",
			SafeZone:  "8%-92%",
		},
		{
			ID:        "kb_tmall_luxury_lighting",
			Platform:  "tmall",
			Category:  "beauty_skincare",
			RuleTitle: "天猫美妆高奢质感布光与卖点视觉层级",
			RuleBody:  "采用微距流体质感背光，核心成分卖点置于黄金分割位，保持 8%-92% 排版安全边界避免移动端圆角裁切。",
			SafeZone:  "8%-92%",
		},
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, e := range entries {
			if err := tx.Save(&e).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ListKnowledge(ctx context.Context, platform string) ([]model.EcommerceKnowledgeEntry, error) {
	q := s.db.WithContext(ctx)
	if strings.TrimSpace(platform) != "" {
		q = q.Where("platform = ?", strings.TrimSpace(platform))
	}
	var list []model.EcommerceKnowledgeEntry
	err := q.Order("platform ASC, id ASC").Find(&list).Error
	return list, err
}

type CreateEcommerceInput struct {
	OwnerID        string
	ProductName    string
	CategoryCode   string
	TargetPlatform string
	RegionCasting  string
}

func (s *Service) CreateEcommerceProject(ctx context.Context, in CreateEcommerceInput) (*model.EcommerceProject, error) {
	prodName := strings.TrimSpace(in.ProductName)
	if prodName == "" {
		prodName = "AI 智能降噪耳机 Pro"
	}
	plat := strings.TrimSpace(in.TargetPlatform)
	if plat == "" {
		plat = "amazon"
	}
	cat := strings.TrimSpace(in.CategoryCode)
	if cat == "" {
		cat = "consumer_electronics"
	}
	casting := strings.TrimSpace(in.RegionCasting)
	if casting == "" {
		casting = "north_america"
	}

	directions, _ := json.Marshal([]map[string]any{
		{
			"id":          "dir_hero_white",
			"title":       "方向 A：合规纯白底高锐度主图 (85% 占比)",
			"safeZone":    "8%-92%",
			"imagePrompt": fmt.Sprintf("Pure white background RGB(255,255,255), studio product photography of %s, 85%% frame fill, 8%%-92%% safe margin", prodName),
		},
		{
			"id":          "dir_lifestyle_cast",
			"title":       fmt.Sprintf("方向 B：区域本土模特生活场景图 (%s)", casting),
			"safeZone":    "8%-92%",
			"imagePrompt": fmt.Sprintf("Lifestyle commercial photography of %s with %s model, natural key light, 8%%-92%% typography safe zone", prodName, casting),
		},
	})

	images, _ := json.Marshal([]map[string]any{
		{
			"id":       "ec_img_1",
			"role":     "main_white_bg",
			"url":      "https://placehold.co/1000x1000/ffffff/0f172a.png?text=Compliant+Main+Image+(8%-92%+SafeZone)",
			"safeZone": "8%-92%",
		},
		{
			"id":       "ec_img_2",
			"role":     "lifestyle_scene",
			"url":      "https://placehold.co/1000x1000/1e293b/38bdf8.png?text=Region+Casting+Lifestyle+Shot",
			"safeZone": "8%-92%",
		},
	})

	proj := &model.EcommerceProject{
		ID:              uuid.NewString(),
		OwnerID:         in.OwnerID,
		ProductName:     prodName,
		CategoryCode:    cat,
		TargetPlatform:  plat,
		RegionCasting:   casting,
		SafeAreaMargin:  "8%-92%",
		ComplianceScore: 98,
		DirectionsJSON:  string(directions),
		SelectedDirID:   "dir_hero_white",
		ImagesJSON:      string(images),
		Status:          "completed",
	}
	if err := s.db.WithContext(ctx).Create(proj).Error; err != nil {
		return nil, err
	}
	return proj, nil
}

func (s *Service) ListEcommerceProjects(ctx context.Context, ownerID string) ([]model.EcommerceProject, error) {
	var list []model.EcommerceProject
	err := s.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&list).Error
	return list, err
}

type CreatePrevisInput struct {
	OwnerID         string
	Title           string
	CityPreset      string
	Weather         string
	CameraRecipe    string
	CharacterPose   string
	CanvasProjectID string
}

func (s *Service) CreatePrevisScene(ctx context.Context, in CreatePrevisInput) (*model.PrevisScene, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "3D 导演预演场景 #1"
	}
	city := strings.TrimSpace(in.CityPreset)
	if city == "" {
		city = "cyberpunk_blocks"
	}
	weather := strings.TrimSpace(in.Weather)
	if weather == "" {
		weather = "neon_night"
	}
	cam := strings.TrimSpace(in.CameraRecipe)
	if cam == "" {
		cam = "dolly_zoom"
	}
	pose := strings.TrimSpace(in.CharacterPose)
	if pose == "" {
		pose = "umbrella_stand"
	}

	propsJSON, _ := json.Marshal(map[string]any{
		"groundMarkers": []map[string]any{
			{"label": "A (起点)", "x": -2.5, "z": 1.0},
			{"label": "B (终点)", "x": 2.5, "z": -1.5},
		},
		"parametricProps": []string{"street_neon_arch", "rain_puddle_plane", "patrol_drone_rig"},
	})

	id := uuid.NewString()
	nodeID := "node_previs_" + id[:8]
	depthURL := fmt.Sprintf("/api/v1/previs/scenes/%s/depth.mp4", id)
	whiteURL := fmt.Sprintf("/api/v1/previs/scenes/%s/whitemodel.mp4", id)

	scene := &model.PrevisScene{
		ID:              id,
		OwnerID:         in.OwnerID,
		Title:           title,
		CityPreset:      city,
		Weather:         weather,
		CameraRecipe:    cam,
		CharacterPose:   pose,
		PropsJSON:       string(propsJSON),
		DepthVideoURL:   depthURL,
		WhiteModelVideo: whiteURL,
		CanvasNodeID:    nodeID,
	}
	if err := s.db.WithContext(ctx).Create(scene).Error; err != nil {
		return nil, err
	}

	// Write exported 3D Previs Depth & White-Model node back to user's Infinite Canvas
	if s.canvas != nil {
		projects, err := s.canvas.ListProjects(ctx, in.OwnerID)
		if err == nil && len(projects) > 0 {
			targetProj := projects[0]
			if in.CanvasProjectID != "" {
				if p, err := s.canvas.GetProject(ctx, in.OwnerID, in.CanvasProjectID); err == nil && p != nil {
					targetProj = *p
				}
			}
			_, _ = s.canvas.ApplyOperations(ctx, canvas.ApplyOperationsInput{
				UserID:       in.OwnerID,
				ProjectID:    targetProj.ID,
				BaseRevision: targetProj.Revision,
				ClientID:     "previs_export_" + id,
				Operations: []canvas.Operation{
					{
						Op: "node.add",
						Node: &canvas.NodeData{
							ID:       nodeID,
							Type:     "video",
							Title:    "3D Previs 预演深度控制视频 · " + title,
							Position: canvas.Position{X: 520, Y: 440},
							Width:    340,
							Height:   220,
							Metadata: map[string]any{
								"prompt": fmt.Sprintf("[3D Previs] City=%s Weather=%s Camera=%s Pose=%s", city, weather, cam, pose),
								"url":    depthURL,
								"status": "success",
							},
						},
					},
				},
			})
		}
	}

	return scene, nil
}

func (s *Service) ListPrevisScenes(ctx context.Context, ownerID string) ([]model.PrevisScene, error) {
	var list []model.PrevisScene
	err := s.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (s *Service) CreateTeam(ctx context.Context, ownerID, name string, parentTeamID *string, initialPoolMicro int64) (*model.Team, error) {
	tName := strings.TrimSpace(name)
	if tName == "" {
		tName = "影视工作室团队空间"
	}
	var team model.Team
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		team = model.Team{
			ID:               uuid.NewString(),
			OwnerID:          ownerID,
			ParentTeamID:     parentTeamID,
			Name:             tName,
			PoolMicrocredits: initialPoolMicro,
		}
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		mem := model.TeamMembership{
			ID:                uuid.NewString(),
			TeamID:            team.ID,
			UserID:            ownerID,
			Role:              "owner",
			QuotaMicrocredits: initialPoolMicro,
		}
		return tx.Create(&mem).Error
	})
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *Service) ListTeams(ctx context.Context, userID string) ([]model.Team, error) {
	var list []model.Team
	err := s.db.WithContext(ctx).Where("owner_id = ?", userID).Order("created_at DESC").Find(&list).Error
	return list, err
}
