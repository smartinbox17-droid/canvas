package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/canvas"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"gorm.io/gorm"
)

var (
	ErrRunNotFound      = errors.New("agent run not found")
	ErrApprovalNotFound = errors.New("agent approval not found")
	ErrForbidden        = errors.New("access to agent resource is forbidden")
)

type ToolCapability struct {
	Name             string `json:"name"`
	Category         string `json:"category"` // canvas | storyboard | generation | memory | mcp
	RequiresApproval bool   `json:"requiresApproval"`
	Description      string `json:"description"`
}

var BuiltinTools = []ToolCapability{
	{Name: "canvas.read_snapshot", Category: "canvas", RequiresApproval: false, Description: "读取当前无限画布节点与连线拓扑"},
	{Name: "canvas.create_storyboard_workflow", Category: "storyboard", RequiresApproval: false, Description: "在画布上自动创建剧本、分镜图与视频节点工作流"},
	{Name: "canvas.delete_nodes", Category: "canvas", RequiresApproval: true, Description: "删除画布上的节点（高风险操作需用户审批）"},
	{Name: "generation.submit_batch", Category: "generation", RequiresApproval: true, Description: "提交消耗积分的批量多模态生成任务（需用户审批）"},
	{Name: "memory.save_document", Category: "memory", RequiresApproval: false, Description: "写入长期创作记忆文档"},
}

type Service struct {
	db     *gorm.DB
	canvas *canvas.Service
}

func NewService(db *gorm.DB, canvasSvc *canvas.Service) *Service {
	return &Service{db: db, canvas: canvasSvc}
}

func (s *Service) ListCapabilities() []ToolCapability {
	return BuiltinTools
}

type StartRunInput struct {
	UserID          string
	ConversationID  string
	ProjectID       string
	Prompt          string
	Model           string
	RequireApproval bool
}

func (s *Service) StartRun(ctx context.Context, in StartRunInput) (*model.CloudAgentRun, *model.CloudAgentApproval, error) {
	convID := strings.TrimSpace(in.ConversationID)
	if convID == "" {
		conv := model.CloudAgentConversation{
			ID:        uuid.NewString(),
			UserID:    in.UserID,
			ProjectID: in.ProjectID,
			Title:     truncateTitle(in.Prompt),
		}
		if err := s.db.WithContext(ctx).Create(&conv).Error; err != nil {
			return nil, nil, err
		}
		convID = conv.ID
	}

	modelCode := strings.TrimSpace(in.Model)
	if modelCode == "" {
		modelCode = "mock-gpt-4o"
	}

	run := &model.CloudAgentRun{
		ID:             uuid.NewString(),
		ConversationID: convID,
		UserID:         in.UserID,
		ProjectID:      in.ProjectID,
		Prompt:         strings.TrimSpace(in.Prompt),
		Model:          modelCode,
		Status:         "running",
		ToolBudget:     16,
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, nil, err
	}

	seq := 1
	s.appendEvent(ctx, run.ID, &seq, "run.started", map[string]any{
		"runId":  run.ID,
		"model":  run.Model,
		"prompt": run.Prompt,
	})

	// Check if prompt or flag requests an approval-gated tool (e.g., batch generation or deletion)
	needsApproval := in.RequireApproval ||
		strings.Contains(strings.ToLower(in.Prompt), "审批") ||
		strings.Contains(strings.ToLower(in.Prompt), "批量") ||
		strings.Contains(strings.ToLower(in.Prompt), "batch")

	if needsApproval {
		argsBytes, _ := json.Marshal(map[string]any{
			"prompt":    in.Prompt,
			"projectId": in.ProjectID,
			"count":     3,
		})
		approval := &model.CloudAgentApproval{
			ID:       "appr_" + uuid.NewString()[:12],
			RunID:    run.ID,
			UserID:   in.UserID,
			ToolName: "generation.submit_batch",
			ArgsJSON: string(argsBytes),
			Status:   "pending",
		}
		if err := s.db.WithContext(ctx).Create(approval).Error; err != nil {
			return nil, nil, err
		}
		_ = s.db.WithContext(ctx).Model(run).Update("status", "waiting_approval").Error
		run.Status = "waiting_approval"

		s.appendEvent(ctx, run.ID, &seq, "approval.required", map[string]any{
			"approvalId": approval.ID,
			"toolName":   approval.ToolName,
			"args":       json.RawMessage(approval.ArgsJSON),
		})
		return run, approval, nil
	}

	// Otherwise execute canvas workflow tool directly and record inverse operations for one-click Undo
	if err := s.executeCanvasWorkflowTool(ctx, run, &seq); err != nil {
		return nil, nil, err
	}
	return run, nil, nil
}

