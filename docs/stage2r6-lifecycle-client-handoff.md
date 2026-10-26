# R6 生命周期客户端实施写窗

2026-10-26；正式C2 0.7.0已由真人批准并冻结。后端工程提交由A在交写权消息中点名，必须以该清洁HEAD开始；本说明不构造自身提交哈希。当前后端已加载二进制SHA `af6e939567afe2edb176d110096fd4626d2e1b8d92a59a31643f6ac9583c915e`，`runtime_ready=true`，Web仍R5。

## 1. 已有能力与边界

- [正式冻结](knowledge-source-lifecycle-v1.freeze.json)、[10操作／20schema](knowledge-source-lifecycle-v1.openapi.json)、[冻结向量](knowledge-source-lifecycle-v1.examples.json)。32份旧契约／候选和新增正式三件套均受保护；不修改这些文件、旧DTO、摘要投影、共享模型或Go。
- A已完成私有世代、三写头核心、所有旧准备／任务／项目的栅栏、四动作、逻辑位置／退役占位、原键固定回执、当前／归档列表。合成真实Go／MySQL／S3最小闭环已通过，尚未完成R6整体验证。
- 当前合成域、账户与旧过渡材料只在被忽略的`.local-data/stage2r6-20261026/old-preparations.private.json`及既有本机合成账户文件中。不得复制凭据到代码、聊天或报告。A持运行测试职责；消费会话本窗只做合成Mock与构建，不登录运行后端、不改配置、服务、数据或API。
- 已停止旧API／worker后迁移；不得回切无世代旧后端继续写入。旧成功回执和固定历史保留；0.2／0.5／0.6旧未消费准备已实际拒绝，同一有效Web Capture可新明确Preview且不重新联网。目录／批次／项目有定向Go过渡测试，完整运行补证由A负责。
- 不调用模型／本地推理／真实网站，不启动审查、不建设基础设施、不合main；本窗不取得main发布或其他目录写权。

## 2. 唯一可写范围

收到A明确交窗前仍完全只读。收到交窗后，消费会话独占以下源码与Git写权，A只写忽略运行证据；不切分支、不改他人现场、不触碰白名单外文件。

新增生产文件：

- `web/src/interfaces/request/knowledge-lifecycle.ts`
- `web/src/interfaces/database/knowledge-lifecycle.ts`
- `web/src/services/knowledge-lifecycle-service.ts`
- `web/src/hooks/use-knowledge-lifecycle.ts`
- `web/src/utils/knowledge-lifecycle-schema.ts`
- `web/src/utils/knowledge-lifecycle.ts`
- `web/src/pages/dataset/knowledge-catalogue/lifecycle.tsx`
- `web/src/pages/dataset/knowledge-catalogue/lifecycle-actions.tsx`
- `web/src/pages/dataset/knowledge-catalogue/lifecycle-receipt.tsx`
- `web/src/pages/dataset/knowledge-catalogue/use-lifecycle-flow.ts`

允许局部修改：

- `web/src/pages/dataset/knowledge-catalogue/index.tsx`
- `web/src/pages/dataset/knowledge-import/original-revisions.tsx`
- `web/src/pages/dataset/knowledge-import/web-revisions.tsx`
- `web/src/hooks/use-knowledge-foundation.ts`、`web/src/hooks/use-knowledge-web.ts`：只增当前列表失效所需key factory前缀，不改旧key身份。
- `web/src/locales/zh.ts`、`web/src/locales/en.ts`

新增定向测试／夹具：

- `web/src/services/__tests__/knowledge-lifecycle-service.test.ts`、`knowledge-lifecycle-wire.test.ts`
- `web/src/utils/__tests__/knowledge-lifecycle.test.ts`
- `web/src/hooks/__tests__/use-knowledge-lifecycle.test.tsx`
- `web/src/pages/dataset/knowledge-catalogue/__tests__/lifecycle.test.tsx`、`lifecycle-recovery.test.tsx`、`lifecycle-fixture.ts`

以上测试同目录下的第二个文件名继承所在路径。既有导入页面测试可只读回归；如确需修改旧flow或白名单外文件，先交具体问题与当前未提交现场给A，不自行扩大写入。

## 3. 实施和验收

1. 正式service覆盖全部10操作，原生登录及局部双版本头，25秒、32KiB、严格正式schema、错误`outcome_known`；不能修改全局request或旧协议。
2. Hooks使用统一key factory，按session／owner／domain／source及完整过滤隔离；空语言、未传语言、all／ungrouped／exact不同。列表每页最多20，允许预算缩页；cursor复用完整原筛选和limit。
3. 页面提供当前与归档列表、独立当前详情、四动作差异与默认未认可。移动读取原登记精确目标，单独GET核对但不预约；目录／Web／无登记C1禁移动。状态no-op按协议处理，UI突出当前有意义的动作。
4. 先持久化完整原请求槽，再POST；每主体域1个未决、最多8域、单槽8KiB，满／损坏／存储失败禁止POST，不驱逐unknown。刷新／丢响应／超时／503先GET原动作原键；404不是未执行证明，核验当前访问后只明确重发相同body/key。失权清可见信息但保留隔离槽；换新准备必须先查原结果并明确放弃，重新读详情、换键及认可。
5. 固定回执与当前详情分开；回放旧archive不重归档。C1、原件和网页固定阅读继续旧协议，网页snapshot SHA为decoded正文，原Feed另有SHA。成功后精确失效当前来源列表，不把回执after当当前状态。
6. schema／摘要／HTTP wire、动作／认可重置、恢复／主体切换和容量槽、旧固定入口与已知过渡错误定向验证；旧用例只运行相关范围，不扩全仓错误修复。

验证命令沿用`web/package.json`与既有Jest配置：局部oxfmt／oxlint、新增6套定向用例＋相关旧页面回归、生产构建、全量类型增量。既有全量207错误不是通过；要求新增0并保存前后证据。原契约32份及正式0.7三件套SHA不变，正常Git hooks提交，不绕过检查。

## 4. 回交

交清洁HEAD、完整文件清单、实际命令／计数／日志定位、构建和类型增量、未覆盖浏览器项。可以新增`docs/stage2r6-lifecycle-client-verification.json`与`docs/stage2r6-lifecycle-client-notes.md`记录本窗事实；不得修改A的统一报告、中文方案／接续、协调文件或其他报告。

清洁回交后消费会话立即只读，A明确收回写权，再做浏览器、双API、SQL／进程故障、1000源五次容量、64MiB原件对象正文IO=0及旧链路定向验证。发现问题按具体路径修复，不启动多轮审查；工程完成后按既有真人授权推main，工程验收独立请求。
