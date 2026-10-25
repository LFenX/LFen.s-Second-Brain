# S2-R4 文档原件接续

日期：2026-10-26；文档v0.42。**C2 0.5.0仍是待确认候选，不是原件功能交付**。运行main仍为R3的`38ce003`；候选在`codex/stage2r4-original-candidate`，MinIO私有准备00bed83、格式／暂存8048ede及当前快照核心准备在后续准备分支，当前检出`codex/stage2r4-original-preparation`；不混为业务接口已实现。

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

[R6生命周期前置盘点](stage2r6-lifecycle-preparation.md)已回传并由A核实关键路径：现成域归档、来源级状态缺口、原生物理move与逻辑位置区别、历史revision复用和固定清单保留。只形成事实及建议，没有新接口、迁移或实际移动／删除。

## 6. v0.40包私有流式存储准备

A继续实现`internal/storage/minio_stream.go`及测试，沿用现有MinIO客户端与桶／前缀解析；新增put/open均包私有，私有reader只实现已有io.ReadCloser。不改Storage接口、原Put/Get、共享领域实体、数据库、路由／服务、Web或运行应用。实际代码尚无业务调用者，等待0.5确认后的正式适配，不把它说成上传闭环。

`bash build.sh --test -json -run '^TestMinioStream' ./internal/storage`通过7顶层／28含子例，LLD22.1.8、实际SDK7.0.99＋纯合成Transport；无真实HTTP／MinIO／数据库或模型。0／3／64MiB各一次，64MiB最大源Read64KiB、TotalAlloc差额77,912字节（3MiB相同），测试阈值<4MiB；不是峰值RSS／端到端性能。固定元信息／ETag／字节范围、缺省桶创建、超界／短流／故障／body关闭／截断／取消有定向证据。[完整报告](stage2r4-stream-preparation-verification.json)包含版本／范围和未验项。

上传原语信任调用方预先测量／计算的SHA，本身不做格式接纳／授权／发布；读取只验证元信息，部分范围不重算全件SHA。业务后续须完成真实字节验证、暂存与不可变命名、SQL／配额／租约／确认／CAS、原生旁路和Web／真实MinIO验证；旧对象不默认具有新摘要元信息。旧17资源及37574fb候选三件套不改，正式应用未加载本批代码。

消费方[R7退出追踪](stage2r7-exit-readiness.md)已保存。真实AICS／文档／网页退出样本仍需另选范围；模型0下可继续内部准备和获批接口后的合成闭环，不以历史Accepted或本次unit覆盖全阶段验收。

## 7. v0.41格式与暂存内部准备

新增`knowledge_original_prepare.go`、`knowledge_original_office.go`及测试，全部包私有。沿用安全os.Root、文本／凭据规则和流式探针；生成0700目录／0600文件，64KiB缓冲保存已声明字节，超声明的探测字节不写文件，同时计算SHA与兼容原生的XXH128。有限PDF头尾、Office真实中央目录预检／安全成员／预算／必要XML／根关系及CRC检查，失败仅清本次生成临时项，根被替换也不跟随新路径。无共享DTO／领域模型／路由／SQL／Web或正式运行接线。

`build.sh --test -json -run '^TestKnowledge(OriginalPreparation|SourcePreparation)' ./internal/service/dataset`通过13顶层／76含子例：新原件6／57＋旧采集7／19；另在一个测试内有90组输入／分块对标准UTF-8等oracle。首轮近64MiB密集中文累计分配50,562,560字节，定位旧私有探针拼接会复制整块，修复后268,984字节；3MiB由2,441,112降至81,560。日志保留失败与修复，新增场景后的最终运行绑定源码；没有多轮审查。[报告](stage2r4-original-preparation-verification.json)给出边界。

这些是参数化私有原语，测试用2MiB中央目录／256层XML等预算不是公开默认值；小包原型不支持ZIP64／多卷／非UTF-8 XML，不做完整Office schema、渲染／病毒扫描或正文解析。真实ENOSPC／Sync故障未注入，写错／短写由实际复制原语的合成Writer覆盖；整体路径覆盖读错／取消／失败清理／根替换。正式适配前须收敛支持范围与预算，不静默改37574fb或声称0.5冻结。

## 8. v0.42原生快照元数据核心

`RecordSourceSnapshot`签名／调用者保持，仍写SHA地址对象；后续原生树／前驱／提交和提交项记录提取为同文件的唯一私有`recordSourceSnapshot`。它接摘要和长度，不接正文、不I/O对象、不更新File或知识修订、不执行授权。未来流式确认经具体批准后才能调用正式适配，不能把无Storage元数据单元当作对象存在证明。

11顶层／27含子例通过：新私有核心4／16、既有file路径2／6、dataset消费者5／5。覆盖同SHA不同身份、0／3MiB／64MiB元数据、固定旧树／兄弟项／未来时钟前驱、源行不变，非法摘要／长度、SQL写错／损坏及null树／缺失folder／取消不推进头，以及C1和手工／目录／批次回归。[报告](stage2r4-snapshot-preparation-verification.json)绑定最终源码；SQLite不证明MySQL并发，既有原生树仍整件JSON、空间随树规模增长。正式应用／0.5／Web未因此接线。
