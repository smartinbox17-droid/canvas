import { http } from "@/services/api/request";

export interface AuthUser {
  id: string;
  email: string;
  role: string;
  status: string;
  referralCode?: string;
}

export interface CreditSummary {
  userId: string;
  balanceMicrocredits: number;
  frozenMicrocredits: number;
}

export interface AuthSessionResponse {
  authenticated: boolean;
  user: AuthUser | null;
  credits?: CreditSummary;
}

export interface AssetItem {
  id: string;
  ownerId: string;
  folderId?: string;
  name: string;
  kind: "image" | "video" | "audio" | "file";
  mimeType: string;
  sizeBytes: number;
  url: string;
  sha256: string;
  createdAt: string;
}

export interface AssetFolder {
  id: string;
  ownerId: string;
  parentId?: string;
  name: string;
  createdAt: string;
}

export const authApi = {
  getSession: () => http.get<AuthSessionResponse>("/v1/auth/session"),
  login: (email: string, password: string) =>
    http.post<{ user: AuthUser; credits?: CreditSummary }>("/v1/auth/login", { email, password }),
  register: (email: string, password: string) =>
    http.post<{ user: AuthUser; credits?: CreditSummary }>("/v1/auth/register", { email, password }),
  logout: () => http.post<{ loggedOut: boolean }>("/v1/auth/logout"),
};

export const assetsApi = {
  listAssets: (params?: { folderId?: string; kind?: string }) =>
    http.get<{ items: AssetItem[] }>("/v1/assets", { params }),
  listFolders: () => http.get<{ items: AssetFolder[] }>("/v1/asset-folders"),
  createFolder: (name: string, parentId?: string) =>
    http.post<AssetFolder>("/v1/asset-folders", { name, parentId }),
  uploadAsset: (file: File, folderId?: string) => {
    const form = new FormData();
    form.append("file", file);
    if (folderId) {
      form.append("folderId", folderId);
    }
    return http.post<AssetItem>("/v1/assets", form);
  },
  deleteAsset: (id: string) => http.delete<{ deleted: boolean }>(`/v1/assets/${id}`),
};
