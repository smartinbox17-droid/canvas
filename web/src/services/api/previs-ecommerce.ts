import { http } from "@/services/api/request";

export interface EcommerceKnowledgeItem {
  id: string;
  platform: string;
  category: string;
  ruleTitle: string;
  ruleBody: string;
  safeZone: string;
}

export interface EcommerceProjectItem {
  id: string;
  productName: string;
  categoryCode: string;
  targetPlatform: string;
  regionCasting: string;
  safeAreaMargin: string;
  complianceScore: number;
  directionsJson?: string;
  imagesJson?: string;
  status: string;
}

export interface PrevisSceneItem {
  id: string;
  title: string;
  cityPreset: string;
  weather: string;
  cameraRecipe: string;
  characterPose: string;
  propsJson?: string;
  depthVideoUrl?: string;
  whiteModelVideo?: string;
  canvasNodeId?: string;
}

export interface TeamItem {
  id: string;
  ownerId: string;
  parentTeamId?: string;
  name: string;
  poolMicrocredits: number;
}

export const previsEcommerceApi = {
  listKnowledge: (platform?: string) =>
    http.get<{ items: EcommerceKnowledgeItem[] }>("/v1/ecommerce/knowledge/entries", {
      params: platform ? { platform } : undefined,
    }),
  listEcommerceProjects: () => http.get<{ items: EcommerceProjectItem[] }>("/v1/ecommerce/projects"),
  createEcommerceProject: (payload: {
    productName: string;
    categoryCode: string;
    targetPlatform: string;
    regionCasting: string;
  }) => http.post<{ project: EcommerceProjectItem }>("/v1/ecommerce/projects", payload),

  listPrevisScenes: () => http.get<{ items: PrevisSceneItem[] }>("/v1/previs/scenes"),
  createPrevisScene: (payload: {
    title: string;
    cityPreset: string;
    weather: string;
    cameraRecipe: string;
    characterPose: string;
  }) => http.post<{ scene: PrevisSceneItem }>("/v1/previs/scenes", payload),

  listTeams: () => http.get<{ items: TeamItem[] }>("/v1/teams"),
  createTeam: (payload: { name: string; poolMicrocredits: number; parentTeamId?: string }) =>
    http.post<{ team: TeamItem }>("/v1/teams", payload),
};
