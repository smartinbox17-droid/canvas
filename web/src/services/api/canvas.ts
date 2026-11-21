import { http } from "@/services/api/request";
import type { CanvasConnection, CanvasNodeData, ViewportTransform } from "@/types/canvas";

export interface CanvasSnapshot {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
  viewport: ViewportTransform;
}

export interface CanvasProjectItem {
  id: string;
  ownerId: string;
  title: string;
  revision: number;
  snapshotJson?: string;
  shareToken?: string;
  editLeaseOwner?: string;
  updatedAt: string;
}

export type CanvasOperation =
  | { op: "node.add"; node: CanvasNodeData }
  | { op: "node.update"; id: string; patch: Record<string, unknown> }
  | { op: "node.remove"; id: string }
  | { op: "node.move"; ids: string[]; dx: number; dy: number }
  | { op: "connection.add"; connection: CanvasConnection }
  | { op: "connection.remove"; id: string }
  | { op: "viewport.set"; viewport: ViewportTransform };

export interface ApplyOperationsResponse {
  revision: number;
  missing?: Array<{
    id: string;
    revision: number;
    clientId: string;
    opsJson: string;
  }>;
}

export const canvasApi = {
  listProjects: () => http.get<{ items: CanvasProjectItem[] }>("/v1/canvas-projects"),
  createProject: (title: string) =>
    http.post<{ project: CanvasProjectItem }>("/v1/canvas-projects", { title }),
  getProject: (id: string) => http.get<{ project: CanvasProjectItem }>(`/v1/canvas-projects/${id}`),
  patchOperations: (id: string, payload: {
    baseRevision: number;
    clientId: string;
    operations: CanvasOperation[];
  }) => http.patch<ApplyOperationsResponse>(`/v1/canvas-projects/${id}/operations`, payload),
  enableShare: (id: string) =>
    http.post<{ shareToken: string }>(`/v1/canvas-projects/${id}/share`),
};
