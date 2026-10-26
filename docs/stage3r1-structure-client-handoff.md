# 阶段3-R1：客户端唯一写窗交接

2026-10-26，v0.59。具体写窗须由A在清洁提交后发消息开启；本文件自身不授予并行写入。真人已冻结ddc5630的Structure 0.1.0；R6与R7纯合成验收独立Approved。整R1、阶段3与真实资料退出均未验收。

## 固定输入与实际后端

- 正式三件：`knowledge-structure-v1.freeze.json`、`knowledge-structure-v1.openapi.json`、`knowledge-structure-v1.examples.json`。旧35件、原候选3件和正式3件共41件保护，见`stage3r1-structure-backend-verification.json`；不得编辑。
- Go已加载二进制SHA `0efd5563a48a87d0cc41c2661a2ee58dcbc2a1ec88b2bc952b53e152acdcc061`，实际能力`runtime_ready=true`。原六服务、卷、凭据保留；Web当前仍`c759990`。A后端固定提交由开启窗口消息补充，不能把本报告当全片完成。
- 实际MySQL/S3：8份合成TS/TSX/Go/Markdown，48个结构节点、80条关系、50项事实通过；原件、passage覆盖、typed-LP身份和关系证据独立核对。第一次能力DTO数组错误已修；四mode帮助与新能力JSON往返通过。
- `GET /api/v1/knowledge/structure-contract`与域下15操作严格使用基础1.0.0＋Structure0.1.0双头；首次新写201、恢复200，版本412。旧C2.4异步来源任务仍202，不改旧协议。

## 接方拥有范围

取得明确写窗后，接方成为唯一tracked源码／Git写入者。A停止tracked与部署写入，只在忽略目录准备／运行独立合成验证；接方不执行真实后端导入、变更策略、故障或部署，不与A争用运行数据。需要运行接口或扩大文件范围先通过协调消息串行安排，不要求真人重复批准既有范围。

允许的Web路径：

- 新增`src/interfaces/request/knowledge-structure.ts`、`src/interfaces/database/knowledge-structure.ts`、`src/services/knowledge-structure-service.ts`、`src/hooks/use-knowledge-structure.ts`、`src/utils/knowledge-structure-schema.ts`、`src/utils/knowledge-structure.ts`及对应定向测试／合成夹具。
- 在既有`src/pages/dataset/knowledge-catalogue/`内新增结构入口、策略、精确准备、任务、视图／树、原文和一跳关系组件／flow及测试；组件可在此目录按拥有职责拆分，不能重排既有目录。
- 最小接入`knowledge-catalogue/index.tsx`、zh/en对应本地化文件；如需现有`batch-manifest.tsx`的ProjectPicker，仅局部导出和复用，不改.4行为。
- 客户端交付记录`docs/stage3r1-structure-client-verification.json`和`docs/stage3r1-structure-client-notes.md`。

保持Go、正式冻结件、中文主方案、README、运行配置、依赖锁、共享`components/ui/`只读；若证明需要改共享工具或依赖，先给A具体拥有路径／理由，再决定交替写窗。读并遵守`web/AGENTS.md`。不建新独立后台、不触碰Go/Python切换、不接模型、PDF/PPTX/DOCX或Wiki。

## 必须落地的消费语义

1. 独立service→hook→catalogue局部组件，复用现有树／分栏／只读Monaco／列表。组织树、源文件成员、派生结构树分开；结构ID64hex只走新接口，C1根只作固定回链。
2. 精确选择来自.4当前固定manifest，再逐项.7 detail核修订、路径、分类、active与世代；不从manifest推世代、不自动替换。parser必须匹配后缀；32文件／2MiB单件／16MiB合计。alias target为空或带尾`/`的相对目录，如`frontend/`；至多8个不重叠prefix。
3. 基础catalogue／六用途模型禁止，与structure-only enabled/version并列。写策略明确确认，固定回执后另读当前策略。关闭不隐藏当前有权历史。
4. task/policy/cancel三个写操作都POST前预存完整原body／key／规范摘要和owner/domain/目标；每主体8槽、每槽16KiB。存储失败／损坏／满额禁新POST，不驱逐未知。cancel路径task_id进入身份；放弃cancel不能丢task跟踪。保留首次member/alias顺序，摘要排序而原请求不排序。
5. by-key识别Task或固定Rejection；命令识别策略／取消／固定拒绝。404/401/412/超时/503、错误信封hint和单独rejection_id均不得清槽。只在GET核对完整固定拒绝后开放明确放弃→重读→新body/key→重新认可；异摘要冲突不覆盖。固定回执不当当前状态。
6. revision是取消CAS；`cancel_requested`不是终态；staged/reused在整个Task成功前不可读为已发布节点。等待25秒转恢复，前台轮询至多2秒一次，隐藏／离页／终态停止，无后台自动POST或新键。
7. 固定view与当前head分别缓存。head的全局reasons即使affected_sources为空也显示；归档不是撤权。节点／关系按scope/limit/opaque cursor懒加载，页≤20，不拉全图。仅resolved且同view目标可跳转，unresolved保留原因／原引用／证据。
8. 原文单窗≤64KiB；首次start/end可省，续页使用next_start_byte。显式end需start；核SHA／UTF8绝对字节范围／next／complete，不跟随当前头，不照搬旧C1的400缩窗重试。BOM／CRLF／emoji坐标独立转换，未读前缀不编造绝对列。
9. Markdown/HTML源作为转义文本／只读代码，不自动加载URL／图片。授权切换隐藏数据、清可见查询、保留原owner隔离恢复槽；迟到响应不得穿透新session。

## 接续与回交验收

- 直接消费39合法／37拒绝例及8摘要向量；另外测15操作、三类恢复槽、选择／别名／世代、超时／无权／stale、原文／树／关系以及轮询生命周期。静态schema例不算后端竞态证明。
- 使用合成Mock／隔离渲染验证，不调用模型或真实资料；浏览器真实同版闭环、数据库／进程故障与性能统一由A收回写权后执行。
- 记录真实文件范围、受影响测试、局部格式／lint、生产构建、重新取得的类型错误基线及新增量。旧207仅历史值，不据此假定本次结果；不扩大既有错误修复。
- 正常hooks提交到A开启窗口时指定的同一分支；不换分支、不rebase／reset、不合main、不部署。回交清洁HEAD、结果、已知缺口和保护摘要核对。没有真人验收则Pending；不询问或启动多轮审查。

A收回写权后补完整合成HTTP／浏览器／故障恢复／五次性能和实际RSS证据，定向修复，再按既有授权推main。后续富文档／Wiki及C4—C6保持各自候选和阶段边界。

## v0.60 写窗已交回

接方已在ca74ff8清洁回交，A持全部写权并完成70eebed局部修复、同版浏览器／真实后端／性能联调。[统一报告](stage3r1-structure-verification.json)与[当前接续](stage3r1-structure-handoff.md)覆盖本文件旧“前端尚未完成”时点；本文件不再授予任何新写窗。原client-verification及ui-fix-verification保留各提交时证据，最终浏览器复验见统一报告，不把其历史Pending改写成从未存在。客户端会话只在忽略目录整理富文档契约形状，等待A集中收敛及必要真人冻结，不改源码或运行。
