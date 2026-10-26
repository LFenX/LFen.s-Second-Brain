# 阶段3-R2：文档结构客户端唯一写窗

2026-10-27，v0.62。真人已批准 `02c1951` 的 Document Structure 0.1.0；批准和三件摘要见 `knowledge-document-structure-v1.freeze.json`。本文件只规定交替边界，**取得 A 明确消息及干净固定 HEAD 后才开启写窗**。R2 工程验收、R1 人的决定、真实资料退出和阶段3整体均独立 Pending。

## 固定后端与证据

- A 已实现并加载 Go 二进制 SHA `b563a51dad95594fb448896f762a82eba82c3774f9167e078f8405246ef4a2f7`，新能力三个 profile 均 ready。现有 Web 仍固定 `70eebed`；原六服务／卷／凭据保留。
- [中间报告](stage3r2-document-backend-verification.json)：85 顶层／733 含子通过事件；27 预标源事实、4 身份作用域、14 组 owned-child 重复20次通过；计数有重叠，不相加。实际 MySQL／原件存储／HTTP 核心716次请求，三格式79节点、树／manifest／节点身份、文字范围与完整覆盖、固定下载、三类原键及负权限通过。
- 正式 `knowledge-document-structure-v1.openapi.json`、examples、freeze 及候选原文只读。完整47件保护哈希见中间报告 `contracts.protected_files`；不得修改任何一件。正式 DTO 101 schemas、DC01—DC12，不能消费旧私稿替代。
- 当前合成域 `87778c6ec71d4c4d8cb72a6c44741005` 只用于 A 后续真实联调；文档策略最终关闭，代码策略及基础六用途保持禁止。接方不登录、不调用该运行域，不从数据库／凭据文件取得信息。
- A 实际故障、三格式容量／RSS／首屏及浏览器尚待完成；不能把本后端中间交付记为整 R2 通过。

## 写入白名单与单写入

A 给固定 HEAD 后，接方成为该分支唯一 tracked／Git 写入者；不切分支、不 rebase/reset、不合 main、不推 main、不部署。A 停止 tracked、Git和部署变更，只在 ignored 目录准备／执行独立合成验证。接方只做 Mock／隔离组件／构建，不变更运行数据；任何后端或扩大文件范围需要先消息交回写权。

允许路径（保留现有目录／命名，不重排）：

- 新增 `web/src/interfaces/request/knowledge-document*.ts`、`web/src/interfaces/database/knowledge-document*.ts`、`web/src/services/knowledge-document*-service.ts`、`web/src/hooks/use-knowledge-document*.ts`、`web/src/utils/knowledge-document*.ts`，及对应现有 `__tests__/` 中同前缀定向测试／合成夹具。
- `web/src/pages/dataset/knowledge-catalogue/` 内新增 `document-*.tsx`、`document-*.ts`、`use-document-*.ts` 及其 `__tests__/document-*`；最小接入现有 `index.tsx`。复用 `.5` 原件下载和 `.7` 当前世代读取的正式客户端，不重写旧流程。
- 仅新功能所需的 `web/src/locales/zh.ts`、`web/src/locales/en.ts`。
- 客户端交付记录 `docs/stage3r2-document-client-verification.json`、`docs/stage3r2-document-client-notes.md`。

其他 Go、根中文文档／README、冻结件、依赖锁、运行配置、共享 `components/ui/` 均只读。现有 `services/document-structure-service.ts` 和 `interfaces/database/document-structure.ts` 属于旧图功能，保持只读，不覆盖。读并遵守 `web/AGENTS.md`，统一 service→hook→局部页面及 query-key factory。

## 必须实现的语义

