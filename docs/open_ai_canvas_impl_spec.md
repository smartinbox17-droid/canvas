# open-ai-canvas 复刻 · 第二部分：实现级规格（Implementation Spec）

> 本文是 [open_ai_canvas_dev_plan.md](file:///home/antigravity/.gemini/jetski/brain/v1/default/jetski_remote_agents_3p/2a926a20-ea30-41b1-89cf-8a9e7283a460/open_ai_canvas_dev_plan.md) 的续篇。第一部分讲“做什么 / 用什么 / 什么顺序”，本篇讲“**数据长什么样、接口怎么定、核心算法怎么写**”。字段与路由均取自参考仓库源码（`web/src/types/canvas.ts`、`backend/internal/model/models_task.go`、`backend/internal/payment/rpc.go`、`plugin-packages/*/manifest.json`、router 注册表）。

---

## 目录
1. 核心数据结构（前端 TS / 后端 Go）
2. API 清单与关键契约
3. 声明式插件 Manifest v2 + 表达式 DSL 完整规格
4. 支付插件 RPC 协议
5. 核心算法伪代码（10 个）
6. 环境变量参考 & 本地开发启动
7. MVP 裁剪路径 & 人员配置

---

## 1. 核心数据结构

### 1.1 画布（`web/src/types/canvas.ts`）

```ts
export type Position = { x: number; y: number };
export type ViewportTransform = { x: number; y: number; k: number }; // k = 缩放

export enum CanvasNodeType {
  Image = "image", Text = "text", Drawing = "drawing", Script = "script",
  Skill = "skill", Config = "config", Video = "video", Audio = "audio",
  Frame = "frame", Panorama = "panorama", ColorGrade = "colorgrade",
  MediaConversion = "media-conversion", BatchTable = "batch-table",
}
// 插件贡献的节点类型与内置类型共用同一个持久化字段
export type PluginCanvasNodeType = string & { readonly __pluginCanvasNodeType?: unique symbol };
export type CanvasNodeTypeId = CanvasNodeType | PluginCanvasNodeType;

export type CanvasNodeStatus = "idle" | "success" | "loading" | "error";

export type CanvasNodeData = {
  id: string;
  type: CanvasNodeTypeId;
  title: string;
  createdAt?: string; updatedAt?: string;
  position: Position;
  width: number; height: number;
  parentId?: string;            // Frame/文件夹嵌套
  metadata?: CanvasNodeMetadata; // 约 400 行的大联合字段，按节点类型取用
};

export type CanvasConnection = {
  id: string;
  fromNodeId: string; toNodeId: string;
  fromHandleId?: string; toHandleId?: string;
  fromAnchorRatio?: number; toAnchorRatio?: number; // 0~1 锚点在边上的位置
  relation?: "storyboard-output" | "storyboard-asset-reference" | "batch-output" | "batch-input";
  storyboardRowId?: string;
};
```

**`CanvasNodeMetadata` 关键分组**（实现时建议拆成判别联合 + 公共字段）：

| 分组 | 主要字段 |
|---|---|
| 生成通用 | `prompt, model, producedModel, size, quality, seconds, references[], taskId, taskDurationMs, status, errorDetails` |
| 媒体 | `url, thumbnailUrl, mimeType, naturalWidth/Height, durationMs, versions: CanvasMediaVersion[], activeVersionId` |
| 工作流 | `workflowKind: "free"\|"script"\|"story_input"\|"character"\|"scene"\|"storyboard"\|"shot"\|"final"\|"styleboard"\|"reference_set"\|"reference_video"\|"action_board"` |
| 视频编辑 | `videoEditOperation: text_to_video \| image_to_video \| reference_to_video \| extend \| inpaint \| replace_element \| camera_motion \| style_transfer \| audio_to_video \| compare_versions \| concat \| enhance \| depth` |
| 分镜 | `storyboard: StoryboardData { rows: StoryboardRow[], shotDuration, shotCount, videoInputMode: "direct"\|"keyframe" }` |
| 批量 | `batchTable: CanvasBatchTableData`、`generationBatch: CanvasGenerationBatch` |
| 技能 | `skill: CanvasSkillSnapshot { id, name, category, outputMode: text\|json\|image_prompt\|workflow }` |
| 短剧回链 | `drama: CanvasDramaRef { dramaId, episodeId, shotId, segmentIndex }` |
| 其他 | `colorGrade, mediaConversion, artCritique, portraitTexture, styleExecutionPlan, cameraControl, subtitles(SrtEntry[]), generationSpec` |

**`StoryboardRow`（分镜行，导演级字段）**：
```ts
type StoryboardRow = {
  id: string; index: number; durationSec: number;
  sceneDescription: string; dialogue: string; viewerPOV: string;
  performanceBlocking: string; shotSize: string; emotion: string;
  lightingAndAtmosphere: string; audioEffects: string;
  camera: string; motion: string; timeBeats: string;
  imageGenerationPrompt: string; videoMotionPrompt: string;
  imagePromptTemplateVariables?: Record<string, string>;
  videoPromptTemplateVariables?: Record<string, string>;
  mustHave: string[]; optionalDetails: string[];
  continuityOut: string;      // 交给下一镜的连续性描述
  negativePrompt: string;
  assetBindings: { assetId: string; role: StoryboardAssetRole; name: string; url?: string }[];
  // role: character|environment|wardrobe|prop|weapon|style|motion|audio
  sourceStartMs?: number; sourceEndMs?: number; keyframeTimeMs?: number; // 原视频拆解
  imageNodeId?: string; videoNodeId?: string;  // 生成结果节点回链
  status?: CanvasNodeStatus; errorDetails?: string;
  previsSceneId?: string;     // 关联 3D 预演场景
};
```

**批量生成状态机**：
```
Batch:      queued → running → completed | partial_failed | cancelled
BatchItem:  waiting → submitting → queued → running → succeeded | failed | cancelled
```

### 1.2 画布操作（增量同步单元）

```ts
type CanvasOperation =
  | { op: "node.add";        node: CanvasNodeData }
  | { op: "node.update";     id: string; patch: Partial<CanvasNodeData> }   // 深合并 metadata
  | { op: "node.remove";     id: string }
  | { op: "node.move";       ids: string[]; dx: number; dy: number }
  | { op: "connection.add";  connection: CanvasConnection }
  | { op: "connection.remove"; id: string }
  | { op: "viewport.set";    viewport: ViewportTransform };

type CanvasOperationBatch = {
  baseRevision: number;     // 客户端已知版本
  clientId: string;         // 回声过滤
  leaseToken?: string;      // 编辑租约（单写者）
  operations: CanvasOperation[];
};
```

### 1.3 后端任务模型（`model.Task`，节选关键字段）

```go
type Task struct {
  ID, UserID, ProjectID string
  Type        string      // image|video|audio|text|agent|...
  Status      TaskStatus  // queued|running|polling|succeeded|failed|cancelled
  Stage       string; Progress int
  Prompt, Operation, Provider, Model string
  LogicalModelID, LogicalModelRevisionID, RouteID, ChannelModelID string // 路由解析结果
  ProtocolBindingJSON string   // 冻结的插件协议绑定
  RouteRun   int               // 仅用户主动重试时 +1；租约恢复不换路由世代
  BillingOrderID string; AuthorizedChargeMicrocredits int64
  ProviderRequestID string     // 上游 taskId
  ProviderCancelStatus / Attempts / NextCheckAt  // 上游取消补偿
  PollStage string; NextPollAt *time.Time        // 分离式轮询
  LeaseOwner string; LeaseExpiresAt *time.Time   // worker 租约
  InputJSON, ResultJSON string
  MediaRecoveryJSON string     // 加密的媒体落盘断点，仅服务端
  MediaStage string; TextDraft string
  Error string; Attempts int
  CreationSubmissionID *string // uniqueIndex → 幂等提交
  SubmissionHash string
  AgentRunID, GenerationID, ApprovalID string
  CancellationSource, CancellationActorID string
  StartedAt, CompletedAt *time.Time; CreatedAt, UpdatedAt time.Time
}
// 复合索引：idx_tasks_claim(status, lease_expires_at, created_at)
//          idx_tasks_user_project_created(user_id, project_id, created_at)
//          idx_tasks_provider_cancel(status, provider_cancel_status, next_check_at)
```

### 1.4 其他核心表（关键字段）

| 表 | 关键字段 |
|---|---|
| `users` | id, email, phone, password_hash(bcrypt), role(user/admin), group_id, status, referral_code, inviter_id |
| `sessions` | id, user_id, token_hash, expires_at, ip, ua（Cookie `HttpOnly; SameSite=Lax`） |
| `credit_accounts` | user_id, balance_microcredits(int64), frozen_microcredits, version(乐观锁) |
| `credit_ledger` | id, user_id, delta, balance_after, kind(recharge/reserve/settle/refund/redeem/commission), ref_type, ref_id, idem_key(unique) |
| `billing_orders` | id, user_id, task_id, status(reserved/settled/refunded), reserved, actual, pricing_snapshot_json |
| `payment_orders` | id, user_id, product_id, amount_cents, provider_id, config_id, status(pending/paid/closed/failed), out_trade_no(unique), paid_at |
| `channels` / `channel_models` | 渠道：plugin_id, base_url, secrets(AES-GCM 加密), weight, priority, concurrency；渠道模型：upstream_model, capability, enabled, health |
| `logical_models` / `revisions` / `routes` | 逻辑模型（用户可见）→ 修订版 → 多条路由（priority + weight → channel_model） |
| `model_pricings` | model, capability, unit(per_image/per_second/per_1k_tokens), price, param_multipliers_json |
| `canvas_projects` | id, owner_id, team_id, title, revision(int64), snapshot_json/分片, share_token, edit_lease_owner/expires_at |
| `drama_*` | dramas / chapters / episodes / shots / segments / takes / cast / looks / jobs |
| `cloud_agent_conversations / runs / executions / events / approvals` | run: status, model, tool_budget; event: seq(单调), type, payload; approval: tool, args_json, decision |
| `plugins` / `plugin_installations` | id, version, manifest_json, package_sha256, enabled, config(加密) |

---

## 2. API 清单与关键契约

### 2.1 统一约定
- 前缀 `/api/v1`；响应信封：
```json
{ "code": 0, "data": { }, "msg": "ok", "reason": "" }
```
  `code != 0` 为业务错误；`reason` 是机器可读错误码（如 `INSUFFICIENT_CREDITS`、`ROUTE_UNAVAILABLE`），前端据此 i18n。
- 鉴权：Session Cookie + CSRF（双提交 `X-CSRF-Token`）；Agent 运行时回调使用 Bearer `YINGCE_AGENT_TOKEN`。
- 分页：`?page=1&pageSize=20` → `data: { items, total }`。
- 幂等：创建类请求可带 `Idempotency-Key` / `creationSubmissionId`。

### 2.2 路由分组（源码实际注册，节选）

| 分组 | 路由 |
|---|---|
| 认证 | `POST /auth/login` `/auth/register` `/auth/logout` `/auth/email-code` `/auth/verification[/login\|/bind]` `/auth/password-reset[-code]`；`GET /auth/session` `/auth/settings`；OIDC `GET /auth/oidc/start` `/auth/oidc/callback`；微信 `/auth/wechat/start\|callback\|registration`；LinuxDo `/auth/linuxdo/start\|callback` |
| 画布 | `GET/DELETE /canvas-projects[/:id]`；`PATCH /canvas-projects/:id/operations`；`POST /canvas-projects/:id/edit-lease`；`GET /canvas-projects/:id/realtime`(WebSocket)；`GET /canvas-projects/:id/sync-status`；分享 `GET/POST/DELETE /canvas-projects/:id/share`；导入 `POST /canvas-projects/:id/import/libtv\|tapnow` |
| 任务 | `POST /tasks` `/tasks/quote`；`GET /tasks` `/tasks/:id` `/tasks/:id/logs`；`GET /tasks/:id/text-events`(SSE) `/tasks/:id/text-deltas`；`POST /tasks/:id/cancel\|retry\|query-provider\|recover-media\|text-replay-complete`；工作台 `GET/POST /workbench/tasks` |
| Agent | `POST /agent/runs`；`GET /agent/runs/:id` `/agent/runs/:id/events`(SSE)；`POST /agent/runs/:id/messages\|cancel\|undo\|interjections`；`POST /agent/runs/:id/approvals/:approvalId/decision`；`GET /agent/conversations[/:id[/runs]]`；记忆 `GET /agent/memories[/export\|/proposals\|/revisions\|/settings]`；`GET /agent/capabilities`；`POST /agent/generate/stream` |
| 短剧 | `POST/GET /dramas`；`POST /dramas/estimate`；`POST /dramas/:id/start\|pause\|retry-failed`；`GET/PUT /dramas/:id/draft`；角色 `/dramas/:id/cast[...]`（import/regenerate/looks/voice/image-prompt/impact）；分集 `/dramas/:id/episodes/:episodeId/{approve,compose,rescript,jianying-draft[/preview],segments/generate,segments/prompts,segments/:index/reprocess,segments/:index/current-take,segments/:index/video}`；`GET /dramas/:id/jobs[/:jobId]` `/dramas/:id/takes` `/dramas/:id/canvas` |
| 电商 | `/ecommerce/projects[/:id[/rounds\|/tasks\|/rounds-convert]]`、`/ecommerce/creative-directions[/:id/select]`、`/ecommerce/knowledge/*`、`/ecommerce/skills/prompt` |
| 支付 | `GET /payments/products\|providers\|settings\|orders[/:id[/checkout]]`；`POST /payments/orders` `/payments/orders/:id/close\|query\|checkout/refresh`；回调 `GET/POST /payments/notify/:providerId/:configId`；`GET /payments/return/:providerId` |
| 后台 | `/admin/users|user-groups|channels|channel-models|logical-models|model-pricings|billing-orders|redeem-batches|referral/commissions|skills|prompt-templates|api-logs|login-logs|email-logs|analytics/*|traffic/*|storage/stats|system-performance|system-update|settings/{appearance,credits,email,features,oss,registration,turnstile,wechat,linuxdo,referral,runtime-policy,response-interception,billing-auto-refund,drawing-engine,inspiration-feed}` |

### 2.3 关键契约示例

**画布增量写入** `PATCH /canvas-projects/:id/operations`
```json
// req
{ "baseRevision": 128, "clientId": "c_9f2", "leaseToken": "l_abc",
  "operations": [
    { "op": "node.add", "node": { "id": "n1", "type": "image", "title": "主角",
      "position": {"x": 120, "y": 80}, "width": 320, "height": 320,
      "metadata": { "prompt": "赛博朋克少女", "status": "idle" } } },
    { "op": "connection.add", "connection": { "id": "e1", "fromNodeId": "n0", "toNodeId": "n1" } }
  ] }
// resp 成功
{ "code": 0, "data": { "revision": 129 } }
// resp 冲突（baseRevision 落后）
{ "code": 409, "reason": "REVISION_CONFLICT", "data": { "revision": 131, "missing": [/* ops 129..131 */] } }
```
WebSocket `/realtime` 推送：`{ "type": "ops", "revision": 129, "clientId": "c_9f2", "operations": [...] }`，客户端按 `clientId` 过滤回声。

**创建生成任务** `POST /tasks`
```json
{ "creationSubmissionId": "uuid", "projectId": "p1", "type": "video",
  "logicalModelId": "lm_seedance", "operation": "image_to_video",
  "request": {
    "capability": "video", "model": "seedance-1.0-pro", "prompt": "镜头缓慢推近",
    "images": [{ "id": "a1", "url": "https://.../f.png", "kind": "image", "role": "first_frame", "order": 0 }],
    "output": { "count": 1, "duration": 5, "aspectRatio": "16:9", "resolution": "1080p", "generateAudio": false },
    "providerOptions": { "volcengine": { "camera_fixed": false } }
  },
  "canvas": { "nodeId": "n1" } }
// resp
{ "code": 0, "data": { "task": { "id": "t1", "status": "queued", "billingOrderId": "b1",
  "authorizedChargeMicrocredits": 2500000 } } }
```
`POST /tasks/quote` 同请求体，返回 `{ estimateMicrocredits, breakdown[] }`，不扣费。

**文本流** `GET /tasks/:id/text-events`（SSE，支持 `Last-Event-ID` 续传）
```
id: 17
event: delta
data: {"text":"第一幕，","seq":17}

event: done
data: {"status":"succeeded","usage":{"inputTokens":812,"outputTokens":1430}}
```

**Agent 审批** `POST /agent/runs/:id/approvals/:approvalId/decision`
```json
{ "decision": "approve", "remember": "session", "editedArgs": { "count": 2 } }
```
事件流 `GET /agent/runs/:id/events` 事件类型：`run.started / message.delta / tool.call / tool.result / approval.required / approval.resolved / canvas.ops / task.created / run.completed / run.failed / run.cancelled`，每条带单调 `seq`。

---

## 3. 声明式插件 Manifest v2 + 表达式 DSL

### 3.1 包结构
```
openai-videos/
  manifest.json          # 唯一真相源
  README.md
  docs/interface.md      # 构建时拼接进 manifest.documentation
→ build-packages.sh 打包为 openai-videos.yingce-plugin（zip + sha256 校验）
→ generate-catalog.mjs 生成插件市场目录
```

### 3.2 完整示例（基于 `openai-videos`，异步视频：create → poll → result → cancel）
```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "openai-videos",
  "name": "OpenAI Videos / Sora",
  "version": "2.1.0",
  "permissions": ["generation.run", "media.read"],
  "configuration": { "fields": [
    { "name": "apiKey", "type": "secret", "label": "API Key", "required": true }
  ]},
  "contributes": { "providers": [{
    "id": "newapi",
    "label": "OpenAI Videos / Sora",
    "capabilities": ["video"],
    "scopes": ["admin.system-channel", "user.custom-channel", "canvas", "creation", "agent"],
    "baseUrl": "https://api.openai.com",
    "requiresPublicMediaUrls": false,
    "auth": { "type": "bearer", "field": "apiKey" },
    "parameters": [
      { "name": "duration", "type": "integer", "min": 4, "max": 12, "default": 8 },
      { "name": "aspectRatio", "type": "string", "values": ["16:9", "9:16", "1:1"] }
    ],
    "request": {
      "create": {
        "method": "POST", "path": "/v1/videos", "contentType": "multipart/form-data",
        "body": {
          "model":   { "$ref": "request.model" },
          "prompt":  { "$ref": "request.prompt" },
          "seconds": { "$toString": { "$ref": "request.duration" } },
          "size":    { "$omitEmpty": { "$ref": "request.aspectRatio" } },
          "resolution_name": { "$omitEmpty": { "$ref": "request.resolution" } }
        },
        "files": [{
          "name": "input_reference", "filename": "input-reference.png",
          "source": { "$filter": {
            "from": { "$sortByOrder": { "$ref": "request.images" } },
            "as": "media",
            "where": { "$ne": [ { "$ref": "media.role" }, "mask" ] } } }
        }]
      },
      "poll":   { "method": "GET",    "path": "/v1/videos/{{taskId}}" },
      "cancel": { "method": "DELETE", "path": "/v1/videos/{{taskId}}" },
      "result": { "method": "GET",    "path": "/v1/videos/{{taskId}}/content",
                  "headers": { "Accept": "video/mp4" } }
    },
    "response": {
      "taskId":  { "$coalesce": [ {"$ref":"response.id"}, {"$ref":"response.task_id"}, {"$ref":"response.data.id"} ] },
      "status":  { "$coalesce": [ {"$ref":"response.status"}, {"$ref":"response.state"}, "pending" ] },
      "statusMap": { "queued": "pending", "in_progress": "processing", "completed": "succeeded", "failed": "failed" },
      "message": { "$coalesce": [ {"$ref":"response.error.message"}, {"$ref":"response.message"} ] },
      "videos":  { "$coalesce": [ {"$ref":"response.url"}, {"$ref":"response.video_url"} ] },
      "errorPaths": ["error.code"], "messagePaths": ["error.message"],
      "resultEphemeral": true
    }
  }]}
}
```

### 3.3 表达式 DSL 运算符（求值器需实现）

| 类别 | 运算符 | 语义 |
|---|---|---|
| 取值 | `$ref` | 点路径取值；根命名空间：`request` `config` `response` `taskId` 及 `$filter/as` 绑定的局部变量 |
| 空值 | `$coalesce` `$omitEmpty` `$default` | 首个非空；空则从对象中删除该键 |
| 类型 | `$toString` `$toNumber` `$toBool` `$json` | 类型转换 / JSON 序列化 |
| 逻辑 | `$if{condition,then,else}` `$eq` `$ne` `$gt` `$lt` `$and` `$or` `$not` `$in` | 条件 |
| 集合 | `$filter{from,as,where}` `$map{from,as,to}` `$len` `$first` `$sortByOrder` `$pluck` | 媒体列表处理 |
| 字符串 | `$concat` `$template`（`{{var}}`）`$lower` `$upper` | 拼接 |
| 媒体 | `$dataUrl` `$base64` `$publicUrl` | 引用转码（`requiresPublicMediaUrls` 时宿主先上传公网存储） |

**求值器要求**：纯函数、无 IO；深度上限（如 32）与节点数上限防 DoS；未知运算符报错而非忽略；`$omitEmpty` 在对象组装阶段生效。

### 3.4 宿主执行流程
```
ResolveRoute → 解密 channel 配置 → 构建上下文 {request, config}
→ eval(create) → HTTP（SSRF 守卫：禁私网，除非白名单）
→ eval(response) 得 {taskId,status,...}
→ status ∈ {succeeded} ? 下载结果 : 写 ProviderRequestID + NextPollAt，进入分离式轮询
→ poll → statusMap 归一化 → succeeded 时调用 result（若声明）→ resultEphemeral=true 立即落盘到对象存储
```

---

## 4. 支付插件 RPC 协议（`yingce.payment/v1`）

支付插件以**子进程**运行（`backend/payment-plugins/rpc_runner.go`），stdin 写入一条 JSON，stdout 读一条 JSON，超时即杀。

```go
type rpcRequest struct {
  Version   string              `json:"version"`   // "yingce.payment/v1"
  Operation string              `json:"operation"` // 见下
  Config    Config              `json:"config,omitempty"`     // 商户号/密钥（解密后）
  Request   any                 `json:"request,omitempty"`
  Headers   map[string][]string `json:"headers,omitempty"`    // 仅 verify_notification
  Body      string              `json:"bodyBase64,omitempty"` // 原始回调体
  BillDate  string              `json:"billDate,omitempty"`
}
type rpcResponse struct {
  OK bool `json:"ok"`; Error, Code, Message string; Data json.RawMessage `json:"data,omitempty"`
}
```

| operation | request → data |
|---|---|
| `validate_config` | – → `{ valid }` |
| `create_order` | `{ outTradeNo, amountCents, subject, notifyUrl, returnUrl, clientIp }` → `{ checkoutType: "qrcode"\|"redirect"\|"form", codeUrl?, redirectUrl?, formHtml?, expiresAt }` |
| `query_order` | `{ outTradeNo }` → `{ status: "pending"\|"paid"\|"closed", paidAt, providerTradeNo, amountCents }` |
| `close_order` | `{ outTradeNo }` → `{ closed }` |
| `verify_notification` | headers + bodyBase64 → `{ verified, outTradeNo, status, amountCents, ackBody }`（宿主原样回写 `ackBody`，如 `success`） |
| `download_trade_bill` | `{ billDate }` → `{ rows[] }`（对账） |

官方插件：`official-payment-alipay-page`、`-wechat-native`、`-epay`、`-xunhupay`、`-zhifufm`。

**入账铁律**：回调验签成功 → 再 `query_order` 二次确认金额 → 事务内 `payment_orders` 状态 CAS `pending→paid` + 写 `credit_ledger(idem_key=out_trade_no)` + 余额累加 → 发放推广佣金。

---

## 5. 核心算法伪代码

### 5.1 任务认领与租约（多实例安全）
```go
const leaseTTL = 45 * time.Second; const heartbeat = 15 * time.Second

func (w *Worker) claimLoop(ctx) {
  sem := make(chan struct{}, cfg.WorkerConcurrency) // CANVAS_WORKER_CONCURRENCY
  for {
    sem <- struct{}{}
    task := claimOne(ctx) // 无任务则 sleep(1s) + Redis 通知唤醒
    go func() { defer func(){ <-sem }(); w.run(ctx, task) }()
  }
}

func claimOne(ctx) *Task {
  // PostgreSQL：FOR UPDATE SKIP LOCKED；SQLite：单写者 + 条件 UPDATE
  tx.Raw(`SELECT id FROM tasks
          WHERE (status='queued') OR (status IN ('running','polling') AND lease_expires_at < now())
          ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`)
  tx.Exec(`UPDATE tasks SET status='running', lease_owner=?, lease_expires_at=now()+45s,
           attempts=attempts+1 WHERE id=?`, workerID, id)
}

func (w *Worker) run(ctx, t) {
  stop := every(heartbeat, func(){
    n := db.Exec(`UPDATE tasks SET lease_expires_at=now()+45s WHERE id=? AND lease_owner=?`, t.ID, workerID)
    if n == 0 { cancelLocal() } // 租约被他人接管 → 立即放弃，避免双写
  }); defer stop()
  // 每次写状态都带 WHERE lease_owner=? 作为 fencing
}
```

### 5.2 路由选择与故障转移（DispatchState）
```go
func Dispatch(t *Task) Result {
  st := loadDispatchState(t.ID, t.RouteRun) // 记录本世代已尝试的 route
  routes := revision(t.LogicalModelRevisionID).Routes
  for _, r := range orderBy(routes, priority asc) {
    if st.Tried(r.ID) || !health.OK(r.ChannelModelID) || !channelSem(r.ChannelID).TryAcquire() { continue }
    // 同优先级内按 weight 加权随机
    res, err := invokePlugin(r, t)
    channelSem.Release()
    switch classify(err) {
    case nil:              return res
    case Retryable:        st.MarkTried(r.ID); health.Penalize(r); continue // 429/5xx/超时/连接错误
    case NonRetryable:     return Fail(err)   // 400 参数错/内容审核/余额不足 → 不切换
    case SubmittedUnknown: return Poll(res)   // 已提交但响应丢失 → 绝不重复提交，转轮询
    }
  }
  return Fail(ROUTE_UNAVAILABLE)
}
```
> 关键：**一旦上游已接受（拿到 taskId），禁止切换路由**，否则重复扣上游费用。

### 5.3 计费：预扣 → 结算 → 退款
```go
func Reserve(tx, user, task) {
  price := pricing.Quote(task.Model, task.Request) // 单价 × count × duration × 参数倍率(分辨率/音频)
  acc := lockAccount(tx, user)                     // SELECT ... FOR UPDATE 或 version CAS
  if acc.Balance - acc.Frozen < price { return ErrInsufficient }
  acc.Frozen += price
  insert(billing_orders{status:"reserved", reserved: price, pricing_snapshot})
  insert(credit_ledger{kind:"reserve", delta:-price, idem_key:"reserve:"+task.ID})
}
func Settle(tx, order, actual) {           // 成功：actual 取上游 usage 或产出数量重算
  acc.Frozen -= order.Reserved; acc.Balance -= actual
  ledger("settle", idem="settle:"+order.ID)
}
func Refund(tx, order) {                    // 失败/取消（上游未计费）
  acc.Frozen -= order.Reserved; ledger("refund", idem="refund:"+order.ID)
}
// 自动退款策略：admin/settings/billing-auto-refund 可配置“上游已受理但失败”的退款比例
```
所有金额使用 `int64` 微积分（microcredits，1 积分 = 1e6），禁止浮点。

### 5.4 分离式轮询退避
```go
func nextPollDelay(elapsed time.Duration, capability string) time.Duration {
  switch {
  case elapsed < 30*time.Second: return 3 * time.Second
  case elapsed < 2*time.Minute:  return 5 * time.Second
  case elapsed < 10*time.Minute: return 15 * time.Second
  default:                       return 30 * time.Second
  }
} // 加 ±20% jitter；超过能力级最大时长（图 10min / 视频 60min）→ 失败并触发 provider cancel
// 轮询任务不占 worker 槽：status='polling' + next_poll_at，由 poller 扫描 next_poll_at<=now 认领
```

### 5.5 媒体落盘与断点恢复
```
下载临时 URL（流式、限大小、mimetype 嗅探）→ 写本地/对象存储（S3/COS/七牛/本地）
→ 每完成一个产物更新加密 MediaRecoveryJSON {done:[...], pending:[...]}
→ 进程崩溃后租约过期，新 worker 读取断点只补未完成项
→ 用户也可 POST /tasks/:id/recover-media 手动重试
```

### 5.6 上游取消补偿
```
用户取消 → 本地 status=cancelled, ProviderCancelStatus=pending
→ canceller 扫描 idx_tasks_provider_cancel：调用插件 cancel → 成功 = cancelled
→ 失败按 1m/5m/15m/1h 退避重试，超过 N 次标记 failed（记录 ProviderCancelError，供对账）
```

### 5.7 短剧编排器 tick
```go
func (o *Orchestrator) Tick(drama) {        // 每 2~5s，或 job 完成事件驱动
  if drama.Paused { return }
  switch drama.Stage {
  case "script":     ensureJob(LLM 剧本→章节→分集→分镜)
  case "cast":       for c in cast where !c.ImageReady { ensureJob(角色定妆图 + looks) }
  case "segments":
    for ep in episodes where ep.Approved {
      for seg in slice(ep.Shots, maxSegSec) {   // 按模型最大时长切片（5/10/15s）
        if seg.Index>0 && chain && !prev.TailFrameReady { continue } // 尾帧串联
        ensureJob(段视频: 首帧 = prev.TailFrame || 分镜图, 角色参考 = looks)
      }
    }
  case "compose":    ensureJob(ffmpeg concat + 字幕(SRT) + TTS 配音 + BGM 混音)
  }
  // ensureJob 幂等：(drama_id, kind, target_id, attempt) 唯一
}
func extractTailFrame(video) { ffmpeg -sseof -0.1 -i v.mp4 -frames:v 1 tail.png }
```
**分段切片**：累加分镜时长，超过 `maxSegSec` 则开新段；单镜超长时按 `timeBeats` 拆分；每段 prompt = 段内分镜 `videoMotionPrompt` 按时间码拼接 + `continuityOut`。

### 5.8 Agent 桥接循环与审批暂停
```ts
// yingce-agent（Node，pi-coding-agent）
for await (const ev of agent.run(messages, tools)) {
  if (ev.type === "tool_call") {
    const policy = await bridge.call("tool.policy", ev);    // 回调 Go 后端
    if (policy.requiresApproval) {
      await bridge.emit("approval.required", { approvalId, tool, args });
      const d = await bridge.waitDecision(approvalId);       // 长轮询；run 状态=waiting_approval
      if (d.decision === "reject") { agent.pushToolResult(ev.id, { error: "user_rejected" }); continue; }
      ev.args = d.editedArgs ?? ev.args;
    }
    const r = await bridge.call("tool.exec", ev);           // Go 端执行：建节点/提交任务/查资产…
    agent.pushToolResult(ev.id, r);
  }
  bridge.emit(ev);                                          // Go 写 events(seq++) 并经 SSE 推前端
}
```
- 需审批工具：消耗积分的生成、删除节点、批量操作；`remember: session` 时本会话同类工具自动放行。
- `undo`：每个 run 记录产生的 canvas ops 反向操作栈，`POST /agent/runs/:id/undo` 逆序回放。
- 并发/资源：`YINGCE_AGENT_MAX_SESSIONS=8`、`MAX_QUEUE=64`、`QUEUE_TIMEOUT=60s`、孤儿会话 TTL 24h。

### 5.9 画布虚拟化与 LOD
```ts
function visibleNodes(nodes, vp: ViewportTransform, screen) {
  const world = { x: -vp.x/vp.k, y: -vp.y/vp.k, w: screen.w/vp.k, h: screen.h/vp.k };
  const pad = 0.5 * Math.max(world.w, world.h);            // 预渲染缓冲
  return spatialIndex.query(expand(world, pad));           // 网格哈希(cell=512) 或 R-tree(rbush)
}
// LOD：k < 0.35 → 只渲染色块+标题；k < 0.6 → 缩略图（thumbnailUrl）；否则完整节点 + 视频 poster
// 平移/缩放：只改容器 transform: translate3d(x,y,0) scale(k)，rAF 合帧；拖拽中节点用 will-change
// 连线：leafer-ui 矢量层单独绘制，只算两端可见的边
```

### 5.10 撤销/重做与离线草稿
```
每次用户操作 → 生成 ops + inverseOps 入 history 栈（上限 100，合并 300ms 内连续拖动）
本地：localforage 保存未确认 ops 队列；断网恢复后按 baseRevision 重放，冲突则拉取 missing 后 rebase
```

---

## 6. 环境变量参考 & 本地开发

### 6.1 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `CANVAS_BACKEND_ADDR` | `:8080` | 后端监听 |
| `CANVAS_BACKEND_DATA_DIR` / `CANVAS_DATA_PATH` | `./data` | 本地数据/媒体目录 |
| `CANVAS_DATABASE_DRIVER` | `sqlite` | `sqlite` \| `postgres` |
| `DATABASE_URL` | – | Postgres DSN |
| `REDIS_URL` | – | 可选；多实例通知/限流/缓存 |
| `CANVAS_AUTO_MIGRATE` | `true` | 启动时迁移 |
| `CANVAS_WORKER_CONCURRENCY` / `CANVAS_CHANNEL_CONCURRENCY` | `3` / `3` | worker 槽 / 单渠道并发 |
| `CANVAS_REGISTRATION_ENABLED` | `false` | 开放注册 |
| `CANVAS_OIDC_ISSUER / CLIENT_ID / CLIENT_SECRET / REDIRECT_URL / ALLOWED_GROUP` | – | OIDC 单点登录 |
| `CANVAS_ALLOW_PRIVATE_UPSTREAMS` / `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS` | `false` | SSRF 白名单 |
| `CANVAS_MCP_ALLOWED_PRIVATE_HOSTS` | – | MCP 私网白名单 |
| `CANVAS_CORS_ORIGINS` | `http://localhost:3000` | CORS |
| `CANVAS_PUBLIC_BASE_URL` | – | 回调/公网媒体 URL |
| `ENABLE_PROVIDER_PLUGINS` | `false` | 启用第三方插件 |
| `CANVAS_FFMPEG_PATH` / `CANVAS_WHISPER_BASE_URL` | – | 合成 / 语音识别 |
| `CANVAS_SHUTDOWN_TIMEOUT` | `10m` | 优雅退出等待任务 |
| `CANVAS_UPDATER_SOCKET_DIR` / `CANVAS_UPDATER_TOKEN` | `/run/open-ai-canvas-updater` | 宿主更新器 |
| `YINGCE_AGENT_URL / TOKEN / BRIDGE_HOST` | – | Agent 运行时连接 |
| `YINGCE_AGENT_MAX_SESSIONS / SESSION_CAP / MAX_QUEUE / QUEUE_TIMEOUT_SECONDS / BODY_TIMEOUT_SECONDS / KILL_GRACE_SECONDS / ORPHAN_TTL_SECONDS / NODE_MEMORY_MB` | 8/8/64/60/30/5/86400/512 | Agent 资源 |
| `*_MEMORY_LIMIT / *_CPUS / *_PIDS_LIMIT` | 4g/2/256 | 容器资源 |
| `VITE_API_PROXY_TARGET` / `VITE_TLDRAW_LICENSE_KEY` | – | 前端开发代理 / tldraw 许可 |

### 6.2 本地开发启动
```bash
# 依赖：Go 1.25、Bun 1.3.x、Node 22、ffmpeg；可选 Docker
cp .env.example .env

# 后端（SQLite 零依赖）
cd backend && go run ./cmd/server          # 监听 :8080，自动迁移

# Agent 运行时（可选）
cd yingce-agent && npm i && npm run dev

# 前端
cd web && bun install && VITE_API_PROXY_TARGET=http://localhost:8080 bun run dev   # :3000

# 插件打包
cd plugin-packages && ./build-packages.sh && node generate-catalog.mjs

# 一键 Compose：dev / local / server / deploy 四套文件
docker compose -f docker-compose.dev.yml up -d   # Postgres + Redis + backend + agent + web + nginx
```

### 6.3 质量门禁（CI）
```
web:     bun run lint && bun run typecheck && bun run test (vitest) && bun run build
backend: go vet ./... && go test ./... (modernc sqlite，无需 CGO) && golangci-lint
plugins: node generate-catalog.test.mjs && verify-payment-packages.sh
e2e:     Playwright：登录→建画布→生成图片(mock 插件)→连线→分享
```
> 建议准备一个 **mock 插件**（本地 HTTP 假上游，可配置延迟/失败率），用于 CI 与本地联调，避免真实扣费。

---

## 7. MVP 裁剪路径 & 人员配置

### 7.1 三档交付

| 档位 | 范围 | 预计 |
|---|---|---|
| **MVP（8 周）** | 账号（邮箱密码）+ 无限画布（图/文/视频/连线/分组）+ 插件引擎（3 个插件：OpenAI Images、Seedance、OpenAI Chat）+ 任务队列/轮询/落盘 + 积分预扣结算 + 兑换码充值 + 基础后台（用户/渠道/定价） | 3 人 |
| **V1（+8 周）** | 分镜表 + 批量生成 + 技能库 + 云端 Agent（含审批）+ 在线支付（支付宝/微信）+ 实时协作/分享 + 对象存储 | 4–5 人 |
| **V2（+7 周）** | 短剧全流水线 + 电商 Agent + 3D Previs + MCP + 推广分佣 + 数据分析 + Host-Updater + OIDC/微信登录 | 5–6 人 |

**可推迟模块**：3D Previs、Live2D 看板娘、剪映草稿导出、LibTV/TapNow 导入、Excalidraw/tldraw 画板节点、全景节点、调色节点、飞书集成、Turnstile。

### 7.2 人员建议
| 角色 | 人数 | 职责 |
|---|---|---|
| 前端（画布方向） | 2 | 画布引擎、节点体系、分镜表、性能 |
| 前端（业务方向） | 1 | 首页/工作台/任务中心/后台/支付页 |
| Go 后端 | 2 | 任务调度、插件引擎、计费支付、短剧编排 |
| Agent/AI 工程 | 1 | Agent 运行时、工具定义、提示词与技能 |
| 测试/运维（兼） | 0.5–1 | CI、Compose 部署、监控 |

### 7.3 关键风险
1. **上游协议碎片化** → 坚持“声明式 Manifest + DSL”，禁止在宿主写厂商分支。
2. **重复扣费/重复提交** → `creationSubmissionId` 唯一索引 + “已提交禁止切路由” + 账本 `idem_key`。
3. **画布大图性能** → 第 1 周就做 500+ 节点压测基线，并固化 LOD/虚拟化。
4. **Agent 失控消费** → 审批策略 + run 级积分预算上限 + 工具调用次数上限。
