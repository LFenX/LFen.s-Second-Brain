# S2-R4 文档原件接续

日期：2026-10-26；文档v0.39。**本批为C2 0.5.0候选，不是原件功能交付**。运行环境和main仍为已发布R3的`38ce003`；候选在`codex/stage2r4-original-candidate`。

执行入口：[行为定义](knowledge-original-import-v0.5.candidate.md)、[OpenAPI](knowledge-original-import-v0.5.candidate.openapi.json)、[合成向量](knowledge-original-import-v0.5.candidate.examples.json)、[统一核对记录](stage2r4-original-contract-verification.json)。旧C1/C3和C2 0.2／0.3／0.4共17项保护文件保持字节，不创建正式0.5冻结文件或加载未获批接口。

## 1. 谁在推进

- A为主协调及唯一文件／Git写入者；实际已把“完成即回报并接续下一项，不能全部因互等停工”的真人要求传给“阶段2知识消费与前端”会话。
- 消费会话`01a17244-5107-7aa3-beed-2ea36062d042`遵守其最新真人只读要求，已集中回传R4-C1—C6及Axios静态运输边界；A集中修订，不追加多轮审查，也不声称同行在最终版本上无发现。
- A继续R4验证／实施准备；消费会话接续R5网页／RSS本地只读能力勘察。R5调查不访问网站或新增接口，不替代R4实施。
- 持续推进不是越权授权。待确认接口先交具体结果；A可同时完成合成夹具、原生能力和下一阶段调查，不把其他会话空闲当自己停工理由。禁止同时改同一检出／索引／分支；交写权必须固定HEAD和白名单。

## 2. 待确认的最小方案

单件≤64MiB、每用户／域2个未结束会话／128MiB预留，30分钟固定TTL；只接PDF、PPTX、DOCX及固定UTF-8文本代码白名单。使用既有MinIO和原生Document／FileCommitItem；流式暂存后形成固定ready预览，明确确认后短SQL发布；正文不进JSON、不触发解析或模型。原名与逻辑路径分开，新增／更新／未变、双原键、失败状态、取消和固定GET／HEAD单Range均已定义。

原件修订为`original_only`，无C1文本根、无虚假行号，不进入冻结0.4项目清单。原件卡片挂在已有知识域／分组树；旧C1文本阅读保持。原件派生正文／页节点、混合清单和目录原件扫描另提候选。

创建恢复槽只保存单文件有界原请求（含原名／逻辑路径／分类），不保存正文／令牌／绝对路径或整个登记清单；确认槽保存完整原确认元组。登记丢响应通过配置键查询；不能凭404自动创建或更换幂等键。此新增恢复元信息与容量是本候选须确认的明确选择。

## 3. 获准后的单写入顺序

1. **A冻结**：保存真实批准原文，绑定候选三件套SHA，生成正式0.5资源；旧17文件保持。
2. **A后端存储／状态**：在现有`internal/storage`与`internal/service/file`补流式原语、统一快照元数据核心；在现有dataset/entity/handler/router扩展上传状态与原件投影。只新增本项需用的Go模型，不复制领域树／版本算法，不扩展Python。
3. **A后端发布／读取**：原键、CAS、短SQL确认、单调状态、域权限及原生旁路保护；固定原件GET／HEAD／单Range。先包级Go测试，再最小MinIO／SQL合成真实联调；本机代理缓冲和超时一起验证。
4. **前端接续**：待固定后端提交和明确写权后实施独立0.5客户端／Hooks、双恢复槽、单件选择／暂存／确认／回执／原件卡片及认证下载。现消费会话仍只读；不能因候选获批而擅自覆盖其真人限制，A可继续自身范围或按真人授权新增有明确任务的本地会话。
5. **A集成**：回交固定清洁提交后同版真实浏览器、故障、恢复、容量与旧链路回归；完整证据保存后推送main。不能把schema／Mock通过当真实上传、恒内存或工程Acceptance。

建议前端范围来自消费方一次反馈，**尚未授权写入**：

- 新增`web/src/interfaces/{request,database}/knowledge-original.ts`、`services/knowledge-original-service.ts`、`hooks/use-knowledge-original.ts`、`utils/knowledge-original{,-schema}.ts`。
- 新增既有`pages/dataset/knowledge-import/`内`original.tsx`、`original-fields.tsx`、`original-upload.tsx`、`original-confirmation.tsx`、`original-revisions.tsx`、`use-original-flow.ts`。
- 定向service／wire／恢复槽／页面／下载测试和合成夹具；入口仅改现有`knowledge-import/index.tsx`，词条仅`locales/zh.ts/en.ts`。不改共享UI、旧C1／0.2／0.3／0.4 DTO、网络拦截器、依赖或锁文件。

raw PUT仍走`registerNextServer→next-request`原认证。局部Axios transformRequest返回捕获的File/Blob，不能直接data:File被全局转换成普通对象；实施必须通过真实请求拦截器／假adapter与浏览器的原字节测试。浏览器整件下载首版为≤64MiB Blob，明确整件缓冲；服务端流式与Range单独验收。

## 4. 当前验证边界

离线结构／语义／摘要／端点和17保护哈希的实际结果在统一报告。候选没有业务API、模型、在线来源抓取、Go／Web实现或部署变化，未重跑418项旧Web／176项Go含子例。R4工程验收见行为定义第9节：有效文档和大文本两版SHA、原字节、超限／格式／凭据拒绝、崩溃／租约／同键、失败不推进头、授权Range、C1／0.4隔离、三档各3次性能和零外发。

阶段2仍剩R4实施→R5标准网页／订阅→R6显式移动／归档／保留→R7整体退出。真实资料与模型调用仍未放开；阶段3—6按完整链路继续，当前没有宣称全项目完成。

## 5. 候选提交后的并行准备

候选三件套已在`37574fb`推送，main仍38ce003；具体新契约确认已向真人提交，三件套未因后续准备改变。等待相关接入批准期间A继续准备24份合成原件（278,933,675字节），PDF／PPTX／DOCX及3／32／64MiB文本各两版和边界／格式反例，逐份SHA回读一致；样本仅存本机忽略目录，不上传应用或模型，不称业务接纳通过。

另在隔离adapter下执行真实`common-util→next-request→registerNextServer→Axios1.13.6`：7例通过，覆盖局部transformRequest保留File／Blob及原字节、反例直接data:File被转空JSON、既有JSON、signal和原生401处理；认证／UI依赖被隔离，真实HTTP0。这是实现前运输可行性证据，不是正式service、浏览器XHR／进度、MinIO／SQL或性能验收。[准备记录](stage2r4-preimplementation-preparation.json)保留边界。

消费会话的[R5网页／RSS只读调查](stage2r5-web-rss-preparation.md)已归档，关键路径由A核实；消费方随后接续R6原生生命周期能力盘点，仍无文件写入、真实移动／删除或新接口。主会话负责接续，未把同行完成当作全体停工条件。

主会话已实际登记active持续目标并回传消费方，覆盖剩余工程；当前没有标为完成。另有15项合成夹具一致性检查通过：PDF头／xref／EOF，Office ZIP CRC／必要部件／XML良构，全部大文本流式严格UTF-8及跨64KiB的emoji；它们只证明输入准备，不代替应用接纳或Office渲染。
