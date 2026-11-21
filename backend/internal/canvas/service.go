package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"gorm.io/gorm"
)

const EditLeaseTTL = 60 * time.Second

var (
	ErrProjectNotFound  = errors.New("canvas project not found")
	ErrForbidden        = errors.New("access to canvas project is forbidden")
	ErrRevisionConflict = errors.New("canvas baseRevision is behind server revision")
	ErrLeaseConflict    = errors.New("canvas project is currently locked by another editor lease")
)

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Viewport struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	K float64 `json:"k"`
}

type NodeData struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Position  Position       `json:"position"`
	Width     float64        `json:"width"`
	Height    float64        `json:"height"`
	ParentID  string         `json:"parentId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

type ConnectionData struct {
	ID              string  `json:"id"`
	FromNodeID      string  `json:"fromNodeId"`
	ToNodeID        string  `json:"toNodeId"`
	FromHandleID    string  `json:"fromHandleId,omitempty"`
	ToHandleID      string  `json:"toHandleId,omitempty"`
	FromAnchorRatio float64 `json:"fromAnchorRatio,omitempty"`
	ToAnchorRatio   float64 `json:"toAnchorRatio,omitempty"`
	Relation        string  `json:"relation,omitempty"`
	StoryboardRowID string  `json:"storyboardRowId,omitempty"`
}

type Snapshot struct {
	Nodes       []NodeData       `json:"nodes"`
	Connections []ConnectionData `json:"connections"`
	Viewport    Viewport         `json:"viewport"`
}

type Operation struct {
	Op         string          `json:"op"` // node.add | node.update | node.remove | node.move | connection.add | connection.remove | viewport.set
	Node       *NodeData       `json:"node,omitempty"`
	ID         string          `json:"id,omitempty"`
	IDs        []string        `json:"ids,omitempty"`
	DX         float64         `json:"dx,omitempty"`
	DY         float64         `json:"dy,omitempty"`
	Patch      map[string]any  `json:"patch,omitempty"`
	Connection *ConnectionData `json:"connection,omitempty"`
	Viewport   *Viewport       `json:"viewport,omitempty"`
}

type ApplyOperationsInput struct {
	UserID       string
	ProjectID    string
	BaseRevision int64
	ClientID     string
	LeaseToken   string
	Operations   []Operation
}

type ApplyOperationsResult struct {
	Revision int64                  `json:"revision"`
	Missing  []model.CanvasOperationLog `json:"missing,omitempty"`
}

type RealtimeMessage struct {
	Type       string      `json:"type"` // ops | cursor | presence
	ProjectID  string      `json:"projectId"`
	Revision   int64       `json:"revision,omitempty"`
	ClientID   string      `json:"clientId,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
	Payload    any         `json:"payload,omitempty"`
}

type Service struct {
	db    *gorm.DB
	rooms sync.Map // projectID -> *sync.Map of chan RealtimeMessage
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func defaultSnapshot() Snapshot {
	return Snapshot{
		Nodes: []NodeData{
			{
				ID:       "node_welcome_script",
				Type:     "script",
				Title:    "故事剧本 / 分镜输入",
				Position: Position{X: 80, Y: 120},
				Width:    300,
				Height:   200,
				Metadata: map[string]any{
					"prompt": "第一场：赛博朋克雨夜，霓虹灯映照在积水街道，女主转身看向镜头。",
					"status": "idle",
				},
			},
			{
				ID:       "node_welcome_image",
				Type:     "image",
				Title:    "首帧概念图节点",
				Position: Position{X: 480, Y: 120},
				Width:    320,
				Height:   220,
				Metadata: map[string]any{
					"prompt": "Cyberpunk rainy alley, cinematic lighting, 8k",
					"model":  "mock-flux-pro",
					"status": "idle",
				},
			},
		},
		Connections: []ConnectionData{
			{
				ID:         "conn_welcome_1",
				FromNodeID: "node_welcome_script",
				ToNodeID:   "node_welcome_image",
				Relation:   "storyboard-output",
			},
		},
		Viewport: Viewport{X: 0, Y: 0, K: 1},
	}
}

func (s *Service) CreateProject(ctx context.Context, ownerID, title string) (*model.CanvasProject, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		t = "未命名无限画布工程"
	}
	snapBytes, _ := json.Marshal(defaultSnapshot())
	proj := &model.CanvasProject{
		ID:           uuid.NewString(),
		OwnerID:      ownerID,
		Title:        t,
		Revision:     1,
		SnapshotJSON: string(snapBytes),
	}
	if err := s.db.WithContext(ctx).Create(proj).Error; err != nil {
		return nil, err
	}
	return proj, nil
}

