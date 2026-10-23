# AR-6 独立完整审查报告

本轮 skills：run-governed-product-workflow（verify）、go-naming。审查者：新的只读子 Agent /root/review_round6；没有复用前轮审查结论作为判断前提。

- 派发起点：2026-10-24T05:57:13.781+08:00；实际第一项读取：2026-10-24T05:57:37.985+08:00。
- 完成时刻见文末实际记录。
- 被审查交付：`4d63a7c6a010a822ffcb1642f0efc1c04ab46153`，已验证保存于 2026-10-23T21:57:04.251Z。
- subject_sha256（开始）：`2cc97fbf02e26cc2ebb0d30f3b223574086bd5b63966dccbc92e825204f7d7a4`。
- requirements_sha256：`a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403`。
- 结果：**完整覆盖，新增 1 项 P3 文档问题，不能计为干净轮次**。未发现新增代码权限、外发或来源事实丢失问题。

## 1. 原文及范围

独立从 review/before.json 的 request_snapshot 原字节重读，并复核原验证报告、当前技术方案第35—37章、根 AGENTS.md、README、会话接续说明及完整 A→B 接续文档。原文为：

> 阶段0方案通过。开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。暂不调用模型，完成验证后推送远端。

原文 UTF-8 SHA-256 独立重算与 requirements_sha256 相同。启动授权是用户对 a1a171c A阶段1审查回复“启动审查”；本次沿用同一范围，不重开此前拒绝的旧文档审查。

本轮完整范围：A 1A.1/1A.2 的领域/身份/C1-C3/原文修订/原生适配、权限与外发执行、实际构建运行证据、B可直接接续说明。B页面、B真实确认、整体纵向验收、阶段2—6、真实资料及真实模型性能均不在本轮实现承诺。推送待多轮审查完成，未把远端 Pending 认作缺陷或已完成。

## 2. 原话逐条与条件/例外覆盖

各条 quote 均为上面同一完整原句，不把下表的 reading 当成新的需求原文。

| 条目 | 原文拆解/条件 | 状态与正反边界 | 独立核对证据 |
| --- | --- | --- | --- |
| R1 | “阶段0方案通过” | 已确认此前设计获用户批准；本轮审查同意不替代 A Acceptance。 | 原话、35.8与36.1、两份报告 user_decision。 |
| R2 | “开始推进本机A的阶段1” | A的最小模型与适配已实现；B及整体阶段1仍未完成，明确区分。 | 35.6—35.9、源码改动、B说明第1/7/9节、原报告 scope。 |
| R3 | “先冻结双方共享的领域模型” | 使用原生Knowledgebase/Document/File/FileCommit身份；四KB扩展列、内部File所有者、三目录投影表及DTO，不另建后端。未来ProjectSnapshot/关系/Grant等明确尚未实现。 | entity/knowledge.go、dataset.go、file.go；DAO迁移及第35/37章。 |
| R4 | “节点身份” | 文件根/手工节点，逻辑ID与修订实例分开；同输入稳定，不同域/来源/修订/结构版本隔离；源25字节/两行CRLF不重写。 | 共享fixture、entity KnowledgeID、独立Node SHA计算、TestKnowledgeIdentityMatchesSharedFixture、source服务。 |
| R5 | “权限与外发规则” | 有效所有者可读；无权、跨域父级/修订、旧副本成员和目标库所有者拒绝；全部六种用途及排队/后台禁用；普通来源正例保留。标签不是Grant。 | permission/knowledge*.go、policy_ce/database_source、原生服务/后台调用路径、正反测试、脱敏HTTP证据。 |
| R6 | “接口契约” | C1/C3 1.0.0、14操作、认证/错误/分页/范围/ID/JSON规则与Go对应；C2/C4/C5/C6不冒充已实现。 | 完整OpenAPI的各操作/参数/请求/响应/schema/security/x-knowledge-frozen、handler/router、认证路由测试。 |
| R7 | “再完成最小底座适配” | 原生multipart→Document→显式快照→根/手工子节点→固定原文可读；不支持格式/大小/非法范围拒绝，正常UTF-8代码保留。 | document上传、knowledge_source、source_snapshot；原HTTP28组及当前79个Go输入一致。 |
| R8 | “给B整理可直接接续的交接说明” | 专用交接已包含构建/拉取/认证/错误/状态/归档/权限和归属；README尾部仍给废止S0及“仅方案”指令，部分实现，列 AR-25。 | 阶段1-A-B交接说明.md；README.md:108 与 :5/:7、35.8冲突。 |
| R9 | “暂不调用模型” | 本审查未发模型API；基线也未重新运行模型/真实源。策略预检不是唯一证据，执行服务/后台替身、模型配置/用量摘要与受控流程同时核查。没有独立供应商网络抓包。 | guard调用位置、生产wiring替身、当前全量日志、原生HTTP脱敏摘要、构建挂载检查。 |
| R10 | “完成验证后推送远端” | 工程验证已保存，当前审查仍发现问题，故尚未推送返修。原a1历史报告字节保留，不能把本轮审查或成功测试记为人验收。 | fixed-delivery、当前报告 publication.PendingReviewCompletion、git状态/HEAD；不替主Agent执行push。 |

