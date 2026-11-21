import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  List,
  Row,
  Select,
  Space,
  Tabs,
  Tag,
  message,
} from "antd";
import { previsEcommerceApi } from "@/services/api/previs-ecommerce";

export function PrevisEcommerceTeamsStudio() {
  const queryClient = useQueryClient();

  const previsQuery = useQuery({
    queryKey: ["previs", "scenes"],
    queryFn: () => previsEcommerceApi.listPrevisScenes(),
  });

  const ecomProjectsQuery = useQuery({
    queryKey: ["ecommerce", "projects"],
    queryFn: () => previsEcommerceApi.listEcommerceProjects(),
  });

  const ecomKbQuery = useQuery({
    queryKey: ["ecommerce", "kb"],
    queryFn: () => previsEcommerceApi.listKnowledge(),
  });

  const teamsQuery = useQuery({
    queryKey: ["teams", "list"],
    queryFn: () => previsEcommerceApi.listTeams(),
  });

  const createPrevisMutation = useMutation({
    mutationFn: (v: {
      title: string;
      cityPreset: string;
      weather: string;
      cameraRecipe: string;
      characterPose: string;
    }) => previsEcommerceApi.createPrevisScene(v),
    onSuccess: (res) => {
      void message.success(`3D Previs 预演深度控制视频已生成并回写至画布节点 (${res.scene.canvasNodeId})`);
      void queryClient.invalidateQueries({ queryKey: ["previs", "scenes"] });
      void queryClient.invalidateQueries({ queryKey: ["canvas", "projects"] });
    },
  });

  const createEcommerceMutation = useMutation({
    mutationFn: (v: {
      productName: string;
      categoryCode: string;
      targetPlatform: string;
      regionCasting: string;
    }) => previsEcommerceApi.createEcommerceProject(v),
    onSuccess: () => {
      void message.success("已完成 5 步电商合规门禁校验、8%-92% 安全区排版与区域选角出图");
      void queryClient.invalidateQueries({ queryKey: ["ecommerce", "projects"] });
    },
  });

  const createTeamMutation = useMutation({
    mutationFn: (v: { name: string; poolCredits: number }) =>
      previsEcommerceApi.createTeam({
        name: v.name,
        poolMicrocredits: v.poolCredits * 1_000_000,
      }),
    onSuccess: () => {
      void message.success("二级团队空间与公共积分池已创建");
      void queryClient.invalidateQueries({ queryKey: ["teams", "list"] });
    },
  });

  return (
    <Tabs
      items={[
        {
          key: "previs",
          label: "3D 世界引擎 (Previs 导演预演台)",
          children: (
            <Row gutter={16}>
              <Col span={10}>
                <Card title="构建 3D 导演预演场景 (7 大城市 / 7 种天气 / 25 运镜)">
                  <Form
                    layout="vertical"
                    initialValues={{
                      title: "高架桥霓虹雨夜追逐预演",
                      cityPreset: "cyberpunk_blocks",
                      weather: "neon_night",
                      cameraRecipe: "dolly_zoom",
                      characterPose: "umbrella_stand",
                    }}
                    onFinish={(v) => createPrevisMutation.mutate(v)}
                  >
                    <Form.Item name="title" label="预演场景名" rules={[{ required: true }]}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="cityPreset" label="程序化城市场景预设">
                      <Select
                        options={[
                          { value: "cyberpunk_blocks", label: "赛博霓虹街区 (cyberpunk_blocks)" },
                          { value: "modern_cbd", label: "现代摩天 CBD (modern_cbd)" },
                          { value: "ancient_courtyard", label: "古风庭院街巷 (ancient_courtyard)" },
                          { value: "sci_fi_hangar", label: "科幻机甲机库 (sci_fi_hangar)" },
                          { value: "desert_highway", label: "荒漠公路 (desert_highway)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="weather" label="物理天空与天气系统">
                      <Select
                        options={[
                          { value: "neon_night", label: "雨夜霓虹反射 (neon_night)" },
                          { value: "rain", label: "暴雨体积雾 (rain)" },
                          { value: "dusk", label: "黄金夕阳逆光 (dusk)" },
                          { value: "fog", label: "丁达尔晨雾 (fog)" },
                          { value: "clear", label: "晴朗正午 (clear)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="cameraRecipe" label="大师运镜配方">
                      <Select
                        options={[
                          { value: "dolly_zoom", label: "希区柯克滑动变焦 (dolly_zoom)" },
                          { value: "orbit_360", label: "360° 英雄环绕 (orbit_360)" },
                          { value: "low_angle_track", label: "低角度仰拍跟移 (low_angle_track)" },
                          { value: "crane_up", label: "摇臂升空俯瞰 (crane_up)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="characterPose" label="骨骼角色姿势 & A/B 地面标记">
                      <Select
                        options={[
                          { value: "umbrella_stand", label: "撑伞驻足回眸 (A→B 轨迹)" },
                          { value: "action_sprint", label: "高速冲刺闪避 (A→B 轨迹)" },
                          { value: "combat_ready", label: "战术拔刀戒备" },
                        ]}
                      />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" block loading={createPrevisMutation.isPending}>
                      渲染白模 MP4 + 定标深度视频并回写无限画布
                    </Button>
                  </Form>
                </Card>
              </Col>
              <Col span={14}>
                <Card title="已导出的 3D Previs 预演场景与深度控制节点">
                  <List
                    dataSource={previsQuery.data?.items ?? []}
                    locale={{ emptyText: "暂无 3D 预演场景" }}
                    renderItem={(sc) => (
                      <List.Item>
                        <List.Item.Meta
                          title={
                            <Space>
                              <strong>{sc.title}</strong>
                              <Tag color="purple">{sc.cityPreset}</Tag>
                              <Tag color="cyan">{sc.weather}</Tag>
                              <Tag color="blue">{sc.cameraRecipe}</Tag>
                            </Space>
                          }
                          description={`已回写画布节点: ${sc.canvasNodeId} · 定标深度视频: ${sc.depthVideoUrl}`}
                        />
                      </List.Item>
                    )}
                  />
                </Card>
              </Col>
            </Row>
          ),
        },
        {
          key: "ecommerce",
          label: "电商 Agent 与离线合规知识库",
          children: (
            <Row gutter={16}>
              <Col span={10}>
                <Card title="5 步电商合规出图流 (8%-92% 安全区 + 区域选角)">
                  <Form
                    layout="vertical"
                    initialValues={{
                      productName: "AI 智能降噪耳机 Pro",
                      categoryCode: "consumer_electronics",
                      targetPlatform: "amazon",
                      regionCasting: "north_america",
                    }}
                    onFinish={(v) => createEcommerceMutation.mutate(v)}
                  >
                    <Form.Item name="productName" label="商品名称" rules={[{ required: true }]}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="targetPlatform" label="目标海内外电商平台 (36 平台规则库)">
                      <Select
                        options={[
                          { value: "amazon", label: "Amazon (纯白底 85% 占比门禁)" },
                          { value: "tiktok_shop", label: "TikTok Shop (痛点对比与本土模特)" },
                          { value: "tmall", label: "天猫 Tmall (高奢流体背光)" },
                          { value: "shopify", label: "Shopify 独立站 (DTC 品牌叙事)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="categoryCode" label="双语类目树节点">
                      <Select
                        options={[
                          { value: "consumer_electronics", label: "3C 消费电子 (consumer_electronics)" },
                          { value: "fashion_apparel", label: "服饰鞋包 (fashion_apparel)" },
                          { value: "beauty_skincare", label: "美妆个护 (beauty_skincare)" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="regionCasting" label="区域本土模特选角 (Region Casting)">
                      <Select
                        options={[
                          { value: "north_america", label: "北美本土模特 (north_america)" },
                          { value: "southeast_asia", label: "东南亚本土模特 (southeast_asia)" },
                          { value: "europe", label: "欧洲本土模特 (europe)" },
                          { value: "east_asia", label: "东亚本土模特 (east_asia)" },
                        ]}
                      />
                    </Form.Item>
                    <Button
                      type="primary"
                      htmlType="submit"
                      block
                      loading={createEcommerceMutation.isPending}
                    >
                      生成合规主图与区域模特场景套图
                    </Button>
                  </Form>
                </Card>
              </Col>
              <Col span={14}>
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                  <Card title="电商套图项目与合规评分">
                    <List
                      dataSource={ecomProjectsQuery.data?.items ?? []}
                      locale={{ emptyText: "暂无电商出图项目" }}
                      renderItem={(p) => (
                        <List.Item>
                          <List.Item.Meta
                            title={
                              <Space>
                                <strong>{p.productName}</strong>
                                <Tag color="blue">{p.targetPlatform}</Tag>
                                <Tag color="green">合规分: {p.complianceScore}/100</Tag>
                                <Tag color="gold">排版安全区: {p.safeAreaMargin}</Tag>
                              </Space>
                            }
                            description={`类目: ${p.categoryCode} · 区域选角: ${p.regionCasting}`}
                          />
                        </List.Item>
                      )}
                    />
                  </Card>
                  <Card title="离线平台规则与卖点知识库 (零向量库检索引擎)">
                    <List
                      dataSource={ecomKbQuery.data?.items ?? []}
                      renderItem={(kb) => (
                        <List.Item>
                          <List.Item.Meta
                            title={
                              <Space>
                                <Tag color="geekblue">{kb.platform}</Tag>
                                <strong>{kb.ruleTitle}</strong>
                                <Tag>{kb.safeZone}</Tag>
                              </Space>
                            }
                            description={kb.ruleBody}
                          />
                        </List.Item>
                      )}
                    />
                  </Card>
                </Space>
              </Col>
            </Row>
          ),
        },
        {
          key: "teams",
          label: "团队空间与多租户积分池 (Phase 9)",
          children: (
            <Row gutter={16}>
              <Col span={10}>
                <Card title="创建二级团队空间 (公司 → 小组 & 公共积分池)">
                  <Form
                    layout="vertical"
                    initialValues={{ name: "AIGC 影视制作一组", poolCredits: 5000 }}
                    onFinish={(v) => createTeamMutation.mutate(v)}
                  >
                    <Form.Item name="name" label="团队/小组名称" rules={[{ required: true }]}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="poolCredits" label="划拨公共积分池额度">
                      <InputNumber min={100} style={{ width: "100%" }} />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" block loading={createTeamMutation.isPending}>
                      创建团队空间
                    </Button>
                  </Form>
                </Card>
              </Col>
              <Col span={14}>
                <Card title="已管理的团队空间列表">
                  <List
                    dataSource={teamsQuery.data?.items ?? []}
                    locale={{ emptyText: "暂无团队空间" }}
                    renderItem={(t) => (
                      <List.Item>
                        <List.Item.Meta
                          title={<strong>{t.name}</strong>}
                          description={`团队 ID: ${t.id} · 公共积分池: ${(t.poolMicrocredits / 1_000_000).toFixed(0)} 积分`}
                        />
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
