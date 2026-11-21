import { useEffect, useState } from "react";
import { BrowserRouter, Link, Outlet, Route, Routes, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Card,
  ConfigProvider,
  Form,
  Input,
  List,
  Select,
  Space,
  Tag,
  Upload,
  message,
  theme as antdTheme,
} from "antd";
import { useTranslation } from "react-i18next";
import { http, ApiError } from "@/services/api/request";
import { authApi, assetsApi } from "@/services/api/auth-assets";
import { tasksApi } from "@/services/api/tasks";
import { clearAccountStorage } from "@/lib/storage/account-storage";
import { useAppStore } from "@/stores/app-store";

interface ReadyPayload {
  status: string;
  database: string;
  redis: string;
}

function UserLayout() {
  const { t, i18n } = useTranslation();
  const { theme, toggleTheme, currentUserId, setCurrentUserId } = useAppStore();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const healthQuery = useQuery({
    queryKey: ["health", "ready"],
    queryFn: () => http.get<ReadyPayload>("/health/ready"),
    retry: false,
  });

  const sessionQuery = useQuery({
    queryKey: ["auth", "session"],
    queryFn: () => authApi.getSession(),
  });

  useEffect(() => {
    const nextUserId = sessionQuery.data?.user?.id ?? null;
    if (currentUserId && currentUserId !== nextUserId) {
      void clearAccountStorage(currentUserId);
      queryClient.clear();
    }
    if (currentUserId !== nextUserId) {
      setCurrentUserId(nextUserId);
    }
  }, [sessionQuery.data, currentUserId, queryClient, setCurrentUserId]);

  const logoutMutation = useMutation({
    mutationFn: () => authApi.logout(),
    onSuccess: async () => {
      if (currentUserId) {
        await clearAccountStorage(currentUserId);
      }
      setCurrentUserId(null);
      queryClient.clear();
      void navigate("/login");
    },
  });

  const user = sessionQuery.data?.user;
  const credits = sessionQuery.data?.credits;

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <strong style={{ fontSize: 16, marginBottom: 12 }}>{t("brand")}</strong>
        <Link to="/">{t("nav.home")}</Link>
        <Link to="/tasks">任务中心 (Task Center)</Link>
        <Link to="/canvas">{t("nav.canvas")}</Link>
        <Link to="/assets">素材库 (Assets)</Link>
        <Link to="/drama">{t("nav.drama")}</Link>
        <Link to="/admin">{t("nav.admin")}</Link>
      </aside>
      <div>
        <header className="app-header">
          <Space>
            <Tag color={healthQuery.isSuccess ? "green" : "gold"}>
              {healthQuery.isSuccess ? `${t("status.ready")} (${healthQuery.data.database})` : t("status.checking")}
            </Tag>
            {user && credits && (
              <Tag color="blue">
                可用积分: {((credits.balanceMicrocredits - credits.frozenMicrocredits) / 1_000_000).toFixed(2)}
              </Tag>
            )}
          </Space>
          <Space>
            {user ? (
              <>
                <span>{user.email} ({user.role})</span>
                <Button size="small" onClick={() => logoutMutation.mutate()}>
                  退出登录
                </Button>
              </>
            ) : (
              <Button size="small" type="primary" onClick={() => void navigate("/login")}>
                登录 / 注册
              </Button>
            )}
            <Button
              size="small"
              onClick={() => void i18n.changeLanguage(i18n.language === "zh-CN" ? "en-US" : "zh-CN")}
            >
              {i18n.language === "zh-CN" ? "EN" : "中文"}
            </Button>
            <Button size="small" onClick={toggleTheme}>
              {t("theme.toggle")} ({theme})
            </Button>
          </Space>
        </header>
        <main className="app-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function CreationWorkbenchPage() {
  const queryClient = useQueryClient();
  const [selectedModel, setSelectedModel] = useState<string>("mock-gpt-4o");
  const [prompt, setPrompt] = useState<string>("");

  const modelsQuery = useQuery({
    queryKey: ["models", "list"],
    queryFn: () => tasksApi.listModels(),
  });

  const activeModel = modelsQuery.data?.items.find((m) => m.code === selectedModel);

  const submitMutation = useMutation({
    mutationFn: () =>
      tasksApi.createTask({
        creationSubmissionId: `sub_${Date.now()}`,
        logicalModelId: selectedModel,
        capability: activeModel?.capability ?? "text",
        prompt,
      }),
    onSuccess: () => {
      void message.success("任务已提交至异步队列");
      setPrompt("");
      void queryClient.invalidateQueries({ queryKey: ["tasks", "list"] });
      void queryClient.invalidateQueries({ queryKey: ["auth", "session"] });
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        void message.error(`[${err.reason}] ${err.message}`);
      } else {
        void message.error("提交任务失败");
      }
    },
  });

  return (
    <Space direction="vertical" style={{ width: "100%" }} size="large">
      <Card title="多模态创作工作台 (Declarative Plugin Router)">
        <Space direction="vertical" style={{ width: "100%" }}>
          <Space>
            <span>选择逻辑模型：</span>
            <Select
              style={{ width: 280 }}
              value={selectedModel}
              onChange={(v) => setSelectedModel(v)}
              options={(modelsQuery.data?.items ?? []).map((m) => ({
                value: m.code,
                label: `${m.label} [${m.capability}] (${(m.defaultPriceMicro / 1_000_000).toFixed(2)} 积分)`,
              }))}
            />
          </Space>
          <Input.TextArea
            rows={3}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="输入画面描述、视频镜头提示词或剧本需求..."
          />
          <Button
            type="primary"
            disabled={!prompt.trim()}
            loading={submitMutation.isPending}
            onClick={() => submitMutation.mutate()}
          >
            提交生成任务
          </Button>
        </Space>
      </Card>
      <TaskCenterPage />
    </Space>
  );
}