R3—R7 的条件与反例进一步完整核对：

1. 注册只接受空私有有效数据集。非空、team、Connector、任务历史拒绝；同category幂等、换category冲突，非所有者/不存在隐藏404。已归档域的后续独立注册404。
2. 分组与节点是两种树：分组父级只在同域，节点父级只在同来源修订；最大16层、组件侧独立、手工logical_key同kind唯一。跨域/跨修订/过深/改不可变节点拒绝；合法父子与frontend/backend分别成立。
3. 来源导入只承诺≤2MiB UTF-8无NUL文本代码、256字符basename/512总路径；TSX/JSX保字节，PDF/PPT/二进制/非法路径拒绝；不把原生编辑器/Git外部修改说成自动同步。
4. 原文固定修订，UTF-8字节左闭右开，每次64KiB；合法范围/CRLF/末尾换行通过，切断UTF-8/越父级/超限拒绝；哈希或长度错误不返回受损字节。空根和末尾位置安全。
5. 相同输入快照幂等，当前修订CAS；失配409，历史不可追加，旧节点可读取旧字节。失败DB事务不发布source/node/native commit记录，但可留未引用对象，文档没有宣称跨系统原子事务。
6. 已绑定/历史/孤立/已标记来源目录按真实ID保护；改名不变身份；祖先移动/递归删除、普通同名认领/去重/版本/删除与全局FileID越界提交拒绝；合法普通目录与嵌套提交保留。
7. 当前/历史File-Document关系及目录raw子孙的来源所有者约束取交集，不通过team副本或队列扩大。删除注册域保留KB/Folder/Document/映射/Revision/对象，状态0只归档；普通未注册库保留物理删除。
8. CREATED恢复、Load/Execute、生产Embedding解析器、WikiDirty及消费器在依赖/正文工作前复核；普通任务正例继续。Debug context来自请求上传字节且无KB，不是通过受控来源身份读取的旁路。
9. 新接口异常体隐藏SQL/存储；严格字段大小写/重复键、JSON多值/null/必填/16KiB、非法查询编码/重复未知查询/空数字、范围和ID都核对。scope游标每次授权，不是凭据或事务快照。
10. 原生模型配置解析是本地 ResolveInfo/driver 处理，不等于 Encode/Chat/Rerank远程推理；新文档纠正因果，旧报告字节保持。

## 3. 前24项处置（逐项独立核对）