func (s *Service) ResolveApproval(
	ctx context.Context,
	userID, runID, approvalID, decision string,
	editedArgs map[string]any,
) (*model.CloudAgentRun, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	var appr model.CloudAgentApproval
	if err := s.db.WithContext(ctx).First(&appr, "id = ? AND run_id = ?", approvalID, run.ID).Error; err != nil {
		return nil, ErrApprovalNotFound
	}
	if appr.Status != "pending" {
		return run, nil
	}

	now := time.Now().UTC()
	editedJSON := ""
	if len(editedArgs) > 0 {
		if b, err := json.Marshal(editedArgs); err == nil {
			editedJSON = string(b)
		}
	}

	seq := s.nextSeq(ctx, run.ID)
	if decision == "reject" {
		_ = s.db.WithContext(ctx).Model(&appr).Updates(map[string]any{
			"status":      "rejected",
			"decision":    "reject",
			"resolved_at": &now,
		}).Error
		_ = s.db.WithContext(ctx).Model(run).Updates(map[string]any{
			"status":       "cancelled",
			"completed_at": &now,
		}).Error
		s.appendEvent(ctx, run.ID, &seq, "approval.resolved", map[string]any{
			"approvalId": appr.ID,
			"decision":   "reject",
		})
		s.appendEvent(ctx, run.ID, &seq, "run.cancelled", map[string]any{"reason": "user_rejected_approval"})
		_ = s.db.WithContext(ctx).First(run, "id = ?", run.ID).Error
		return run, nil
	}

	_ = s.db.WithContext(ctx).Model(&appr).Updates(map[string]any{
		"status":           "approved",
		"decision":         "approve",
		"edited_args_json": editedJSON,
		"resolved_at":      &now,
	}).Error
	s.appendEvent(ctx, run.ID, &seq, "approval.resolved", map[string]any{
		"approvalId": appr.ID,
		"decision":   "approve",
	})

	if err := s.executeCanvasWorkflowTool(ctx, run, &seq); err != nil {
		return nil, err
	}
	_ = s.db.WithContext(ctx).First(run, "id = ?", run.ID).Error
	return run, nil
}

func (s *Service) executeCanvasWorkflowTool(ctx context.Context, run *model.CloudAgentRun, seq *int) error {
	s.appendEvent(ctx, run.ID, seq, "tool.call", map[string]any{
		"tool":   "canvas.create_storyboard_workflow",
		"prompt": run.Prompt,
	})

	var inverseOps []canvas.Operation
	if run.ProjectID != "" && s.canvas != nil {
		proj, err := s.canvas.GetProject(ctx, run.UserID, run.ProjectID)
		if err == nil && proj != nil {
			shotNodeID := "node_agent_shot_" + uuid.NewString()[:8]
			videoNodeID := "node_agent_video_" + uuid.NewString()[:8]
			connID := "conn_agent_" + uuid.NewString()[:8]

			forwardOps := []canvas.Operation{
				{
					Op: "node.add",
					Node: &canvas.NodeData{
						ID:       shotNodeID,
						Type:     "image",
						Title:    "Agent 分镜首帧: " + truncateTitle(run.Prompt),
						Position: canvas.Position{X: 260, Y: 380},
						Width:    300,
						Height:   200,
						Metadata: map[string]any{
							"prompt": run.Prompt,
							"model":  "mock-flux-pro",
							"status": "idle",
						},
					},
				},
				{
					Op: "node.add",
					Node: &canvas.NodeData{
						ID:       videoNodeID,
						Type:     "video",
						Title:    "Agent 镜头视频: " + truncateTitle(run.Prompt),
						Position: canvas.Position{X: 640, Y: 380},
						Width:    320,
						Height:   200,
						Metadata: map[string]any{
							"prompt": run.Prompt,
							"model":  "mock-seedance-video",
							"status": "idle",
						},
					},
				},
				{
					Op: "connection.add",
					Connection: &canvas.ConnectionData{
						ID:         connID,
						FromNodeID: shotNodeID,
						ToNodeID:   videoNodeID,
						Relation:   "storyboard-output",
					},
				},
			}

			_, applyErr := s.canvas.ApplyOperations(ctx, canvas.ApplyOperationsInput{
				UserID:       run.UserID,
				ProjectID:    proj.ID,
				BaseRevision: proj.Revision,
				ClientID:     "agent_" + run.ID,
				Operations:   forwardOps,
			})
			if applyErr == nil {
				inverseOps = []canvas.Operation{
					{Op: "connection.remove", ID: connID},
					{Op: "node.remove", ID: videoNodeID},
					{Op: "node.remove", ID: shotNodeID},
				}
				s.appendEvent(ctx, run.ID, seq, "canvas.ops", map[string]any{
					"projectId":  proj.ID,
					"operations": forwardOps,
				})
			}
		}
	}

	invBytes, _ := json.Marshal(inverseOps)
	replyText := fmt.Sprintf("已完成导演分镜规划与画布节点编排：%s", run.Prompt)
	s.appendEvent(ctx, run.ID, seq, "message.delta", map[string]any{"text": replyText})
	s.appendEvent(ctx, run.ID, seq, "run.completed", map[string]any{"status": "completed"})

	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(run).Updates(map[string]any{
		"status":           "completed",
		"inverse_ops_json": string(invBytes),
		"completed_at":     &now,
	}).Error
}

