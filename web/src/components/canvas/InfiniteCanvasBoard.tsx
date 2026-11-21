import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button, Input, Select, Space, Tag, Tooltip, message } from "antd";
import { canvasApi, type CanvasOperation, type CanvasSnapshot } from "@/services/api/canvas";
import { tasksApi } from "@/services/api/tasks";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
  type ViewportTransform,
} from "@/types/canvas";

interface HistoryEntry {
  forward: CanvasOperation[];
  inverse: CanvasOperation[];
}

export type LodLevel = "block" | "thumbnail" | "full";

export function computeLodLevel(scale: number): LodLevel {
  if (scale < 0.35) return "block";
  if (scale < 0.6) return "thumbnail";
  return "full";
}

export function InfiniteCanvasBoard() {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const clientIdRef = useRef<string>(`client_${Math.random().toString(36).slice(2, 10)}`);

  const [projectId, setProjectId] = useState<string>("");
  const [projectTitle, setProjectTitle] = useState<string>("无限画布");
  const [revision, setRevision] = useState<number>(1);
  const [nodes, setNodes] = useState<CanvasNodeData[]>([]);
  const [connections, setConnections] = useState<CanvasConnection[]>([]);
  const [viewport, setViewport] = useState<ViewportTransform>({ x: 40, y: 40, k: 1 });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [connectingFromId, setConnectingFromId] = useState<string | null>(null);
  const [shareToken, setShareToken] = useState<string | null>(null);

  const [undoStack, setUndoStack] = useState<HistoryEntry[]>([]);
  const [redoStack, setRedoStack] = useState<HistoryEntry[]>([]);

  const dragStateRef = useRef<{
    mode: "pan" | "node";
    nodeId?: string;
    startClientX: number;
    startClientY: number;
    origX: number;
    origY: number;
  } | null>(null);

  const loadStarterProject = useCallback(async () => {
    try {
      const res = await canvasApi.listProjects();
      const first = res.items[0];
      if (!first) return;
      setProjectId(first.id);
      setProjectTitle(first.title);
      setRevision(first.revision);
      setShareToken(first.shareToken ?? null);
      if (first.snapshotJson) {
        const snap = JSON.parse(first.snapshotJson) as CanvasSnapshot;
        setNodes(snap.nodes ?? []);
        setConnections(snap.connections ?? []);
        if (snap.viewport && snap.viewport.k > 0) {
          setViewport(snap.viewport);
        }
      }
    } catch {
      // Guest user not logged in yet: initialize local demo nodes
      setNodes([
        {
          id: "demo_script",
          type: CanvasNodeType.Script,
          title: "剧本分镜节点 (双击或拖拉连线)",
          position: { x: 80, y: 100 },
          width: 300,
          height: 190,
          metadata: {
            prompt: "第一幕：雨夜霓虹街头，主角抬头望向全息巨幕。",
            status: "idle",
          },
        },
        {
          id: "demo_image",
          type: CanvasNodeType.Image,
          title: "AI 画面生成节点",
          position: { x: 460, y: 100 },
          width: 320,
          height: 210,
          metadata: {
            prompt: "Cyberpunk neon street in rain, 8k",
            model: "mock-flux-pro",
            status: "idle",
          },
        },
      ]);
      setConnections([
        {
          id: "demo_conn_1",
          fromNodeId: "demo_script",
          toNodeId: "demo_image",
          relation: "storyboard-output",
        },
      ]);
    }
  }, []);

  useEffect(() => {
    void loadStarterProject();
  }, [loadStarterProject]);

  const syncOperations = useCallback(
    async (ops: CanvasOperation[], recordHistory?: HistoryEntry) => {
      if (recordHistory) {
        setUndoStack((prev) => [...prev.slice(-99), recordHistory]);
        setRedoStack([]);
      }
      if (!projectId) return;
      try {
        const res = await canvasApi.patchOperations(projectId, {
          baseRevision: revision,
          clientId: clientIdRef.current,
          operations: ops,
        });
        setRevision(res.revision);
      } catch {
        // Reload latest server revision on conflict
        const latest = await canvasApi.getProject(projectId);
        setRevision(latest.project.revision);
        if (latest.project.snapshotJson) {
          const snap = JSON.parse(latest.project.snapshotJson) as CanvasSnapshot;
          setNodes(snap.nodes ?? []);
          setConnections(snap.connections ?? []);
        }
      }
    },
    [projectId, revision],
  );

  // Viewport spatial culling with 50% padding
  const visibleNodes = useMemo(() => {
    const screenW = containerRef.current?.clientWidth || 1280;
    const screenH = containerRef.current?.clientHeight || 760;
    const worldX = -viewport.x / viewport.k;
    const worldY = -viewport.y / viewport.k;
    const worldW = screenW / viewport.k;
    const worldH = screenH / viewport.k;
    const pad = 0.5 * Math.max(worldW, worldH);

    const minX = worldX - pad;
    const minY = worldY - pad;
    const maxX = worldX + worldW + pad;
    const maxY = worldY + worldH + pad;

    return nodes.filter(
      (n) =>
        n.position.x + n.width >= minX &&
        n.position.x <= maxX &&
        n.position.y + n.height >= minY &&
        n.position.y <= maxY,
    );
  }, [nodes, viewport]);

  const nodeMap = useMemo(() => {
    const m = new Map<string, CanvasNodeData>();
    for (const n of nodes) m.set(n.id, n);
    return m;
  }, [nodes]);

  const lod = computeLodLevel(viewport.k);

  const addNode = (type: CanvasNodeType) => {
    const id = `node_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`;
    const worldCenterX = Math.round((320 - viewport.x) / viewport.k);
    const worldCenterY = Math.round((200 - viewport.y) / viewport.k);
    const newNode: CanvasNodeData = {
      id,
      type,
      title: `${type.toUpperCase()} 节点`,
      position: { x: worldCenterX, y: worldCenterY },
      width: 300,
      height: 200,
      metadata: {
        prompt: "输入生成提示词...",
        model: type === CanvasNodeType.Video ? "mock-seedance-video" : "mock-flux-pro",
        status: "idle",
      },
    };
    setNodes((prev) => [...prev, newNode]);
    setSelectedId(id);
    const forward: CanvasOperation[] = [{ op: "node.add", node: newNode }];
    const inverse: CanvasOperation[] = [{ op: "node.remove", id }];
    void syncOperations(forward, { forward, inverse });
  };

  const removeSelectedNode = () => {
    if (!selectedId) return;
    const target = nodeMap.get(selectedId);
    if (!target) return;
    setNodes((prev) => prev.filter((n) => n.id !== selectedId));
    setConnections((prev) => prev.filter((c) => c.fromNodeId !== selectedId && c.toNodeId !== selectedId));
    setSelectedId(null);
    const forward: CanvasOperation[] = [{ op: "node.remove", id: target.id }];
    const inverse: CanvasOperation[] = [{ op: "node.add", node: target }];
    void syncOperations(forward, { forward, inverse });
  };

  const handleUndo = () => {
    const last = undoStack[undoStack.length - 1];
    if (!last) return;
    setUndoStack((prev) => prev.slice(0, -1));
    setRedoStack((prev) => [...prev, last]);
    applyLocalOps(last.inverse);
    void syncOperations(last.inverse);
  };

  const handleRedo = () => {
    const next = redoStack[redoStack.length - 1];
    if (!next) return;
    setRedoStack((prev) => prev.slice(0, -1));
    setUndoStack((prev) => [...prev, next]);
    applyLocalOps(next.forward);
    void syncOperations(next.forward);
  };

  const applyLocalOps = (ops: CanvasOperation[]) => {
    for (const op of ops) {
      if (op.op === "node.add") {
        setNodes((prev) => [...prev.filter((n) => n.id !== op.node.id), op.node]);
      } else if (op.op === "node.remove") {
        setNodes((prev) => prev.filter((n) => n.id !== op.id));
        setConnections((prev) => prev.filter((c) => c.fromNodeId !== op.id && c.toNodeId !== op.id));
      } else if (op.op === "node.move") {
        setNodes((prev) =>
          prev.map((n) =>
            op.ids.includes(n.id)
              ? { ...n, position: { x: n.position.x + op.dx, y: n.position.y + op.dy } }
              : n,
          ),
        );
      } else if (op.op === "connection.add") {
        setConnections((prev) => [...prev, op.connection]);
      } else if (op.op === "connection.remove") {
        setConnections((prev) => prev.filter((c) => c.id !== op.id));
      }
    }
  };

  const runNodeGeneration = async (node: CanvasNodeData) => {
    const capability =
      node.type === CanvasNodeType.Video
        ? "video"
        : node.type === CanvasNodeType.Image
          ? "image"
          : "text";
    const modelCode =
      node.metadata?.model ||
      (capability === "video"
        ? "mock-seedance-video"
        : capability === "image"
          ? "mock-flux-pro"
          : "mock-gpt-4o");

    setNodes((prev) =>
      prev.map((n) =>
        n.id === node.id ? { ...n, metadata: { ...n.metadata, status: "loading" } } : n,
      ),
    );
    try {
      const res = await tasksApi.createTask({
        creationSubmissionId: `canvas_${node.id}_${Date.now()}`,
        logicalModelId: modelCode,
        capability,
        prompt: node.metadata?.prompt || node.title,
      });
      void message.success(`节点已触发异步生成任务 (${res.task.id.slice(0, 8)})`);
      setNodes((prev) =>
        prev.map((n) =>
          n.id === node.id
            ? {
                ...n,
                metadata: {
                  ...n.metadata,
                  status: "success",
                  taskId: res.task.id,
                  url:
                    capability === "image"
                      ? "https://placehold.co/640x360/2563eb/ffffff.png?text=Generated+Frame"
                      : undefined,
                },
              }
            : n,
        ),
      );
    } catch {
      setNodes((prev) =>
        prev.map((n) =>
          n.id === node.id ? { ...n, metadata: { ...n.metadata, status: "error" } } : n,
        ),
      );
      void message.error("请先登录账号后再触发节点生成");
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {/* Top Canvas Toolbar */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          padding: "10px 16px",
          background: "var(--bg-elevated)",
          border: "1px solid var(--border-subtle)",
          borderRadius: 10,
        }}
      >
        <Space wrap>
          <strong>{projectTitle}</strong>
          <Tag color="purple">Rev #{revision}</Tag>
          <Tag color="cyan">
            Zoom: {Math.round(viewport.k * 100)}% · LOD: {lod} · 可见: {visibleNodes.length}/{nodes.length}
          </Tag>
          <Select
            placeholder="+ 添加多模态节点"
            style={{ width: 170 }}
            value={null}
            onChange={(val) => val && addNode(val as CanvasNodeType)}
            options={[
              { value: CanvasNodeType.Script, label: "+ 剧本/分镜节点" },
              { value: CanvasNodeType.Image, label: "+ AI 图像节点" },
              { value: CanvasNodeType.Video, label: "+ AI 视频节点" },
              { value: CanvasNodeType.Text, label: "+ 富文本节点" },
              { value: CanvasNodeType.Audio, label: "+ 音频配音节点" },
              { value: CanvasNodeType.BatchTable, label: "+ 批量表格节点" },
            ]}
          />
          <Button size="small" disabled={undoStack.length === 0} onClick={handleUndo}>
            撤销 ({undoStack.length})
          </Button>
          <Button size="small" disabled={redoStack.length === 0} onClick={handleRedo}>
            重做 ({redoStack.length})
          </Button>
          <Button size="small" danger disabled={!selectedId} onClick={removeSelectedNode}>
            删除选中
          </Button>
        </Space>

        <Space>
          <Button
            size="small"
            onClick={() => setViewport((v) => ({ ...v, k: Math.max(0.2, +(v.k - 0.15).toFixed(2)) }))}
          >
            - 缩小
          </Button>
          <Button size="small" onClick={() => setViewport({ x: 40, y: 40, k: 1 })}>
            100% 复位
          </Button>
          <Button
            size="small"
            onClick={() => setViewport((v) => ({ ...v, k: Math.min(2.5, +(v.k + 0.15).toFixed(2)) }))}
          >
            + 放大
          </Button>
          {projectId && (
            <Button
              size="small"
              type="primary"
              onClick={async () => {
                const res = await canvasApi.enableShare(projectId);
                setShareToken(res.shareToken);
                void message.success(`只读分享码已生成: ${res.shareToken}`);
              }}
            >
              {shareToken ? `已分享 (${shareToken})` : "生成分享链接"}
            </Button>
          )}
        </Space>
      </div>

      {/* Main Infinite Canvas Stage (DOM translate3d + Bezier Vector Underlay) */}
      <div
        ref={containerRef}
        className="canvas-stage"
        style={{
          position: "relative",
          overflow: "hidden",
          userSelect: "none",
          cursor: dragStateRef.current?.mode === "pan" ? "grabbing" : "grab",
        }}
        onWheel={(e) => {
          e.preventDefault();
          const factor = e.deltaY < 0 ? 1.08 : 0.92;
          setViewport((v) => ({
            ...v,
            k: Math.min(2.5, Math.max(0.2, +(v.k * factor).toFixed(2))),
          }));
        }}
        onMouseDown={(e) => {
          if (e.target === e.currentTarget) {
            setSelectedId(null);
            setConnectingFromId(null);
            dragStateRef.current = {
              mode: "pan",
              startClientX: e.clientX,
              startClientY: e.clientY,
              origX: viewport.x,
              origY: viewport.y,
            };
          }
        }}
        onMouseMove={(e) => {
          const st = dragStateRef.current;
          if (!st) return;
          if (st.mode === "pan") {
            const dx = e.clientX - st.startClientX;
            const dy = e.clientY - st.startClientY;
            setViewport((v) => ({ ...v, x: st.origX + dx, y: st.origY + dy }));
          } else if (st.mode === "node" && st.nodeId) {
            const dx = (e.clientX - st.startClientX) / viewport.k;
            const dy = (e.clientY - st.startClientY) / viewport.k;
            setNodes((prev) =>
              prev.map((n) =>
                n.id === st.nodeId
                  ? { ...n, position: { x: Math.round(st.origX + dx), y: Math.round(st.origY + dy) } }
                  : n,
              ),
            );
          }
        }}
        onMouseUp={() => {
          const st = dragStateRef.current;
          dragStateRef.current = null;
          if (st && st.mode === "node" && st.nodeId) {
            const current = nodeMap.get(st.nodeId);
            if (!current) return;
            const dx = current.position.x - st.origX;
            const dy = current.position.y - st.origY;
            if (dx !== 0 || dy !== 0) {
              const forward: CanvasOperation[] = [{ op: "node.move", ids: [st.nodeId], dx, dy }];
              const inverse: CanvasOperation[] = [{ op: "node.move", ids: [st.nodeId], dx: -dx, dy: -dy }];
              void syncOperations(forward, { forward, inverse });
            }
          }
        }}
      >
        {/* World Transform Layer */}
        <div
          style={{
            position: "absolute",
            inset: 0,
            transformOrigin: "0 0",
            transform: `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.k})`,
          }}
        >
          {/* Vector Connection Underlay */}
          <svg
            width={4000}
            height={3000}
            style={{ position: "absolute", left: 0, top: 0, pointerEvents: "none", overflow: "visible" }}
          >
            {connections.map((conn) => {
              const from = nodeMap.get(conn.fromNodeId);
              const to = nodeMap.get(conn.toNodeId);
              if (!from || !to) return null;
              const x1 = from.position.x + from.width;
              const y1 = from.position.y + from.height / 2;
              const x2 = to.position.x;
              const y2 = to.position.y + to.height / 2;
              const cpx = (x1 + x2) / 2;
              const d = `M ${x1} ${y1} C ${cpx} ${y1}, ${cpx} ${y2}, ${x2} ${y2}`;
              return (
                <path
                  key={conn.id}
                  d={d}
                  fill="none"
                  stroke="var(--brand-primary)"
                  strokeWidth={2.5}
                />
              );
            })}
          </svg>

          {/* Virtualized DOM Node Container */}
          {visibleNodes.map((node) => {
            const isSelected = node.id === selectedId;
            return (
              <div
                key={node.id}
                onMouseDown={(e) => {
                  e.stopPropagation();
                  setSelectedId(node.id);
                  dragStateRef.current = {
                    mode: "node",
                    nodeId: node.id,
                    startClientX: e.clientX,
                    startClientY: e.clientY,
                    origX: node.position.x,
                    origY: node.position.y,
                  };
                }}
                style={{
                  position: "absolute",
                  width: node.width,
                  height: node.height,
                  transform: `translate3d(${node.position.x}px, ${node.position.y}px, 0)`,
                  background: "var(--bg-elevated)",
                  border: isSelected ? "2px solid var(--brand-primary)" : "1px solid var(--border-subtle)",
                  borderRadius: 12,
                  padding: 12,
                  boxShadow: "0 8px 24px rgba(0,0,0,0.18)",
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "space-between",
                  cursor: "move",
                }}
              >
                {/* Node Header */}
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Space size={6}>
                    <Tag color="blue">{node.type}</Tag>
                    <strong style={{ fontSize: 13 }}>{node.title}</strong>
                  </Space>
                  <Tooltip title={connectingFromId === node.id ? "点击目标节点完成连线" : "从该节点拖出连线"}>
                    <Button
                      size="small"
                      type={connectingFromId === node.id ? "primary" : "default"}
                      onClick={(e) => {
                        e.stopPropagation();
                        if (!connectingFromId) {
                          setConnectingFromId(node.id);
                        } else if (connectingFromId !== node.id) {
                          const newConn: CanvasConnection = {
                            id: `conn_${Date.now()}`,
                            fromNodeId: connectingFromId,
                            toNodeId: node.id,
                            relation: "storyboard-output",
                          };
                          setConnections((prev) => [...prev, newConn]);
                          setConnectingFromId(null);
                          const forward: CanvasOperation[] = [{ op: "connection.add", connection: newConn }];
                          const inverse: CanvasOperation[] = [{ op: "connection.remove", id: newConn.id }];
                          void syncOperations(forward, { forward, inverse });
                        } else {
                          setConnectingFromId(null);
                        }
                      }}
                    >
                      {connectingFromId && connectingFromId !== node.id ? "连入此节点" : "连线 →"}
                    </Button>
                  </Tooltip>
                </div>

                {/* 3-Tier LOD Body */}
                {lod === "block" ? (
                  <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <span>LOD 极简色块模式</span>
                  </div>
                ) : (
                  <div style={{ flex: 1, marginTop: 8 }}>
                    <Input.TextArea
                      rows={3}
                      value={node.metadata?.prompt ?? ""}
                      onMouseDown={(e) => e.stopPropagation()}
                      onChange={(e) => {
                        const val = e.target.value;
                        setNodes((prev) =>
                          prev.map((n) =>
                            n.id === node.id ? { ...n, metadata: { ...n.metadata, prompt: val } } : n,
                          ),
                        );
                      }}
                      onBlur={() => {
                        void syncOperations([
                          {
                            op: "node.update",
                            id: node.id,
                            patch: { metadata: { prompt: node.metadata?.prompt ?? "" } },
                          },
                        ]);
                      }}
                    />
                  </div>
                )}

                {/* Node Footer */}
                {lod === "full" && (
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 8 }}>
                    <Tag
                      color={
                        node.metadata?.status === "success"
                          ? "green"
                          : node.metadata?.status === "loading"
                            ? "processing"
                            : "default"
                      }
                    >
                      {node.metadata?.status ?? "idle"}
                    </Tag>
                    <Button
                      size="small"
                      type="primary"
                      loading={node.metadata?.status === "loading"}
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        void runNodeGeneration(node);
                      }}
                    >
                      生成产物
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