func (s *Service) ListProjects(ctx context.Context, ownerID string) ([]model.CanvasProject, error) {
	var list []model.CanvasProject
	err := s.db.WithContext(ctx).
		Where("owner_id = ?", ownerID).
		Order("updated_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	// Auto-provision one starter canvas project if user has none yet
	if len(list) == 0 {
		created, err := s.CreateProject(ctx, ownerID, "默认影视创作画布")
		if err == nil && created != nil {
			return []model.CanvasProject{*created}, nil
		}
	}
	return list, nil
}

func (s *Service) GetProject(ctx context.Context, ownerID, projectID string) (*model.CanvasProject, error) {
	var proj model.CanvasProject
	if err := s.db.WithContext(ctx).First(&proj, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if proj.OwnerID != ownerID {
		return nil, ErrForbidden
	}
	return &proj, nil
}

func (s *Service) DeleteProject(ctx context.Context, ownerID, projectID string) error {
	proj, err := s.GetProject(ctx, ownerID, projectID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", proj.ID).Delete(&model.CanvasOperationLog{}).Error; err != nil {
			return err
		}
		return tx.Delete(proj).Error
	})
}

func (s *Service) AcquireEditLease(ctx context.Context, ownerID, projectID, clientID string) (string, time.Time, error) {
	proj, err := s.GetProject(ctx, ownerID, projectID)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	leaseOwner := clientID
	if leaseOwner == "" {
		leaseOwner = uuid.NewString()[:12]
	}
	if proj.EditLeaseOwner != "" && proj.EditLeaseOwner != leaseOwner && proj.EditLeaseExpiresAt != nil && now.Before(*proj.EditLeaseExpiresAt) {
		return "", time.Time{}, ErrLeaseConflict
	}
	exp := now.Add(EditLeaseTTL)
	if err := s.db.WithContext(ctx).Model(proj).Updates(map[string]any{
		"edit_lease_owner":      leaseOwner,
		"edit_lease_expires_at": &exp,
	}).Error; err != nil {
		return "", time.Time{}, err
	}
	return leaseOwner, exp, nil
}

func (s *Service) ApplyOperations(ctx context.Context, in ApplyOperationsInput) (*ApplyOperationsResult, error) {
	proj, err := s.GetProject(ctx, in.UserID, in.ProjectID)
	if err != nil {
		return nil, err
	}

	var result ApplyOperationsResult
	var conflict bool

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.CanvasProject
		if err := tx.First(&current, "id = ?", proj.ID).Error; err != nil {
			return err
		}

		if in.BaseRevision != current.Revision {
			conflict = true
			var missing []model.CanvasOperationLog
			_ = tx.Where("project_id = ? AND revision > ?", current.ID, in.BaseRevision).
				Order("revision ASC").
				Find(&missing).Error
			result = ApplyOperationsResult{
				Revision: current.Revision,
				Missing:  missing,
			}
			return nil
		}

		var snap Snapshot
		if current.SnapshotJSON != "" {
			_ = json.Unmarshal([]byte(current.SnapshotJSON), &snap)
		}
		if snap.Viewport.K == 0 {
			snap.Viewport.K = 1
		}

		snap = applyOpsToSnapshot(snap, in.Operations)
		newSnapBytes, err := json.Marshal(snap)
		if err != nil {
			return err
		}

		nextRev := current.Revision + 1
		upd := tx.Model(&model.CanvasProject{}).
			Where("id = ? AND revision = ?", current.ID, current.Revision).
			Updates(map[string]any{
				"revision":      nextRev,
				"snapshot_json": string(newSnapBytes),
			})
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected == 0 {
			conflict = true
			result = ApplyOperationsResult{Revision: current.Revision + 1}
			return nil
		}

		opsBytes, _ := json.Marshal(in.Operations)
		opLog := model.CanvasOperationLog{
			ID:        uuid.NewString(),
			ProjectID: current.ID,
			Revision:  nextRev,
			ClientID:  in.ClientID,
			OpsJSON:   string(opsBytes),
		}
		if err := tx.Create(&opLog).Error; err != nil {
			return err
		}

		result = ApplyOperationsResult{Revision: nextRev}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if conflict {
		return &result, ErrRevisionConflict
	}

	s.Broadcast(in.ProjectID, RealtimeMessage{
		Type:       "ops",
		ProjectID:  in.ProjectID,
		Revision:   result.Revision,
		ClientID:   in.ClientID,
		Operations: in.Operations,
	})
	return &result, nil
}

