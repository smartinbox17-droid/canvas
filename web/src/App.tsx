import { useEffect } from "react";
import { BrowserRouter, Link, Outlet, Route, Routes } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Button, ConfigProvider, Space, Tag, theme as antdTheme } from "antd";
import { useTranslation } from "react-i18next";
import { http } from "@/services/api/request";
import { useAppStore } from "@/stores/app-store";

interface ReadyPayload {
  status: string;
  database: string;
  redis: string;
}

function UserLayout() {
  const { t, i18n } = useTranslation();
  const { theme, toggleTheme } = useAppStore();
  const healthQuery = useQuery({
    queryKey: ["health", "ready"],
    queryFn: () => http.get<ReadyPayload>("/health/ready"),
    retry: false,
  });

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <strong style={{ fontSize: 16, marginBottom: 12 }}>{t("brand")}</strong>
        <Link to="/">{t("nav.home")}</Link>
        <Link to="/canvas">{t("nav.canvas")}</Link>
        <Link to="/drama">{t("nav.drama")}</Link>
        <Link to="/admin">{t("nav.admin")}</Link>
      </aside>
      <div>
        <header className="app-header">
          <Space>
            <Tag color={healthQuery.isSuccess ? "green" : "gold"}>
              {healthQuery.isSuccess ? `${t("status.ready")} (${healthQuery.data.database})` : t("status.checking")}
            </Tag>
          </Space>
          <Space>
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

function AdminShell() {
  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <strong>Admin Console</strong>
        <Link to="/admin">Overview</Link>
        <Link to="/">Back to Workspace</Link>
      </aside>
      <main className="app-content">
        <h2>Admin Console Shell (Phase 0)</h2>
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
        <p>Phase 0 foundation ready. Infinite Canvas DOM + Leafer-UI layer mounts in Phase 4.</p>
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
            <Route index element={<CanvasStagePlaceholder title="Creation Home & Workbench" />} />
            <Route path="canvas" element={<CanvasStagePlaceholder title="Infinite Canvas Engine" />} />
            <Route path="drama" element={<CanvasStagePlaceholder title="Short Drama Production Pipeline" />} />
          </Route>
          <Route path="admin/*" element={<AdminShell />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
}
