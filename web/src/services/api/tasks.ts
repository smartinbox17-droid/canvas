import { http } from "@/services/api/request";

export interface LogicalModelItem {
  id: string;
  code: string;
  label: string;
  capability: "text" | "image" | "video" | "audio";
  defaultPriceMicro: number;
}

export interface GenerationTaskItem {
  id: string;
  userId: string;
  projectId: string;
  type: string;
  status: "queued" | "running" | "polling" | "succeeded" | "failed" | "cancelled";
  stage: string;
  progress: number;
  prompt: string;
  model: string;
  authorizedChargeMicrocredits?: number;
  resultJson?: string;
  textDraft?: string;
  error?: string;
  createdAt: string;
}

export interface QuoteResponse {
  estimateMicrocredits: number;
  logicalModelId: string;
  modelCode: string;
  capability: string;
}

export const tasksApi = {
  listModels: () => http.get<{ items: LogicalModelItem[] }>("/v1/models"),
  quoteTask: (logicalModelId: string, prompt: string, capability: string) =>
    http.post<QuoteResponse>("/v1/tasks/quote", {
      logicalModelId,
      request: { capability, model: logicalModelId, prompt },
    }),
  createTask: (payload: {
    creationSubmissionId: string;
    logicalModelId: string;
    capability: string;
    prompt: string;
  }) =>
    http.post<{ task: GenerationTaskItem }>("/v1/tasks", {
      creationSubmissionId: payload.creationSubmissionId,
      logicalModelId: payload.logicalModelId,
      operation: `${payload.capability}_generation`,
      request: {
        capability: payload.capability,
        model: payload.logicalModelId,
        prompt: payload.prompt,
      },
    }),
  listTasks: () => http.get<{ items: GenerationTaskItem[] }>("/v1/tasks"),
  cancelTask: (id: string) => http.post<{ task: GenerationTaskItem }>(`/v1/tasks/${id}/cancel`),
};
