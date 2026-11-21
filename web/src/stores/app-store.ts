import { create } from "zustand";

export type ThemeMode = "light" | "dark";

interface AppStoreState {
  theme: ThemeMode;
  currentUserId: string | null;
  toggleTheme: () => void;
  setCurrentUserId: (id: string | null) => void;
}

export const useAppStore = create<AppStoreState>((set) => ({
  theme: "dark",
  currentUserId: null,
  toggleTheme: () =>
    set((state) => {
      const next: ThemeMode = state.theme === "dark" ? "light" : "dark";
      document.documentElement.dataset.theme = next;
      return { theme: next };
    }),
  setCurrentUserId: (id) => set({ currentUserId: id }),
}));