| ID | 处置 | 当前实现与正反证据 |
| --- | --- | --- |
| AR-01 | Fixed | Agent Search在模型构造/FilterDocuments/翻译前CheckDatasetModelUse；构造与增强计数均0，普通检索套件仍通过。 |
| AR-02 | Fixed | knowledgeObjectFields逐键精确校验；合法body通过，大小写别名、重复、未知键拒绝；handler及真实HTTP摘要一致。 |
| AR-03 | Fixed | basename≤256与总路径≤512分别检查，写节点前400；合法嵌套路径保留。 |
| AR-04 | Fixed | 写入关联32/64小写hex前验证，MySQL宽松排序不会被存成非法ID；跨域仍404。 |
| AR-05 | Fixed | native/CreateCommit与RecordSourceSnapshot同Folder行锁/头锁定读/时间递增；单元树保留与真实MySQL branched=false。 |
| AR-06 | Fixed | LinkToDatasets及后台convertFiles先检查目标与源；注册/版本化源拒绝，普通文件仍可走原生。 |
| AR-07 | Fixed | knowledgeBrowseDB在分页/子树搜索/祖先/大小计数前裁剪；成员0、所有者可见；MySQL递归查询证据0越权行。 |
| AR-08 | Fixed | ParseQuery错误和显式空整数拒绝，正常默认与有效值保留；handler与3组原生HTTP证据。 |
| AR-09 | Fixed | 注册持久KnowledgeFolderID，空目录改名继续绑定；本轮归档保留目录，原POC未注册绑定仍NULL。 |
| AR-10 | Fixed | UpdateDataset先授权、事务内复检再策略错误；无权与不存在相同隐藏结果，无模型解析。 |
| AR-11 | Fixed | OpenAPI当前C4检索/证据/复用、C5 MCP工具，与35.6和交接一致。 |
| AR-12 | Fixed | 无法映射旧名目录不猜域归属，按创建者隐藏；后来普通同名库不能认领。 |
| AR-13 | Fixed | 多历史Folder确定性主绑定；历史Source地址保留；重复迁移正例与双目录保留测试。 |
| AR-14 | Fixed | 受管来源/祖先树移动、删除在任何写入前拒绝；纯改名/普通目录保留。 |
| AR-15 | Fixed | 上传、初始化、版本解析、去重、删除的名称查询排除受管Folder ID，当前实现及回归覆盖。 |
| AR-16 | Fixed | 孤立判断不以普通数据集名认领；从未快照的旧目录与新同名库成员仍不可见。 |
| AR-17 | Fixed | current/history origins与原生资源权限取交集；旧副本精确文件/文档/版本/目标库读取及六用途拒绝，普通正例继续；删副本保留源。 |
| AR-18 | Fixed | native commit整批检租户、URL子树、操作权限，再写对象与版本；跨租户/同租户异树拒绝且0写，合法嵌套正例。 |
| AR-19 | Fixed | CREATED recovery、LoadFromIngestionTask、PipelineExecutor.Execute、Embedder resolver、Dirty Wiki/consumer均实际调用同一guard；旧上下文/副本拒绝，普通任务发布正例。 |
| AR-20 | Fixed | ordinary名称入口排除unclaimed；启动冻结内部所有者；实际MySQL回填/普通同名上传及删除自身目录测试。 |
| AR-21 | Fixed | .knowledgebase只有直接位于真实租户root才享公共例外；下级同名仍私有，原始标记不消失。 |
| AR-22 | Fixed | origins递归传播File.ParentID子孙并用于精确/列表/transfer/目标/模型，raw与嵌套旧副本拒绝，目录外普通文件保留。 |
| AR-23 | Fixed | DeleteDatasets注册域在当前KB行锁中重读并复核主体，status0归档且来源事实/字节保留；raw/uploaded/snapshotted三模式单元/真实HTTP前后拒绝与owner正例。 |
| AR-24 | Fixed | 独立追CreateDataset→verifyEmbeddingAvailability→ResolveInfo→newModelDriverForBaseURL，无Encode/Chat/Rerank；B/OpenAPI已纠正，原报告SHA未改。 |

## 4. 本轮发现

