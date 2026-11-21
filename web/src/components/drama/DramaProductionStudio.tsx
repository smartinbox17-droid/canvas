import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  List,
  Row,
  Select,
  Space,
  Steps,
  Table,
  Tag,
  message,
} from "antd";
import { dramaApi, type DramaDetailResponse } from "@/services/api/drama";

export function DramaProductionStudio() {
  const queryClient = useQueryClient();
  const [activeDetail, setActiveDetail] = useState<DramaDetailResponse | null>(null);
  const [jianyingJson, setJianyingJson] = useState<string | null>(null);

  const dramasQuery = useQuery({
    queryKey: ["dramas", "list"],
    queryFn: () => dramaApi.listDramas(),
  });

  const createMutation = useMutation({
    mutationFn: (v: { title: string; rawScript: string; pacingPreset: string }) =>
      dramaApi.createDrama(v),
    onSuccess: (res) => {
      setActiveDetail(res);
      setJianyingJson(null);
      void message.success("剧本已自动拆解为故事圣经、角色设定集与导演级分镜表");
      void queryClient.invalidateQueries({ queryKey: ["dramas", "list"] });
    },
  });

  const runOrchestratorMutation = useMutation({
    mutationFn: (dramaId: string) => dramaApi.startOrchestrator(dramaId),
    onSuccess: (res) => {
      setActiveDetail(res);
      void message.success("已完成 <=10s 剧情段切分、尾帧续接出片、SRT 字幕合成并同步至精修无限画布");
      void queryClient.invalidateQueries({ queryKey: ["dramas", "list"] });
      void queryClient.invalidateQueries({ queryKey: ["canvas", "projects"] });
    },
  });

  const exportJianyingMutation = useMutation({
    mutationFn: (p: { dramaId: string; episodeId: string }) =>
      dramaApi.exportJianyingDraft(p.dramaId, p.episodeId),
    onSuccess: (res) => {
      setJianyingJson(JSON.stringify(res.draft, null, 2));
      void message.success("剪映草稿包 (Jianying Draft + SRT) 已生成");
    },
  });

  return (
    <Space direction="vertical" style={{ width: "100%" }} size="large">
      <Row gutter={16}>
        <Col span={10}>
          <Card title="1. 新建 AI 短剧项目 (剧本上传 & 节奏档位)">
            <Form
              layout="vertical"
              initialValues={{
                title: "赛博雨夜追缉令 · 第一季",
                pacingPreset: "fast_hook",
                rawScript:
                  "第一集：霓虹雨夜。林零撑着透明雨伞站在高架桥下，全息目镜闪烁收到报警信号。她猛然转身，三架巡逻无人机穿过浓雾锁定地面。",
              }}
              onFinish={(v) => createMutation.mutate(v)}
            >
              <Form.Item name="title" label="短剧名称" rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="pacingPreset" label="分镜叙事节奏 (5 档预设)">
                <Select
                  options={[
                    { value: "fast_hook", label: "黄金前 3 秒快钩子 (fast_hook)" },
                    { value: "standard", label: "标准网剧节奏 (standard)" },
                    { value: "cinematic", label: "电影长镜头感 (cinematic)" },
                    { value: "dialogue", label: "高频对白交锋 (dialogue)" },
                    { value: "montage", label: "高燃蒙太奇 (montage)" },
                  ]}
                />
              </Form.Item>
              <Form.Item name="rawScript" label="原著小说 / 剧本正文" rules={[{ required: true }]}>
                <Input.TextArea rows={4} />
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={createMutation.isPending}>
                AI 拆集 & 提取角色设定与导演分镜表
              </Button>
            </Form>
          </Card>
        </Col>

        <Col span={14}>
          <Card title="短剧工程列表 (点击载入流水线)">
            <List
              dataSource={dramasQuery.data?.items ?? []}
              locale={{ emptyText: "暂无短剧工程，请在左侧创建" }}
              renderItem={(d) => (
                <List.Item
                  actions={[
                    <Button
                      key="open"
                      size="small"
                      onClick={async () => {
                        const res = await dramaApi.getDrama(d.id);
                        setActiveDetail(res);
                        setJianyingJson(null);
                      }}
                    >
                      载入工作台
                    </Button>,
                    <Button
                      key="run"
                      type="primary"
                      size="small"
                      loading={runOrchestratorMutation.isPending}
                      onClick={() => runOrchestratorMutation.mutate(d.id)}
                    >
                      一键尾帧串联出片 & 同步画布
                    </Button>,
                  ]}
                >
                  <List.Item.Meta
                    title={
                      <Space>
                        <strong>{d.title}</strong>
                        <Tag color="purple">{d.pacingPreset}</Tag>
                        <Tag color={d.stage === "completed" ? "green" : "blue"}>{d.stage}</Tag>
                      </Space>
                    }
                    description={
                      d.canvasProjectId
                        ? `已绑定精修画布 ID: ${d.canvasProjectId}`
                        : "尚未同步至精修画布"
                    }
                  />
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>

      {activeDetail && (
        <Card
          title={`短剧全流程导演台 · ${activeDetail.drama.title}`}
          extra={
            <Space>
              <Button
                type="primary"
                loading={runOrchestratorMutation.isPending}
                onClick={() => runOrchestratorMutation.mutate(activeDetail.drama.id)}
              >
                执行分段切片 + 尾帧串联 + FFmpeg 成片合成
              </Button>
              {activeDetail.episodes[0] && (
                <Button
                  onClick={() =>
                    exportJianyingMutation.mutate({
                      dramaId: activeDetail.drama.id,
                      episodeId: activeDetail.episodes[0].id,
                    })
                  }
                >
                  导出剪映草稿 + SRT 字幕
                </Button>
              )}
            </Space>
          }
        >
          <Steps
            style={{ marginBottom: 20 }}
            current={activeDetail.drama.stage === "completed" ? 4 : 1}
            items={[
              { title: "故事圣经 & 拆集" },
              { title: "角色四视图 / 场景三视图" },
              { title: "结构化分镜表" },
              { title: "<=10s 剧情段 & 尾帧续接" },
              { title: "成片合成 & 剪映/画布同步" },
            ]}
          />

          <h4>角色与场景设定集 (Cast Turnaround & Scene Sheets)</h4>
          <Row gutter={12} style={{ marginBottom: 20 }}>
            {activeDetail.cast.map((c) => (
              <Col span={12} key={c.id}>
                <Card size="small" title={`${c.name} [${c.roleType}]`}>
                  <p>{c.description}</p>
                  <Tag color="cyan">Prompt: {c.imagePrompt.slice(0, 64)}...</Tag>
                </Card>
              </Col>
            ))}
          </Row>

          <h4>结构化导演分镜表 (Storyboard Shots)</h4>
          <Table
            size="small"
            rowKey="id"
            pagination={false}
            style={{ marginBottom: 20 }}
            dataSource={activeDetail.shots}
            columns={[
              { title: "镜号", dataIndex: "shotIndex", width: 60 },
              { title: "时长(s)", dataIndex: "durationSec", width: 80 },
              { title: "景别", dataIndex: "shotSize", width: 140 },
              { title: "运镜", dataIndex: "cameraMotion", width: 180 },
              { title: "画面与表演调度", dataIndex: "sceneDescription" },
              { title: "台词 / 音效", dataIndex: "dialogue" },
              { title: "连续性衔接 (continuityOut)", dataIndex: "continuityOut" },
            ]}
          />

          {activeDetail.segments.length > 0 && (
            <>
              <h4>已切分视频生成段 (≤10s 切片 & 尾帧续接链 Tail-Frame Chaining)</h4>
              <List
                dataSource={activeDetail.segments}
                renderItem={(seg) => (
                  <List.Item>
                    <List.Item.Meta
                      title={
                        <Space>
                          <Tag color="green">Segment #{seg.segmentIndex}</Tag>
                          <span>时长: {seg.durationSec}s</span>
                          <Tag color="blue">首帧: {seg.firstFrameUrl}</Tag>
                          <Tag color="purple">抽取尾帧: {seg.tailFrameUrl}</Tag>
                        </Space>
                      }
                      description={`拼接运动提示词: ${seg.combinedPrompt}`}
                    />
                  </List.Item>
                )}
              />
            </>
          )}

          {jianyingJson && (
            <Alert
              type="success"
              style={{ marginTop: 16 }}
              message="剪映草稿工程 JSON (jianying.draft/v1 + SRT 字幕轨)"
              description={<pre style={{ maxHeight: 240, overflow: "auto" }}>{jianyingJson}</pre>}
            />
          )}
        </Card>
      )}
    </Space>
  );
}