// UndoRun replays the recorded inverse canvas operations in reverse to roll back an Agent run's canvas mutations.
func (s *Service) UndoRun(ctx context.Context, userID, runID string) (*model.CloudAgentRun, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	if run.ProjectID != "" && run.InverseOpsJSON != "" && s.canvas != nil {
		var inverseOps []canvas.Operation
		if err := json.Unmarshal([]byte(run.InverseOpsJSON), &inverseOps); err == nil && len(inverseOps) > 0 {
			proj, err := s.canvas.GetProject(ctx, userID, run.ProjectID)
			if err == nil && proj != nil {
				_, _ = s.canvas.ApplyOperations(ctx, canvas.ApplyOperationsInput{
					UserID:       userID,
					ProjectID:    proj.ID,
					BaseRevision: proj.Revision,
					ClientID:     "agent_undo_" + run.ID,
					Operations:   inverseOps,
				})
			}
		}
	}

	seq := s.nextSeq(ctx, run.ID)
	s.appendEvent(ctx, run.ID, &seq, "run.undone", map[string]any{"runId": run.ID})
	_ = s.db.WithContext(ctx).Model(run).Update("status", "undone").Error
	run.Status = "undone"
	return run, nil
}

func (s *Service) GetRun(ctx context.Context, userID, runID string) (*model.CloudAgentRun, error) {
	var run model.CloudAgentRun
	if err := s.db.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	if run.UserID != userID {
		return nil, ErrForbidden
	}
	return &run, nil
}

func (s *Service) ListRuns(ctx context.Context, userID string) ([]model.CloudAgentRun, error) {
	var list []model.CloudAgentRun
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Limit(30).Find(&list).Error
	return list, err
}

func (s *Service) ListRunEvents(ctx context.Context, userID, runID string, afterSeq int) ([]model.CloudAgentEvent, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	var events []model.CloudAgentEvent
	err = s.db.WithContext(ctx).
		Where("run_id = ? AND seq > ?", run.ID, afterSeq).
		Order("seq ASC").
		Find(&events).Error
	return events, err
}

func (s *Service) ListPendingApprovals(ctx context.Context, userID, runID string) ([]model.CloudAgentApproval, error) {
	q := s.db.WithContext(ctx).Where("user_id = ? AND status = ?", userID, "pending")
	if runID != "" {
		q = q.Where("run_id = ?", runID)
	}
	var list []model.CloudAgentApproval
	err := q.Order("created_at ASC").Find(&list).Error
	return list, err
}

func (s *Service) SaveMemory(ctx context.Context, userID, scope, title, content string) (*model.AgentMemoryDocument, error) {
	if scope == "" {
		scope = "global"
	}
	doc := &model.AgentMemoryDocument{
		ID:      uuid.NewString(),
		UserID:  userID,
		Scope:   scope,
		Title:   strings.TrimSpace(title),
		Content: strings.TrimSpace(content),
	}
	if doc.Title == "" {
		doc.Title = "创作风格偏好记忆"
	}
	if err := s.db.WithContext(ctx).Create(doc).Error; err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *Service) ListMemories(ctx context.Context, userID string) ([]model.AgentMemoryDocument, error) {
	var list []model.AgentMemoryDocument
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&list).Error
	return list, err
}

func (s *Service) RegisterMCPServer(ctx context.Context, userID, name, transport, endpoint string) (*model.MCPServerConfig, error) {
	if transport == "" {
		transport = "streamable_http"
	}
	srv := &model.MCPServerConfig{
		ID:        "mcp_" + uuid.NewString()[:12],
		OwnerID:   userID,
		Name:      strings.TrimSpace(name),
		Transport: transport,
		Endpoint:  strings.TrimSpace(endpoint),
		ToolsJSON: `[{"name":"search_storyboard_refs","description":"检索远程影视参考镜头库"}]`,
		Enabled:   true,
	}
	if err := s.db.WithContext(ctx).Create(srv).Error; err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Service) ListMCPServers(ctx context.Context, userID string) ([]model.MCPServerConfig, error) {
	var list []model.MCPServerConfig
	err := s.db.WithContext(ctx).Where("owner_id = ?", userID).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (s *Service) nextSeq(ctx context.Context, runID string) int {
	var last model.CloudAgentEvent
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("seq DESC").First(&last).Error; err == nil {
		return last.Seq + 1
	}
	return 1
}

func (s *Service) appendEvent(ctx context.Context, runID string, seq *int, evType string, payload any) {
	b, _ := json.Marshal(payload)
	ev := model.CloudAgentEvent{
		ID:          uuid.NewString(),
		RunID:       runID,
		Seq:         *seq,
		Type:        evType,
		PayloadJSON: string(b),
	}
	_ = s.db.WithContext(ctx).Create(&ev).Error
	*seq++
}

func truncateTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return "新 Agent 创作会话"
	}
	if len(r) > 28 {
		return string(r[:28]) + "..."
	}
	return string(r)
}