1. 12 操作严格基础1.0.0＋Document0.1.0双头；DC01能力为真实 producer/profile。首次写201、原键重放200、412版本不符、429仅固定拒绝、503结果未知；错误 hint 不代替完整原键记录。
2. 从一份 `.5` 已保存固定原件开始准备；另取 `.7` active世代、独立文档策略、基础策略、真实 profile／producer和source head。复核 source／revision／SHA／字节数／MIME／路径／分类／双策略及世代；任一变化停止准备，不自动换目标或确认。三种 profile 对应 PDF／DOCX／PPTX，处理预算8MiB，原件旧64MiB保存规则不改。
3. 独立 `document_structure_only` 默认关闭，策略明确确认。文档策略不打开代码结构或模型用途；关闭只令 head stale，不抹掉有权固定历史。显示 source/policy/producer 等所有 stale reasons；不能把 archive 当撤权。
4. task/policy/cancel POST 前存完整首次body、key、规范摘要与 owner/domain/target；三类恢复槽按主体隔离，每主体8槽、每槽16KiB。存储失败／满额／损坏禁新POST，不驱逐未知。401/404/412/503/超时、单独 rejection_id 均不清槽；完整 by-key记录匹配后才恢复。未知不自动换键，固定拒绝后明确放弃再重新准备。
5. queued/running/cancel_requested及 phase/progress=null 真实显示；取消回执不当终态。取消CAS先取 revision，已有任务跟踪不因取消槽结束而丢失。前台最多2秒一次，隐藏／离页／终态停止，25秒转恢复，不自动POST。
6. 固定 Representation／DocumentNode 与当前 head 分别缓存。父子直接页≤20，opaque cursor，不能一次加载全树。新文档根64hex不进入C1。`.5` 原件回链先核 domain/source/revision/SHA/长度/MIME，复用完整SHA授权下载，不使用当前preview／匿名URL。
7. 按页／slide／section显示安全文本和物理位置：PDF不承诺OCR或版面；DOCX无Word物理页。`derived_utf8` 半开字节范围不是原件offset。表格按行列／空cell、列表按item、内联媒体零宽anchor；空白／仅图／unsupported明确显示，禁止拼接父聚合正文造成重复。
8. heading自身取 `own_reading_range`，聚合整节由用户明确选择；`complete`相对aggregate节点，读完 own_end 即停，即使 complete=false。自身范围为空不发非推进读取。每窗≤64KiB、UTF8边界、片段SHA、own SHA及aggregate SHA区分；只在拥有全部对应字节后核整体摘要。显式end不能被静默截断。
9. 固定入口参数 `document_structure=1`、`document_source`、`document_revision`、`document_original_sha`、`document_representation` 和可选 `document_node`；不全／不符报错，不退回当前head。导航、刷新和跨session保留正确固定身份。
10. 内容只作为转义文本／只读代码，不能执行HTML、加载文内远程媒体或 URL。失权／换session先隐藏私有内容、中止请求、丢弃迟到响应；原owner恢复槽不丢失、也不能在新owner下显示。

## 验证、回交与待办

- 消费140合法／148拒绝例、28身份向量及10文字向量，另做12操作、三类完整恢复／原键冲突、精确原件准备、heading own窗口、分页／读取边界、unsupported／空白／图、stale／失权／session及轮询生命周期定向测试。机器示例不当真实后端竞态证明。
- 只合成 Mock／隔离渲染。运行触达、浏览器真正同版闭环、数据库／进程故障和性能由 A统一完成；不得调用模型／本地推理，不启动多轮审查。
- 局部 oxfmt／oxlint、必要定向测试、生产构建、当前全量类型基线和新增量、正常 hooks。207是历史错误数，必须实测当前值，不扩修既有错误。
- 正常提交当前 `codex/stage3r2-document`，可推同名分支，回交清洁HEAD、文件白名单、47保护哈希核对、报告、构建产物路径与摘要、未完成项。不要把 Mock 通过或本轮冻结记为用户工程验收。
- A 收回唯一写权后完成实际合成浏览器／故障／三格式各五次任务及各类20读取、50ms名义父子RSS和首屏。发现实际问题只针对性修复，验证后按真人授权推 main，人的R2决定独立记录。