function TaskCenterPage() {
  const queryClient = useQueryClient();
  const tasksQuery = useQuery({
    queryKey: ["tasks", "list"],
    queryFn: () => tasksApi.listTasks(),
    refetchInterval: 2000,
  });

  const cancelMutation = useMutation({
    mutationFn: (id: string) => tasksApi.cancelTask(id),
    onSuccess: () => {
      void message.info("任务已取消并退回预扣积分");
      void queryClient.invalidateQueries({ queryKey: ["tasks", "list"] });
      void queryClient.invalidateQueries({ queryKey: ["auth", "session"] });
    },
  });

  return (
    <Card title="异步任务中心 (Task Center)">
      <List
        loading={tasksQuery.isLoading}
        dataSource={tasksQuery.data?.items ?? []}
        locale={{ emptyText: "暂无生成任务（登录后在上方工作台提交）" }}
        renderItem={(task) => (
          <List.Item
            actions={
              task.status === "queued" || task.status === "running" || task.status === "polling"
                ? [
                    <Button key="cancel" size="small" danger onClick={() => cancelMutation.mutate(task.id)}>
                      取消任务
                    </Button>,
                  ]
                : []
            }
          >
            <List.Item.Meta
              title={
                <Space>
                  <Tag color={task.status === "succeeded" ? "green" : task.status === "failed" ? "red" : "blue"}>
                    {task.status}
                  </Tag>
                  <strong>{task.model}</strong>
                  <span>{task.prompt}</span>
                </Space>
              }
              description={
                task.textDraft
                  ? `输出: ${task.textDraft}`
                  : task.error
                    ? `错误: ${task.error}`
                    : `预扣积分: ${((task.authorizedChargeMicrocredits ?? 0) / 1_000_000).toFixed(2)}`
              }
            />
          </List.Item>
        )}
      />
    </Card>
  );
}

function LoginPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [errMsg, setErrMsg] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const submitMutation = useMutation({
    mutationFn: (values: { email: string; password: string }) =>
      mode === "login"
        ? authApi.login(values.email, values.password)
        : authApi.register(values.email, values.password),
    onSuccess: async () => {
      setErrMsg(null);
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] });
      void navigate("/");
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setErrMsg(`[${err.reason}] ${err.message}`);
      } else {
        setErrMsg("Authentication failed");
      }
    },
  });

  return (
    <div style={{ maxWidth: 420, margin: "40px auto" }}>
      <Card
        title={mode === "login" ? "账号登录" : "注册新账号（首账号自动为管理员）"}
        extra={
          <Button type="link" onClick={() => setMode(mode === "login" ? "register" : "login")}>
            {mode === "login" ? "去注册" : "去登录"}
          </Button>
        }
      >
        {errMsg && <Alert type="error" message={errMsg} style={{ marginBottom: 16 }} />}
        <Form layout="vertical" onFinish={(v) => submitMutation.mutate(v)}>
          <Form.Item name="email" label="邮箱" rules={[{ required: true, type: "email" }]}>
            <Input placeholder="you@example.com" />
          </Form.Item>
          <Form.Item name="password" label="密码 (至少 8 位)" rules={[{ required: true, min: 8 }]}>
            <Input.Password placeholder="••••••••" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitMutation.isPending}>
            {mode === "login" ? "登录" : "注册并领取 100 初始积分"}
          </Button>
        </Form>
      </Card>
    </div>
  );
}