### AR-25（P3）README当前交付范围仍给出废止的阶段顺序和错误完成状态

- 位置：`README.md:108`（未标注为历史的“交付范围”最后一项）。
- 可复现条件：B在当前冻结HEAD读取该节，直接看到“下一步按第33章S0先冻结…当前仅完成方案”。
- 预期：当前入口统一指向第35章新阶段0—6，说明A1A.1/1A.2已实现/验证，B需核对共享契约并完成真实页面闭环；未完成项保留阶段2—6。
- 实际：该行指向已由35章替代的S0—S7，且把本次已经有Go底座及运行产物的状态退回“仅方案”。与README:5/:7、35.8、专用B说明第1/9节冲突。
- 影响：新的开发者/Agent按当前README尾部执行会重走旧开发顺序，当前接续说明存在两种相互冲突的“下一步”；不是历史报告保真所需。
- 修复边界：只纠正当前README这项状态/顺序，不改写历史技术方案章节和原验证报告；无需改业务代码或重新建设设施。
- evidence_refs：README.md:108、README.md:5、阶段1-A-B交接说明.md:9、本地第二大脑知识库技术方案.md第35.8/36.6节；全文manifest绑定本轮SHA。

## 5. 新反例与 NotAnIssue

本轮采用“归档后的原生普通行为与重叠请求”视角，与前轮仅检查副本删除保护区分；未预置干净结论。

### NA6-01：delete_all遇到已归档域是否阻塞普通库删除

源码初看服务Query未显式status条件；在隔离build-src新增临时单元测试，两分支分别是有效注册域与先执行原生DeleteDatasets归档后再delete_all。SQLite/MemoryStorage实测两者都`code=0/error=nil`，普通KB剩余0。下钻DAO.Query发现已固定筛status=1，因此不是缺陷，不要求重复加条件。

证据：build-src/review-round6-archive-deleteall.log、review-round6-state-counterexamples-final.log；测试原件另存round6-state-counterexamples.reference.txt。没有真实HTTP/DB写入。

### NA6-02：与归档重叠的幂等注册成功响应

在已注册域前置有效读取/授权后、事务锁定读前，通过局部GORM回调模拟status变0的顺序。初版强断言“锁定读看到0必须返回404”失败，观察是`archive_injected=true/returned_domain=true/error=nil/persisted_status=0`。这项观察本身真实，但初版期望比契约更强。

独立复核：请求与归档重叠，同category重放没有发布新内容、激活域、改变category、清除来源/模型限制；可线性化到前置有效读取。后续独立GetKnowledgeDomain仍404，CheckKnowledgeIngestion仍拒绝。最终更正测试2个顶层/4个含子用例均通过。group创建当前也只追加归档会保留的元数据；重叠完成可按先创建、后归档排序。source/node关键发布已有status锁定检查，未发现不可线性化的来源发布或权限扩张。

因此登记为NotAnIssue，不把缺少额外加固或初版测试的错误强断言算产品漏洞。原失败观察保留于review-round6-archive-registration.log；最终验证见review-round6-state-counterexamples-final.log。没有修改冻结源。

## 6. 工程、运行、证据真实性核对

本轮独立执行摘要保存在round-6-verification.json（含全部86项expected/actual），脚本round-6-verify.mjs仅读公开源/脱敏证据/构建日志及Docker显式状态字段，没有读取env、凭据、dump或原始HTTP记录。

