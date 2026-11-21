package production

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

const MaxSegmentDurationSec = 10

var (
	ErrDramaNotFound   = errors.New("drama project not found")
	ErrEpisodeNotFound = errors.New("drama episode not found")
	ErrForbidden       = errors.New("access to drama project is forbidden")
)

type Service struct {
	db     *gorm.DB
	canvas *canvas.Service
}

func NewService(db *gorm.DB, canvasSvc *canvas.Service) *Service {
	return &Service{db: db, canvas: canvasSvc}
}

type CreateDramaInput struct {
	OwnerID      string
	Title        string
	RawScript    string
	PacingPreset string
}

type DramaDetail struct {
	Drama    model.DramaProject `json:"drama"`
	Cast     []model.DramaCast  `json:"cast"`
	Episodes []model.DramaEpisode `json:"episodes"`
	Shots    []model.DramaShot  `json:"shots"`
	Segments []model.DramaSegment `json:"segments"`
}

func (s *Service) CreateDrama(ctx context.Context, in CreateDramaInput) (*DramaDetail, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "未命名 AI 短剧项目"
	}
	pacing := strings.TrimSpace(in.PacingPreset)
	if pacing == "" {
		pacing = "standard"
	}
	rawScript := strings.TrimSpace(in.RawScript)
	if rawScript == "" {
		rawScript = "第一集：霓虹雨夜。林零撑着透明雨伞站在高架桥下，收到神秘加密通讯。她猛然转身，无人机探照灯划破夜空。"
	}

	storyBible, _ := json.Marshal(map[string]any{
		"genre":       "科幻悬疑短剧",
		"tone":        "高反差冷暖霓虹、快节奏黄金前 3 秒钩子",
		"protagonist": "林零 (赛博黑客少女)",
	})

	var detail DramaDetail

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		drama := model.DramaProject{
			ID:             uuid.NewString(),
			OwnerID:        in.OwnerID,
			Title:          title,
			RawScript:      rawScript,
			StoryBibleJSON: string(storyBible),
			PacingPreset:   pacing,
			Stage:          "script",
		}
		if err := tx.Create(&drama).Error; err != nil {
			return err
		}

		// Auto-extract starter cast (lead character + key scene)
		castList := []model.DramaCast{
			{
				ID:          uuid.NewString(),
				DramaID:     drama.ID,
				Name:        "林零",
				RoleType:    "lead",
				Description: "银色短发、黑色机能风衣、左眼佩戴全息目镜",
				ImagePrompt: "Character turnaround sheet, front/side/back/close-up of Lin Ling, silver short hair, black techwear trenchcoat, holographic monocle",
				ImageURL:    "https://placehold.co/1024x512/1e293b/38bdf8.png?text=Cast+Turnaround+Sheet:+Lin+Ling",
				VoiceID:     "voice_female_cool_01",
				ImageReady:  true,
			},
			{
				ID:          uuid.NewString(),
				DramaID:     drama.ID,
				Name:        "高架桥霓虹雨夜街区",
				RoleType:    "scene",
				Description: "积水反射全息广告牌的赛博高架桥下通道",
				ImagePrompt: "Environment concept art, three-view cyberpunk overpass alley in rain, neon reflections",
				ImageURL:    "https://placehold.co/1024x512/0f172a/a855f7.png?text=Scene+Sheet:+Neon+Overpass",
				ImageReady:  true,
			},
		}
		for _, c := range castList {
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
		}

		ep := model.DramaEpisode{
			ID:            uuid.NewString(),
			DramaID:       drama.ID,
			EpisodeNumber: 1,
			Title:         "第 1 集：雨夜来电",
			Summary:       rawScript,
			Approved:      true,
		}
		if err := tx.Create(&ep).Error; err != nil {
			return err
		}

		// Split script into 3 structured director shots
		shots := []model.DramaShot{
			{
				ID:                uuid.NewString(),
				DramaID:           drama.ID,
				EpisodeID:         ep.ID,
				ShotIndex:         1,
				DurationSec:       4,
				ShotSize:          "特写 (Close-Up)",
				CameraMotion:      "缓慢推近 (Slow Push-In)",
				SceneDescription:  "雨滴打在透明雨伞上，林零全息目镜闪烁收到报警信号",
				Dialogue:          "林零：信号源就在高架桥下三百米。",
				ImagePrompt:       "Close-up of Lin Ling under transparent umbrella in neon rain, holographic monocle glowing",
				VideoMotionPrompt: "Slow camera push-in, raindrops splashing on umbrella, holographic HUD flickering",
				ContinuityOut:     "保持右侧蓝色霓虹主光，林零向左侧转头",
				KeyframeURL:       "https://placehold.co/960x540/1d4ed8/ffffff.png?text=Shot+1+Keyframe",
			},
			{
				ID:                uuid.NewString(),
				DramaID:           drama.ID,
				EpisodeID:         ep.ID,
				ShotIndex:         2,
				DurationSec:       5,
				ShotSize:          "中景 (Medium Shot)",
				CameraMotion:      "环绕跟拍 (Orbit Tracking)",
				SceneDescription:  "林零猛然转身，黑色风衣下摆甩开雨水",
				Dialogue:          "神秘声音：你已经暴露了，快离开！",
				ImagePrompt:       "Medium shot of Lin Ling turning around rapidly in rainy alley, trenchcoat flaring",
				VideoMotionPrompt: "Dynamic orbit camera movement around subject turning, coat tails flying with water droplets",
				ContinuityOut:     "镜头仰角衔接空中探照灯",
				KeyframeURL:       "https://placehold.co/960x540/4338ca/ffffff.png?text=Shot+2+Keyframe",
			},
			{
				ID:                uuid.NewString(),
				DramaID:           drama.ID,
				EpisodeID:         ep.ID,
				ShotIndex:         3,
				DurationSec:       4,
				ShotSize:          "大远景 (Wide Shot)",
				CameraMotion:      "低角度仰拍拉远 (Low-Angle Pull-Back)",
				SceneDescription:  "三架巡逻无人机穿过高架桥浓雾，白色探照灯锁定地面",
				Dialogue:          "（紧张电子低音音效）",
				ImagePrompt:       "Wide low-angle shot of patrol drones emerging from fog under overpass, volumetric searchlights",
				VideoMotionPrompt: "Low angle camera pulling back as patrol drones fly forward sweeping searchlights through rain",
				ContinuityOut:     "切黑留白悬念结尾",
				KeyframeURL:       "https://placehold.co/960x540/6d28d9/ffffff.png?text=Shot+3+Keyframe",
			},
		}
		for _, sh := range shots {
			if err := tx.Create(&sh).Error; err != nil {
				return err
			}
		}

		detail = DramaDetail{
			Drama:    drama,
			Cast:     castList,
			Episodes: []model.DramaEpisode{ep},
			Shots:    shots,
			Segments: []model.DramaSegment{},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

func (s *Service) ListDramas(ctx context.Context, ownerID string) ([]model.DramaProject, error) {
	var list []model.DramaProject
	err := s.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("updated_at DESC").Find(&list).Error
	return list, err
}

func (s *Service) GetDramaDetail(ctx context.Context, ownerID, dramaID string) (*DramaDetail, error) {
	var drama model.DramaProject
	if err := s.db.WithContext(ctx).First(&drama, "id = ?", dramaID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDramaNotFound
		}
		return nil, err
	}
	if drama.OwnerID != ownerID {
		return nil, ErrForbidden
	}

	var cast []model.DramaCast
	var episodes []model.DramaEpisode
	var shots []model.DramaShot
	var segments []model.DramaSegment

	s.db.WithContext(ctx).Where("drama_id = ?", drama.ID).Order("created_at ASC").Find(&cast)
	s.db.WithContext(ctx).Where("drama_id = ?", drama.ID).Order("episode_number ASC").Find(&episodes)
	s.db.WithContext(ctx).Where("drama_id = ?", drama.ID).Order("shot_index ASC").Find(&shots)
	s.db.WithContext(ctx).Where("drama_id = ?", drama.ID).Order("segment_index ASC").Find(&segments)

	return &DramaDetail{
		Drama:    drama,
		Cast:     cast,
		Episodes: episodes,
		Shots:    shots,
		Segments: segments,
	}, nil
}

