import { http } from "@/services/api/request";

export interface DramaProjectItem {
  id: string;
  ownerId: string;
  title: string;
  rawScript: string;
  storyBibleJson?: string;
  pacingPreset: string;
  stage: "script" | "cast" | "segments" | "compose" | "completed";
  canvasProjectId?: string;
  createdAt: string;
}

export interface DramaCastItem {
  id: string;
  dramaId: string;
  name: string;
  roleType: string;
  description: string;
  imagePrompt: string;
  imageUrl?: string;
  voiceId?: string;
  imageReady: boolean;
}

export interface DramaEpisodeItem {
  id: string;
  dramaId: string;
  episodeNumber: number;
  title: string;
  summary: string;
  approved: boolean;
  composedUrl?: string;
  srtContent?: string;
}

export interface DramaShotItem {
  id: string;
  dramaId: string;
  episodeId: string;
  shotIndex: number;
  durationSec: number;
  shotSize: string;
  cameraMotion: string;
  sceneDescription: string;
  dialogue: string;
  imagePrompt: string;
  videoMotionPrompt: string;
  continuityOut: string;
  keyframeUrl?: string;
}

export interface DramaSegmentItem {
  id: string;
  dramaId: string;
  episodeId: string;
  segmentIndex: number;
  durationSec: number;
  combinedPrompt: string;
  firstFrameUrl?: string;
  tailFrameUrl?: string;
  tailFrameReady: boolean;
  videoUrl?: string;
  status: string;
}

export interface DramaDetailResponse {
  drama: DramaProjectItem;
  cast: DramaCastItem[];
  episodes: DramaEpisodeItem[];
  shots: DramaShotItem[];
  segments: DramaSegmentItem[];
}

export const dramaApi = {
  listDramas: () => http.get<{ items: DramaProjectItem[] }>("/v1/dramas"),
  createDrama: (payload: { title: string; rawScript: string; pacingPreset: string }) =>
    http.post<DramaDetailResponse>("/v1/dramas", payload),
  getDrama: (id: string) => http.get<DramaDetailResponse>(`/v1/dramas/${id}`),
  startOrchestrator: (id: string) => http.post<DramaDetailResponse>(`/v1/dramas/${id}/start`),
  exportJianyingDraft: (dramaId: string, episodeId: string) =>
    http.post<{ draft: Record<string, unknown> }>(
      `/v1/dramas/${dramaId}/episodes/${episodeId}/jianying-draft`,
    ),
};