function AssetsPage() {
  const queryClient = useQueryClient();
  const assetsQuery = useQuery({
    queryKey: ["assets", "list"],
    queryFn: () => assetsApi.listAssets(),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => assetsApi.deleteAsset(id),
    onSuccess: () => {
      void message.success("素材已删除");
      void queryClient.invalidateQueries({ queryKey: ["assets", "list"] });
    },
    onError: (err) => {
      if (err instanceof ApiError && err.reason === "ASSET_IN_USE") {
        void message.error("该素材正被画布或任务引用，禁止删除");
      } else {
        void message.error("删除失败");
      }
    },
  });

  return (
    <Card
      title="个人素材库 (Assets Library)"
      extra={
        <Upload
          showUploadList={false}
          customRequest={async ({ file, onSuccess, onError }) => {
            try {
              await assetsApi.uploadAsset(file as File);
              await queryClient.invalidateQueries({ queryKey: ["assets", "list"] });
              onSuccess?.({});
            } catch (e) {
              onError?.(e as Error);
            }
          }}
        >
          <Button type="primary">上传素材</Button>
        </Upload>
      }
    >
      <List
        loading={assetsQuery.isLoading}
        dataSource={assetsQuery.data?.items ?? []}
        locale={{ emptyText: "暂无素材，点击右上角上传（需先登录）" }}
        renderItem={(item) => (
          <List.Item
            actions={[
              <Button
                key="del"
                danger
                size="small"
                onClick={() => deleteMutation.mutate(item.id)}
              >
                删除
              </Button>,
            ]}
          >
            <List.Item.Meta
              title={
                <Space>
                  <span>{item.name}</span>
                  <Tag>{item.kind}</Tag>
                  <Tag>{item.mimeType}</Tag>
                </Space>
              }
              description={`SHA256: ${item.sha256.slice(0, 16)}... · ${(item.sizeBytes / 1024).toFixed(1)} KB`}
            />
          </List.Item>
        )}
      />
    </Card>
  );
}

function AdminShell() {
  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <strong>Admin Console</strong>
        <Link to="/admin">Overview</Link>
        <Link to="/">Back to Workspace</Link>
      </aside>
      <main className="app-content">
        <h2>Admin Console Shell</h2>
        <p>Channels, Logical Models, Users, Billing Orders, and System Settings mount here.</p>
      </main>
    </div>
  );
}

function CanvasStagePlaceholder({ title }: { title: string }) {
  return (
    <div className="canvas-stage">
      <div>
        <h2>{title}</h2>
        <p>Phase 2 Protocol Router & Task Center active. Infinite Canvas DOM + Leafer-UI layer mounts in Phase 4.</p>
      </div>
    </div>
  );
}

export function App() {
  const theme = useAppStore((s) => s.theme);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  return (
    <ConfigProvider
      theme={{
        algorithm: theme === "dark" ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
      }}
    >
      <BrowserRouter>
        <Routes>
          <Route element={<UserLayout />}>
            <Route index element={<CreationWorkbenchPage />} />
            <Route path="tasks" element={<TaskCenterPage />} />
            <Route path="login" element={<LoginPage />} />
            <Route path="assets" element={<AssetsPage />} />
            <Route path="canvas" element={<CanvasStagePlaceholder title="Infinite Canvas Engine" />} />
            <Route path="drama" element={<CanvasStagePlaceholder title="Short Drama Production Pipeline" />} />
          </Route>
          <Route path="admin/*" element={<AdminShell />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
}