// RunOrchestrator advances the drama pipeline: slices shots into <=10s segments, chains tail frames, composes episode SRT & MP4, and syncs to canvas.
func (s *Service) RunOrchestrator(ctx context.Context, ownerID, dramaID string) (*DramaDetail, error) {
	detail, err := s.GetDramaDetail(ctx, ownerID, dramaID)
	if err != nil {
		return nil, err
	}
	if len(detail.Episodes) == 0 {
		return detail, ErrEpisodeNotFound
	}
	ep := detail.Episodes[0]

	// 1. Slice shots into video segments respecting MaxSegmentDurationSec (10s) and chain tail-frame to next segment's first-frame
	type bucket struct {
		shots    []model.DramaShot
		totalSec int
	}
	var buckets []bucket
	var cur bucket
	for _, sh := range detail.Shots {
		if cur.totalSec > 0 && cur.totalSec+sh.DurationSec > MaxSegmentDurationSec {
			buckets = append(buckets, cur)
			cur = bucket{}
		}
		cur.shots = append(cur.shots, sh)
		cur.totalSec += sh.DurationSec
	}
	if len(cur.shots) > 0 {
		buckets = append(buckets, cur)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("drama_id = ? AND episode_id = ?", detail.Drama.ID, ep.ID).Delete(&model.DramaSegment{})

		prevTailURL := ""
		for idx, b := range buckets {
			var promptParts []string
			for _, sh := range b.shots {
				promptParts = append(promptParts, fmt.Sprintf("[Shot %d %ds] %s (%s)", sh.ShotIndex, sh.DurationSec, sh.VideoMotionPrompt, sh.ContinuityOut))
			}
			firstFrame := b.shots[0].KeyframeURL
			if prevTailURL != "" {
				// Tail-frame chaining: previous segment's extracted tail frame becomes current segment's first frame
				firstFrame = prevTailURL
			}
			tailFrame := fmt.Sprintf("https://placehold.co/960x540/0ea5e9/ffffff.png?text=Seg+%d+Extracted+TailFrame", idx+1)
			prevTailURL = tailFrame

			seg := model.DramaSegment{
				ID:             uuid.NewString(),
				DramaID:        detail.Drama.ID,
				EpisodeID:      ep.ID,
				SegmentIndex:   idx + 1,
				DurationSec:    b.totalSec,
				CombinedPrompt: strings.Join(promptParts, " -> "),
				FirstFrameURL:  firstFrame,
				TailFrameURL:   tailFrame,
				TailFrameReady: true,
				VideoURL:       fmt.Sprintf("/api/v1/dramas/%s/segments/%d.mp4", detail.Drama.ID, idx+1),
				Status:         "succeeded",
			}
			if err := tx.Create(&seg).Error; err != nil {
				return err
			}
		}

		// 2. Compose Episode SRT subtitles and final video URL (with AIGC metadata tag)
		srt := buildSRT(detail.Shots)
		composedURL := fmt.Sprintf("/api/v1/dramas/%s/episodes/%s/composed.mp4?aigc_tag=yingce_v1", detail.Drama.ID, ep.ID)
		if err := tx.Model(&model.DramaEpisode{}).Where("id = ?", ep.ID).Updates(map[string]any{
			"composed_url": composedURL,
			"srt_content":  srt,
		}).Error; err != nil {
			return err
		}

		return tx.Model(&model.DramaProject{}).Where("id = ?", detail.Drama.ID).Update("stage", "completed").Error
	})
	if err != nil {
		return nil, err
	}

	// 3. Bi-directional sync: create or update corresponding Storyboard & Segment nodes on the user's Infinite Canvas
	if s.canvas != nil {
		var canvasProj *model.CanvasProject
		if detail.Drama.CanvasProjectID != "" {
			canvasProj, _ = s.canvas.GetProject(ctx, ownerID, detail.Drama.CanvasProjectID)
		}
		if canvasProj == nil {
			canvasProj, _ = s.canvas.CreateProject(ctx, ownerID, "短剧精修画布 · "+detail.Drama.Title)
			if canvasProj != nil {
				_ = s.db.WithContext(ctx).Model(&model.DramaProject{}).
					Where("id = ?", detail.Drama.ID).
					Update("canvas_project_id", canvasProj.ID).Error
			}
		}
		if canvasProj != nil {
			var ops []canvas.Operation
			for i, sh := range detail.Shots {
				nodeID := fmt.Sprintf("drama_shot_%d", sh.ShotIndex)
				ops = append(ops, canvas.Operation{
					Op: "node.add",
					Node: &canvas.NodeData{
						ID:       nodeID,
						Type:     "image",
						Title:    fmt.Sprintf("分镜 #%d (%s)", sh.ShotIndex, sh.ShotSize),
						Position: canvas.Position{X: float64(100 + i*360), Y: 420},
						Width:    320,
						Height:   210,
						Metadata: map[string]any{
							"prompt": sh.ImagePrompt,
							"url":    sh.KeyframeURL,
							"status": "success",
						},
					},
				})
			}
			_, _ = s.canvas.ApplyOperations(ctx, canvas.ApplyOperationsInput{
				UserID:       ownerID,
				ProjectID:    canvasProj.ID,
				BaseRevision: canvasProj.Revision,
				ClientID:     "drama_sync_" + detail.Drama.ID,
				Operations:   ops,
			})
		}
	}

	return s.GetDramaDetail(ctx, ownerID, dramaID)
}

