package skills

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"gorm.io/gorm"
)

var (
	ErrSkillNotFound = errors.New("skill not found")
	ErrForbidden     = errors.New("access to skill is forbidden")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) BootstrapBuiltins(ctx context.Context) error {
	builtins := []model.Skill{
		{
			ID:           "skill_builtin_storyboard_director",
			Name:         "影视导演分镜拆解大师",
			Category:     "storyboard",
			OutputMode:   "workflow",
			Description:  "将小说或剧本段落拆解为包含景别、运镜、情绪、首帧图提示词与运动提示词的标准分镜表",
			SystemPrompt: "你是好莱坞与国漫资深分镜导演。将用户输入拆解为结构化分镜行：景别(shotSize)、运镜(camera)、光影(lightingAndAtmosphere)、首帧画面提示词(imageGenerationPrompt)、视频运动提示词(videoMotionPrompt)、连续性衔接(continuityOut)。",
			FilesJSON:    `{"SKILL.md":"# 影视导演分镜拆解大师\n输出标准 StoryboardRow 数组。","examples.json":"[]"}`,
			IsBuiltin:    true,
			Enabled:      true,
		},
		{
			ID:           "skill_builtin_character_sheet",
			Name:         "角色四视图定妆设定",
			Category:     "image",
			OutputMode:   "image_prompt",
			Description:  "生成同一角色正面、侧面、背面与特写的一致性四视图定妆照提示词",
			SystemPrompt: "输出包含 character turnaround sheet, front view, side view, back view, close-up portrait, consistent costume and lighting 的高质量绘画提示词。",
			FilesJSON:    `{"SKILL.md":"# 角色四视图定妆设定\n保持跨镜头人物服装与五官一致。"}`,
			IsBuiltin:    true,
			Enabled:      true,
		},
		{
			ID:           "skill_builtin_video_motion_polish",
			Name:         "Seedance / Sora 运镜提示词精修",
			Category:     "video",
			OutputMode:   "text",
			Description:  "针对图生视频模型优化主体运动轨迹、镜头推拉摇移与物理时间节拍",
			SystemPrompt: "将静态画面描述改写为包含 camera movement, subject motion, time beats, ambient motion 的视频生成提示词。",
			FilesJSON:    `{"SKILL.md":"# 视频运镜提示词精修"}`,
			IsBuiltin:    true,
			Enabled:      true,
		},
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, sk := range builtins {
			if err := tx.Save(&sk).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ListSkills(ctx context.Context, userID, category string) ([]model.Skill, error) {
	q := s.db.WithContext(ctx).Where("enabled = ? AND (is_builtin = ? OR owner_id = ?)", true, true, userID)
	if strings.TrimSpace(category) != "" {
		q = q.Where("category = ?", strings.TrimSpace(category))
	}
	var list []model.Skill
	err := q.Order("is_builtin DESC, likes_count DESC, created_at DESC").Find(&list).Error
	return list, err
}

type CreateSkillInput struct {
	OwnerID      string
	Name         string
	Category     string
	OutputMode   string
	Description  string
	SystemPrompt string
	Files        map[string]string
}

func (s *Service) CreateCustomSkill(ctx context.Context, in CreateSkillInput) (*model.Skill, error) {
	filesJSON := "{}"
	if len(in.Files) > 0 {
		if b, err := json.Marshal(in.Files); err == nil {
			filesJSON = string(b)
		}
	}
	cat := strings.TrimSpace(in.Category)
	if cat == "" {
		cat = "utility"
	}
	mode := strings.TrimSpace(in.OutputMode)
	if mode == "" {
		mode = "text"
	}
	sk := &model.Skill{
		ID:           "skill_" + uuid.NewString()[:12],
		OwnerID:      in.OwnerID,
		Name:         strings.TrimSpace(in.Name),
		Category:     cat,
		OutputMode:   mode,
		Description:  strings.TrimSpace(in.Description),
		SystemPrompt: strings.TrimSpace(in.SystemPrompt),
		FilesJSON:    filesJSON,
		IsBuiltin:    false,
		Enabled:      true,
	}
	if sk.Name == "" {
		sk.Name = "自定义创作技能"
	}
	if err := s.db.WithContext(ctx).Create(sk).Error; err != nil {
		return nil, err
	}
	return sk, nil
}
