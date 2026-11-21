import i18n from "i18next";
import { initReactI18next } from "react-i18next";

const resources = {
  "zh-CN": {
    translation: {
      brand: "AI 无限画布工作台",
      nav: {
        home: "创作首页",
        canvas: "无限画布",
        drama: "短剧流水线",
        admin: "管理后台",
      },
      theme: {
        toggle: "切换明暗主题",
      },
      status: {
        ready: "后端连接就绪",
        checking: "检测连接中...",
      },
    },
  },
  "en-US": {
    translation: {
      brand: "AI Canvas Workbench",
      nav: {
        home: "Creation Home",
        canvas: "Infinite Canvas",
        drama: "Drama Pipeline",
        admin: "Admin Console",
      },
      theme: {
        toggle: "Toggle Theme",
      },
      status: {
        ready: "Backend Ready",
        checking: "Checking health...",
      },
    },
  },
};

void i18n.use(initReactI18next).init({
  resources,
  lng: "zh-CN",
  fallbackLng: "zh-CN",
  interpolation: { escapeValue: false },
});

export default i18n;