| 对象 | 独立检查与实际结果 |
| --- | --- |
| 全manifest | 86文件逐个SHA重算；79个Go文件与隔离构建输入逐字节摘要相同；无不符。 |
| 原文绑定 | request_snapshot原字节SHA为a62c5bf…403，与本轮requirements相同。 |
| 原历史报告 | 当前阶段1-A底座验证报告.json与git show a1a171c原字节SHA相同，未改成当前证据。 |
| fixture | logical/node/source SHA独立计算均相同；25 UTF-8字节/两行，CRLF保留。 |
| 最新完整测试日志 | review-archive-final.log逐JSON Action计算：19包、3478顶层、5660含子用例pass、fail0、既有skip10；原生native/croptest两包ok；不是本轮重跑5660次。 |
| 10个skip | 独立保留名称于verification JSON，未把skip算pass；既有WordNet/环境/未支持pipeline等，不含新的知识域安全拒绝用例。 |
| 四模式启动 | 四份help日志有Usage及相应模式；实际Docker top每种模式1个进程。原生help退出1是既有约定，不称退出0。 |
| 运行二进制 | 实际容器内文件SHA=234dc7959f050cbcd733239bc26ab47a41d320cd50494f8159a32d4948e213d1，匹配冻结/构建；挂载RW=false、状态running、health=healthy。 |
| 原依赖/其他应用 | 对最初containers.before.json的7个其他容器按ID查当前，ID/镜像/source-destination-type挂载均相同。未按临时另项目容器名字推断；无重启/改写。 |
| 原生HTTP与MySQL | 核对28个原断言、33附加组（6+3+1+6+8+6+3）脱敏结果；检查archive/provenance脚本输入与断言，三归档模式保留来源事实/原始raw字节，真实并发CAS1成功1冲突，native/snapshot头不分叉，实际MySQL递归scope无越权行。没有重放真实HTTP写入。 |
| 旧POC/模型 | baseline与smoke的KB/doc/provider_config/native_usage摘要均相同，6文档/42块/0 active任务；不以这些摘要冒充供应商逐包网络审计。 |
| 秘密/构建隔离 | 独立git check-ignore验证真实env/本地Compose/私有目录被忽略，git ls-files上述项无输出；builder只挂build-src/tool-downloads，build-src无真实env、本地Compose或.local-data。实际秘密扫描报告为12值/5427 blobs/0命中；本审查没有读取秘密重扫，明确依据其脱敏报告。 |
| 变更归属 | product diff保持既有cmd/internal/web/docker/root文档布局；Go实现，无新Python/服务/基础设施/模型/真实来源导入；源根go.mod/go.sum/License等本轮未改。 |

证据范围限制：没有执行破坏性DB恢复；备份/Compose只验证存在与受控报告，不读取秘密或dump。没有独立模型网络抓包、真实模型质量/性能评测、B消费页面、临时MCP凭据或完整阶段1验收。它们是已明确排除/待验范围，不将其静默记为通过，也不把本轮变成用户已拒绝的其他审查。

## 7. 收口与完整性

- `coverage`: `Complete`。
- `unreviewed_items`: `[]`（本轮约定范围逐条已核对；明确排除/未运行范围见上）。
- `findings`: `[{"id":"AR-25","severity":"P3","summary":"README当前交付范围仍给出废止S0及仅方案指令","evidence_refs":["README.md:108","README.md:5","阶段1-A-B交接说明.md:9"]}]`。
- 前24项处置：全部按当前交付独立核对为Fixed；NA6-01/NA6-02明确NotAnIssue。
- model_api_calls：0；real_data_mutations：0；product_or_contract_edits：0；runtime_restart：0。
- 只在忽略的隔离构建副本增加临时SQLite/MemoryStorage测试及本轮独立证据文件；测试原件归档后移除，不进入最终构建清单。没有读取env/凭据/dump/原始HTTP。
- A用户Acceptance、B确认/页面与完整阶段1纵向验收仍Pending。本轮发现P3，须修复并重新冻结后继续完整审查；不能计为干净轮次。

完成时刻与最后manifest复算由随后的实际命令追加，不预填未来时间。

实际完成：2026-10-23T22:07:14.219Z。结束HEAD：4d63a7c6a010a822ffcb1642f0efc1c04ab46153。结束manifest SHA-256：`2cc97fbf02e26cc2ebb0d30f3b223574086bd5b63966dccbc92e825204f7d7a4`；开始/结束86项均相同，mismatches=[]；requirements摘要保持`a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403`。