func applyOpsToSnapshot(snap Snapshot, ops []Operation) Snapshot {
	for _, op := range ops {
		switch op.Op {
		case "node.add":
			if op.Node != nil {
				replaced := false
				for i := range snap.Nodes {
					if snap.Nodes[i].ID == op.Node.ID {
						snap.Nodes[i] = *op.Node
						replaced = true
						break
					}
				}
				if !replaced {
					snap.Nodes = append(snap.Nodes, *op.Node)
				}
			}

		case "node.update":
			for i := range snap.Nodes {
				if snap.Nodes[i].ID == op.ID {
					if title, ok := op.Patch["title"].(string); ok {
						snap.Nodes[i].Title = title
					}
					if w, ok := toFloat64(op.Patch["width"]); ok && w > 0 {
						snap.Nodes[i].Width = w
					}
					if h, ok := toFloat64(op.Patch["height"]); ok && h > 0 {
						snap.Nodes[i].Height = h
					}
					if posMap, ok := op.Patch["position"].(map[string]any); ok {
						if x, ok := toFloat64(posMap["x"]); ok {
							snap.Nodes[i].Position.X = x
						}
						if y, ok := toFloat64(posMap["y"]); ok {
							snap.Nodes[i].Position.Y = y
						}
					}
					if metaPatch, ok := op.Patch["metadata"].(map[string]any); ok {
						if snap.Nodes[i].Metadata == nil {
							snap.Nodes[i].Metadata = make(map[string]any)
						}
						for k, v := range metaPatch {
							snap.Nodes[i].Metadata[k] = v
						}
					}
					break
				}
			}

		case "node.move":
			idSet := make(map[string]struct{}, len(op.IDs))
			for _, id := range op.IDs {
				idSet[id] = struct{}{}
			}
			for i := range snap.Nodes {
				if _, ok := idSet[snap.Nodes[i].ID]; ok {
					snap.Nodes[i].Position.X += op.DX
					snap.Nodes[i].Position.Y += op.DY
				}
			}

		case "node.remove":
			filteredNodes := make([]NodeData, 0, len(snap.Nodes))
			for _, n := range snap.Nodes {
				if n.ID != op.ID {
					filteredNodes = append(filteredNodes, n)
				}
			}
			snap.Nodes = filteredNodes
			filteredConns := make([]ConnectionData, 0, len(snap.Connections))
			for _, c := range snap.Connections {
				if c.FromNodeID != op.ID && c.ToNodeID != op.ID {
					filteredConns = append(filteredConns, c)
				}
			}
			snap.Connections = filteredConns

		case "connection.add":
			if op.Connection != nil {
				snap.Connections = append(snap.Connections, *op.Connection)
			}

		case "connection.remove":
			filtered := make([]ConnectionData, 0, len(snap.Connections))
			for _, c := range snap.Connections {
				if c.ID != op.ID {
					filtered = append(filtered, c)
				}
			}
			snap.Connections = filtered

		case "viewport.set":
			if op.Viewport != nil {
				snap.Viewport = *op.Viewport
			}
		}
	}
	return snap
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func (s *Service) EnableShare(ctx context.Context, ownerID, projectID string) (string, error) {
	proj, err := s.GetProject(ctx, ownerID, projectID)
	if err != nil {
		return "", err
	}
	if proj.ShareToken != nil && *proj.ShareToken != "" {
		return *proj.ShareToken, nil
	}
	tok := "sh_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if err := s.db.WithContext(ctx).Model(proj).Update("share_token", &tok).Error; err != nil {
		return "", err
	}
	return tok, nil
}

func (s *Service) DisableShare(ctx context.Context, ownerID, projectID string) error {
	proj, err := s.GetProject(ctx, ownerID, projectID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(proj).Update("share_token", nil).Error
}

func (s *Service) GetSharedProject(ctx context.Context, shareToken string) (*model.CanvasProject, error) {
	var proj model.CanvasProject
	if err := s.db.WithContext(ctx).Where("share_token = ?", shareToken).First(&proj).Error; err != nil {
		return nil, ErrProjectNotFound
	}
	return &proj, nil
}

func (s *Service) Subscribe(projectID string) (chan RealtimeMessage, func()) {
	ch := make(chan RealtimeMessage, 32)
	val, _ := s.rooms.LoadOrStore(projectID, &sync.Map{})
	room := val.(*sync.Map)
	room.Store(ch, struct{}{})
	unsubscribe := func() {
		room.Delete(ch)
		close(ch)
	}
	return ch, unsubscribe
}

func (s *Service) Broadcast(projectID string, msg RealtimeMessage) {
	val, ok := s.rooms.Load(projectID)
	if !ok {
		return
	}
	room := val.(*sync.Map)
	room.Range(func(key, _ any) bool {
		if ch, ok := key.(chan RealtimeMessage); ok {
			select {
			case ch <- msg:
			default:
			}
		}
		return true
	})
}
