import { http } from "@/services/api/request";

export interface SkillItem {
  id: string;
  ownerId?: string;
  name: string;
  category: string;
  outputMode: string;
  description: string;
  systemPrompt: string;
  filesJson?: string;
  isBuiltin: boolean;
}

export interface AgentToolCapability {
  name: string;
  category: string;
  requiresApproval: boolean;
  description: string;
}

export interface CloudAgentRunItem {
  id: string;
  conversationId: string;
  projectId?: string;
  prompt: string;
  model: string;
  status: "running" | "waiting_approval" | "completed" | "failed" | "cancelled" | "undone";
  createdAt: string;
}

export interface CloudAgentApprovalItem {
  id: string;
  runId: string;
  toolName: string;
  argsJson: string;
  status: "pending" | "approved" | "rejected";
}

export interface AgentMemoryItem {
  id: string;
  scope: string;
  title: string;
  content: string;
  updatedAt: string;
}

export interface MCPServerItem {
  id: string;
  name: string;
  transport: string;
  endpoint: string;
  toolsJson?: string;
  enabled: boolean;
}

export const agentSkillsApi = {
  listSkills: (category?: string) =>
    http.get<{ items: SkillItem[] }>("/v1/skills", { params: category ? { category } : undefined }),
  createSkill: (payload: {
    name: string;
    category: string;
    outputMode: string;
    description: string;
    systemPrompt: string;
    files: Record<string, string>;
  }) => http.post<{ skill: SkillItem }>("/v1/skills", payload),

  getCapabilities: () => http.get<{ tools: AgentToolCapability[] }>("/v1/agent/capabilities"),
  listRuns: () =>
    http.get<{ items: CloudAgentRunItem[]; pendingApprovals: CloudAgentApprovalItem[] }>("/v1/agent/runs"),
  startRun: (payload: { projectId?: string; prompt: string; requireApproval?: boolean }) =>
    http.post<{ run: CloudAgentRunItem; approval?: CloudAgentApprovalItem }>("/v1/agent/runs", payload),
  resolveApproval: (runId: string, approvalId: string, decision: "approve" | "reject") =>
    http.post<{ run: CloudAgentRunItem }>(
      `/v1/agent/runs/${runId}/approvals/${approvalId}/decision`,
      { decision },
    ),
  undoRun: (runId: string) => http.post<{ run: CloudAgentRunItem }>(`/v1/agent/runs/${runId}/undo`),

  listMemories: () => http.get<{ items: AgentMemoryItem[] }>("/v1/agent/memories"),
  createMemory: (title: string, content: string) =>
    http.post<{ memory: AgentMemoryItem }>("/v1/agent/memories", { scope: "global", title, content }),

  listMCPServers: () => http.get<{ items: MCPServerItem[] }>("/v1/mcp/servers"),
  createMCPServer: (name: string, endpoint: string, transport = "streamable_http") =>
    http.post<{ server: MCPServerItem }>("/v1/mcp/servers", { name, endpoint, transport }),
};
