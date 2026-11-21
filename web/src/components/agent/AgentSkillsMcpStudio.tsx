import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card, Checkbox, Col, Form, Input, List, Row, Select, Space, Tabs, Tag, message } from "antd";
import { agentSkillsApi } from "@/services/api/agent-skills";
import { canvasApi } from "@/services/api/canvas";

export function AgentSkillsMcpStudio() {
  const queryClient = useQueryClient();
  const [agentPrompt, setAgentPrompt] = useState("");
  const [requireApproval, setRequireApproval] = useState(true);

  const projectsQuery = useQuery({
    queryKey: ["canvas", "projects"],
    queryFn: () => canvasApi.listProjects(),
  });
  const activeProjectId = projectsQuery.data?.items[0]?.id;

  const capsQuery = useQuery({
    queryKey: ["agent", "capabilities"],
    queryFn: () => agentSkillsApi.getCapabilities(),
  });

  const runsQuery = useQuery({
    queryKey: ["agent", "runs"],
    queryFn: () => agentSkillsApi.listRuns(),
    refetchInterval: 3000,
  });

  const skillsQuery = useQuery({
    queryKey: ["skills", "list"],
    queryFn: () => agentSkillsApi.listSkills(),
  });

  const memoriesQuery = useQuery({
    queryKey: ["agent", "memories"],
    queryFn: () => agentSkillsApi.listMemories(),
  });

  const mcpQuery = useQuery({
    queryKey: ["mcp", "servers"],
    queryFn: () => agentSkillsApi.listMCPServers(),
  });

  const startRunMutation = useMutation({
    mutationFn: () =>
      agentSkillsApi.startRun({
        projectId: activeProjectId,
        prompt: agentPrompt,
        requireApproval,
      }),
    onSuccess: (res) => {
      setAgentPrompt("");
      if (res.run.status === "waiting_approval") {
        void message.warning("Agent 已暂停在审批 Checkpoint，请确认是否放行高消耗工具调用");
      } else {
        void message.success("Agent 已完成画布分镜节点编排");
      }
      void queryClient.invalidateQueries({ queryKey: ["agent", "runs"] });
    },
  });

  const decisionMutation = useMutation({
    mutationFn: (p: { runId: string; approvalId: string; decision: "approve" | "reject" }) =>
      agentSkillsApi.resolveApproval(p.runId, p.approvalId, p.decision),
    onSuccess: (_, vars) => {
      void message.success(vars.decision === "approve" ? "已批准执行并写入画布节点" : "已拒绝该工具调用");
      void queryClient.invalidateQueries({ queryKey: ["agent", "runs"] });
    },
  });

  const undoMutation = useMutation({
    mutationFn: (runId: string) => agentSkillsApi.undoRun(runId),
    onSuccess: () => {
      void message.info("已回滚该次 Agent 运行在画布上产生的全部节点与连线");
      void queryClient.invalidateQueries({ queryKey: ["agent", "runs"] });
    },
  });

  const createSkillMutation = useMutation({
    mutationFn: (v: { name: string; category: string; description: string; systemPrompt: string; skillMd: string }) =>
      agentSkillsApi.createSkill({
        name: v.name,
        category: v.category,
        outputMode: "workflow",
        description: v.description,
        systemPrompt: v.systemPrompt,
        files: { "SKILL.md": v.skillMd },
      }),
    onSuccess: () => {
      void message.success("自定义创作技能已保存到技能库");
      void queryClient.invalidateQueries({ queryKey: ["skills", "list"] });
    },
  });

  const createMcpMutation = useMutation({
    mutationFn: (v: { name: string; endpoint: string }) =>
      agentSkillsApi.createMCPServer(v.name, v.endpoint, "streamable_http"),
    onSuccess: () => {
      void message.success("远程 MCP 服务已挂载");
      void queryClient.invalidateQueries({ queryKey: ["mcp", "servers"] });
    },
  });

  const createMemoryMutation = useMutation({
    mutationFn: (v: { title: string; content: string }) =>
      agentSkillsApi.createMemory(v.title, v.content),
    onSuccess: () => {
      void message.success("长期创作记忆文档已保存");
      void queryClient.invalidateQueries({ queryKey: ["agent", "memories"] });
    },
  });

  const pendingApprovals = runsQuery.data?.pendingApprovals ?? [];

  return (
    <Tabs
      items={[
        {
          key: "agent",
          label: "云端 AI Agent & 审批 / 撤销",
          children: (
            <Space direction="vertical" style={{ width: "100%" }} size="large">
              <Card title="发起云端 Agent 多轮画布编排 (Pi Coding Agent Bridge)">
                <Space direction="vertical" style={{ width: "100%" }}>
                  <Input.TextArea
                    rows={3}
                    value={agentPrompt}
                    onChange={(e) => setAgentPrompt(e.target.value)}
                    placeholder="例如：帮我在当前画布上拆解 3 个赛博朋克雨夜追逐镜头节点并自动连线..."
                  />
                  <Space>
                    <Checkbox
                      checked={requireApproval}
                      onChange={(e) => setRequireApproval(e.target.checked)}
                    >
                      高消耗批量操作前启用人工审批 Checkpoint (waiting_approval)
                    </Checkbox>
                    <Button
                      type="primary"
                      disabled={!agentPrompt.trim()}
                      loading={startRunMutation.isPending}
                      onClick={() => startRunMutation.mutate()}
                    >
                      启动 Agent Run
                    </Button>
                  </Space>
                </Space>
              </Card>

              {pendingApprovals.length > 0 && (
                <Card title="待人工审批的工具调用 (Approval Checkpoints)">
                  <List
                    dataSource={pendingApprovals}
                    renderItem={(appr) => (
                      <List.Item
                        actions={[
                          <Button
                            key="approve"
                            type="primary"
                            size="small"
                            onClick={() =>
                              decisionMutation.mutate({
                                runId: appr.runId,
                                approvalId: appr.id,
                                decision: "approve",
                              })
                            }
                          >
                            批准并继续执行
                          </Button>,
                          <Button
                            key="reject"
                            danger
                            size="small"
                            onClick={() =>
                              decisionMutation.mutate({
                                runId: appr.runId,
                                approvalId: appr.id,
                                decision: "reject",
                              })
                            }
                          >
                            拒绝调用
                          </Button>,
                        ]}
                      >
                        <List.Item.Meta
                          title={
                            <Space>
                              <Tag color="gold">waiting_approval</Tag>
                              <strong>工具: {appr.toolName}</strong>
                            </Space>
                          }
                          description={`参数: ${appr.argsJson}`}
                        />
                      </List.Item>
                    )}
                  />
                </Card>
              )}

              <Card title="Agent 运行历史与画布一键回滚 (Run Undo Stack)">
                <List
                  dataSource={runsQuery.data?.items ?? []}
                  locale={{ emptyText: "暂无 Agent 运行记录" }}
                  renderItem={(run) => (
                    <List.Item
                      actions={
                        run.status === "completed"
                          ? [
                              <Button
                                key="undo"
                                size="small"
                                onClick={() => undoMutation.mutate(run.id)}
                              >
                                一键撤销画布变更 (Undo)
                              </Button>,
                            ]
                          : []
                      }
                    >
                      <List.Item.Meta
                        title={
                          <Space>
                            <Tag
                              color={
                                run.status === "completed"
                                  ? "green"
                                  : run.status === "waiting_approval"
                                    ? "gold"
                                    : run.status === "undone"
                                      ? "purple"
                                      : "default"
                              }
                            >
                              {run.status}
                            </Tag>
                            <span>{run.prompt}</span>
                          </Space>
                        }
                        description={`Run ID: ${run.id} · Model: ${run.model}`}
                      />
                    </List.Item>
                  )}
                />
              </Card>

              <Card title="已注册 Agent 工具能力表 (Capabilities)">
                <List
                  dataSource={capsQuery.data?.tools ?? []}
                  renderItem={(tool) => (
                    <List.Item>
                      <Space>
                        <Tag color="blue">{tool.category}</Tag>
                        <strong>{tool.name}</strong>
                        {tool.requiresApproval && <Tag color="volcano">需审批</Tag>}
                        <span>{tool.description}</span>
                      </Space>
                    </List.Item>
                  )}
                />
              </Card>
            </Space>
          ),
        },
        {
          key: "skills",
          label: "技能生态与在线 Skill IDE",
          children: (
            <Row gutter={16}>
              <Col span={12}>
                <Card title="创作技能库 (Built-in & Custom Skills)">
                  <List
                    dataSource={skillsQuery.data?.items ?? []}
                    renderItem={(sk) => (
                      <List.Item>
                        <List.Item.Meta
                          title={
                            <Space>
                              <strong>{sk.name}</strong>
                              <Tag color="cyan">{sk.category}</Tag>
                              {sk.isBuiltin && <Tag color="green">官方内置</Tag>}
                            </Space>
                          }
                          description={sk.description}
                        />
                      </List.Item>
                    )}
                  />
                </Card>
              </Col>
              <Col span={12}>
                <Card title="在线多文件 Skill IDE (创建自定义技能)">
                  <Form
                    layout="vertical"
                    initialValues={{
                      category: "storyboard",
                      skillMd: "# 自定义分镜技能\n描述技能规则与示例输出。",
                    }}
                    onFinish={(v) => createSkillMutation.mutate(v)}
                  >
                    <Form.Item name="name" label="技能名称" rules={[{ required: true }]}>
                      <Input placeholder="例如：悬疑短剧黄金前 3 秒钩子拆解" />
                    </Form.Item>
                    <Form.Item name="category" label="分类">
                      <Select
                        options={[
                          { value: "storyboard", label: "分镜 (storyboard)" },
                          { value: "writing", label: "剧本写作 (writing)" },
                          { value: "image", label: "图像提示词 (image)" },
                          { value: "video", label: "视频运镜 (video)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="description" label="简介" rules={[{ required: true }]}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="systemPrompt" label="系统提示词 (System Prompt)" rules={[{ required: true }]}>
                      <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="skillMd" label="SKILL.md 文档源码">
                      <Input.TextArea rows={4} style={{ fontFamily: "monospace" }} />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" loading={createSkillMutation.isPending}>
                      保存技能包
                    </Button>
                  </Form>
                </Card>
              </Col>
            </Row>
          ),
        },
        {
          key: "mcp",
          label: "远程 MCP 服务与长期记忆",
          children: (
            <Row gutter={16}>
              <Col span={12}>
                <Card title="挂载远程 MCP 服务 (streamable_http / sse)">
                  <Form layout="vertical" onFinish={(v) => createMcpMutation.mutate(v)}>
                    <Form.Item name="name" label="MCP 服务名称" rules={[{ required: true }]}>
                      <Input placeholder="例如：影视镜头参考库 MCP" />
                    </Form.Item>
                    <Form.Item name="endpoint" label="远程 Endpoint URL" rules={[{ required: true }]}>
                      <Input placeholder="https://mcp.example.com/v1/stream" />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" loading={createMcpMutation.isPending}>
                      注册 MCP Server
                    </Button>
                  </Form>
                  <List
                    style={{ marginTop: 16 }}
                    dataSource={mcpQuery.data?.items ?? []}
                    renderItem={(s) => (
                      <List.Item>
                        <Space>
                          <Tag color="geekblue">{s.transport}</Tag>
                          <strong>{s.name}</strong>
                          <span>{s.endpoint}</span>
                        </Space>
                      </List.Item>
                    )}
                  />
                </Card>
              </Col>
              <Col span={12}>
                <Card title="Agent 长期创作记忆文档 (Memory Documents)">
                  <Form layout="vertical" onFinish={(v) => createMemoryMutation.mutate(v)}>
                    <Form.Item name="title" label="记忆标题" rules={[{ required: true }]}>
                      <Input placeholder="例如：团队角色视觉规范" />
                    </Form.Item>
                    <Form.Item name="content" label="记忆内容" rules={[{ required: true }]}>
                      <Input.TextArea rows={3} placeholder="所有女主镜头默认采用 35mm 电影镜头、柔光侧逆光..." />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" loading={createMemoryMutation.isPending}>
                      写入记忆库
                    </Button>
                  </Form>
                  <List
                    style={{ marginTop: 16 }}
                    dataSource={memoriesQuery.data?.items ?? []}
                    renderItem={(m) => (
                      <List.Item>
                        <List.Item.Meta title={m.title} description={m.content} />
                      </List.Item>
                    )}
                  />
                </Card>
              </Col>
            </Row>
          ),
        },
      ]}
    />
  );
}