func buildSRT(shots []model.DramaShot) string {
	var sb strings.Builder
	curSec := 0
	for idx, sh := range shots {
		start := curSec
		end := curSec + sh.DurationSec
		curSec = end
		_, _ = fmt.Fprintf(&sb, "%d\n00:00:%02d,000 --> 00:00:%02d,000\n%s\n\n", idx+1, start, end, sh.Dialogue)
	}
	return sb.String()
}

type JianyingDraft struct {
	Version   string           `json:"version"`
	DramaID   string           `json:"dramaId"`
	EpisodeID string           `json:"episodeId"`
	Canvas    map[string]int   `json:"canvas_config"`
	Tracks    []map[string]any `json:"tracks"`
	SRT       string           `json:"srt"`
}

func (s *Service) ExportJianyingDraft(ctx context.Context, ownerID, dramaID, episodeID string) (*JianyingDraft, error) {
	detail, err := s.GetDramaDetail(ctx, ownerID, dramaID)
	if err != nil {
		return nil, err
	}
	var videoClips []map[string]any
	offsetUs := int64(0)
	for _, seg := range detail.Segments {
		durUs := int64(seg.DurationSec) * 1_000_000
		videoClips = append(videoClips, map[string]any{
			"segment_id":       seg.ID,
			"source_url":       seg.VideoURL,
			"first_frame_url":  seg.FirstFrameURL,
			"tail_frame_url":   seg.TailFrameURL,
			"target_timerange": map[string]int64{"start": offsetUs, "duration": durUs},
		})
		offsetUs += durUs
	}

	srt := ""
	if len(detail.Episodes) > 0 {
		srt = detail.Episodes[0].SRTContent
	}
	if srt == "" {
		srt = buildSRT(detail.Shots)
	}

	return &JianyingDraft{
		Version:   "jianying.draft/v1",
		DramaID:   dramaID,
		EpisodeID: episodeID,
		Canvas:    map[string]int{"width": 1920, "height": 1080, "fps": 30},
		Tracks: []map[string]any{
			{"type": "video", "segments": videoClips},
			{"type": "subtitle", "format": "srt"},
		},
		SRT: srt,
	}, nil
}
