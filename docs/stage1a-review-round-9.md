# AR-9 独立完整只读审查

结论：完整范围审查确认 **AR-28、AR-29、AR-30 三项 P1 问题**，需要返修复验。本轮不是干净轮次，不以 AR-7 或前27项的处置标签预设结论。产品源码、共享契约、公开报告、运行服务及真实资料均未修改。

| 绑定项 | 实际值 |
| --- | --- |
| reviewer | /root/review_round9，新的只读审查子 Agent，未再委派 |
| 派发/审查开始 | 2026-10-23T22:44:40.516Z（round-9-start.json） |
| 完成 | 2026-10-23T22:58:35.470Z |
| delivery_commit | dce3efa27416969178180eec7cd891a893c18f20 |
| subject_sha256 | 80cd9f874cda025bdc276b52a14732eb22ab1529045f91c54ecc94f5e0d0391d |
| requirements_sha256 | a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403 |
| coverage / unreviewed_items | Complete / [] |
| model_api_calls / real_data_mutations | 0 / 0 |

使用 run-governed-product-workflow 的 SKILL.md、commands/verify.md，go-naming 的 SKILL.md、.agents/rules/named.md；先读根 AGENTS.md。沿用 before.json 中用户明确回复“启动审查”的同一次授权，只查 A阶段1、C1/C3和验证结果及其返修，不重开此前拒绝的文档审查。

## 1. 原始要求逐条覆盖

直接读取 `.local-data/stage1a-20261024/review-20261024/before.json` 的完整 request_snapshot 字符串，并以 UTF-8 原字节独立计算摘要。原话不是从上轮任务表重建：

> 阶段0方案通过。开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。暂不调用模型，完成验证后推送远端。

S1＝完整原句“阶段0方案通过。”；S2＝完整原句“开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。”；S3＝完整原句“暂不调用模型，完成验证后推送远端。”。下面同源多条引用保留上述完整句子及先后条件。

| 项 | 原句/独立拆出的要求 | 状态、证据与差距 |
| --- | --- | --- |
| R01 | S1：保留阶段0已通过的方案 | 已实现；技术方案35章的宏观链路、三种关系、A/B职责、六契约与阶段退出标准保留，36.1记录真实批准；本轮不改写旧验收。 |
| R02 | S2：推进本机A的阶段1 | 已实现；35.8的1A.1/1A.2、36章及B交接对应原生Go拥有者；没有把B页面或阶段2—6当成本轮完成项。 |
| R03 | S2：先冻结共享领域模型 | 已实现；Knowledgebase唯一域身份、四扩展字段、File内部私有owner、三目录投影表；entity/knowledge.go及加法迁移与C1/C3一致。 |
| R04 | S2：冻结节点身份 | 已实现；稳定Document.ID、FileCommitItem修订、JSON数组SHA的逻辑/实例身份；source-v1/manual-v1、fixture的25字节/正文SHA/两个ID独立重算一致。 |
| R05 | S2：冻结权限与外发规则 | **部分实现**；固定owner_only/catalogue/no_model/no_auto、来源所有者交集及原有入口拒绝成立；AR-28删除最后来源事实、AR-29破坏受管树、AR-30技能模块授权/模型入口未使用这些约束。 |
| R06 | S2：冻结接口契约 | 已实现；14操作、全部19个schema与126个引用逐一核对router/handler/DTO/service；1.0.0和C4/C5分工一致，未代替B确认。 |
| R07 | S2：冻结后完成最小底座适配 | **部分实现**；原生版本/上传/存储/权限及构建运行均有事实；发现的3个旁路使“授权/外发钩子闭环”尚未完成。 |
| R08 | S2：整理B可直接接续的交接 | 已实现；完整B交接第1—9节含消费顺序、opaque身份、范围/409/错误、固定策略、归档、构建装配及回退；B核对/页面仍Pending。 |
| R09 | S3：暂不调用模型 | 本轮操作已满足；仅SQLite、MemoryStorage、假DocEngine和进程内假EmbeddingDriver。真实API/真实模型/真实资料均0；进程内假driver的调用单独记数，不冒充真实网络证据。 |
| R10 | S3：完成验证 | **部分实现**；同版19包/构建/运行及73组已保存HTTP结果独立核对；本轮新安全反例实际失败，需修复后复验，原全绿测试不能替代这些失败。 |
| R11 | S3：验证后推送 | 已检查，当前保持PendingReviewCompletion；由主执行者在返修/审查完成后执行已授权推送，不把这个后续动作列为未检查或新增缺陷。 |

完整读取35—37章、README、会话接续当前入口/第16—17节、B交接、两份根JSON报告、C1/C3全部schema与fixture，并查完整冻结增量及其实际拥有路径。组织Group父级与Node来源父级分开，组件侧标签和language独立，关联不授权。B确认/页面/整阶段1以及A人的验收仍Pending；阶段2—6、原七体验问题、60题/AST/Graph/临时MCPKey/人格记忆不作为本轮已承诺实现。

## 2. 条件、正反例与例外

“既有”表示本轮读取源码/测试及对应已保存输出；“本轮”表示确实新增执行的隔离探针。没有重放真实HTTP写入。

| 条件/例外 | 成立输入→预期与实际 | 不成立/例外输入→预期与实际 | 证据 |
| --- | --- | --- | --- |
| 仅空私有库可注册 | 空me且无历史/Connector→成功；同category幂等 | team/有Doc、计数、Connector或任务历史→409；非owner→404 | dataset/knowledge.go:78，DomainRegistrationAndIsolation，smoke28组 |
| 私有与来源交集 | owner可读；源/普通副本六用途拒绝，原生team不能覆盖 | 非owner及副本owner不满足源owner→拒绝；无来源普通库继续 | permission/knowledge_sources.go、policy_ce.go，既有正反；新增删除/skills反例表明仍有旁路 |
| 标签不授予权限 | frontend/backend/shared/cli/non_code及language分开返回 | 分类变化要新修订；不存在通过标签授予Grant的路径 | entity/knowledge.go、CreateGroup/Snapshot/fixture |
| 文本/大小/路径 | UTF-8无NUL的.tsx/.jsx/空文本、2MiB以内、规范相对POSIX路径可用 | 非UTF-8/NUL/PDF/PPT/超限拒绝；路径512、末级256、../、绝对、反斜线限制 | utility与document/knowledge测试，knowledge_source.go:33/106 |
| 精确JSON与查询 | ≤16KiB单对象、精确字段、合法默认/整数、有效游标 | 重复/别名/未知/null/缺必填/多值、非法查询编码、重复参数、空整数→400或413 | handler/knowledge.go:55/110/145/167，handler测试及fixed/round2摘要 |
| 作用域分页 | 默认50/上限200；同域父项/同修订续页，每次重新授权 | 跨域/父项/修订cursor不匹配→400或404；非owner不因cursor放行 | knowledgePageQuery及所有4列表，GroupsPaginationAndParentScope/HTTP |
| 节点范围/身份 | 同域同修订、树含根≤16、UTF-8绝对半开字节、≤65536、CRLF保持 | 跨域/修订、越父级、切字符、17层、过读、改不可变身份/历史追加拒绝 | knowledge_source.go:279—452、NodeRangesIdentityAndIntegrity，前轮深度边界证据 |
| 来源CAS与历史 | 相同当前输入幂等；有效expected发布唯一current；旧节点仍回旧字节 | 陈旧expected→409；Doc失效/域归档后C1隐藏；不声称自动跟随File/Git | SnapshotRevisionCASAndNativeRead、MySQL CAS及commit-fixed.json |
| 失败恢复/对象边界 | DB失败目录/原生版本四表回滚；原当前版保持；重试可发布 | 对象先于DB写入，允许未引用对象残留；不假设跨系统原子性 | source_snapshot.go及SnapshotRollback/前轮StorageFailure证据 |
| 原生commit整批授权 | 同租户同持久子树的嵌套File允许，Folder头共享锁/时间递增 | 跨租户/异子树、受管来源及祖先delete在对象和版本写前拒绝 | file/knowledge_test.go全文件，commit-fixed无分叉 |
| 祖先历史不可扩权 | 当前子树/历史TreeState文件和parent/CommitItem均检查，受限祖先整体隐藏 | 普通已删File历史内容仍可读；无历史的受限子树changes也拒绝 | permission/knowledge_history.go全文件，handler/knowledge_history_test.go全文件，12组真实摘要 |
| 名称与目录身份 | KnowledgeFolderID及孤立owner标记持久；全部历史Folder保留 | 普通同名不认领/去重/删除受管树；仅真实租户根公共.knowledgebase例外 | dao/knowledge_folder/migration、file/document目录测试；FileService skills成立，独立SkillSpace不成立见AR-29 |
| 旧任务/来源不是模型凭据 | 恢复/Load/Execute/生产Embed/WikiDirty/consumer在依赖前拒绝 | 无受限来源的普通任务继续；DB不可用关闭 | task/knowledge_policy、consumer、service/knowledge测试；skills索引独立入口见AR-30 |
| 域归档保留事实 | raw/uploaded/snapshotted三模式保留KB/Folder/Doc/map/revision/object及副本约束 | 普通库物理删除；归档域不得重激活；不提供永久清除/恢复 | dataset/crud.go:398、archive测试及3组实际摘要；源Doc删除是另一路见AR-28 |
| 源Doc删除不得释放已知副本 | **本轮**无历史且在primary外的local/upload File，删除后仍应约束副本→实际两种均释放读权与6用途 | **本轮**File仍在primary或已有revision→删除后原限制继续，2组对照通过 | round-9-document-deletion.log，reference TestReviewRound9DocumentDeletionRetainsExplicitOrigins |
| 系统skills不得采用/删除受管树 | **本轮**受管root别名、受管同名子目录及缓存均须拒绝/另选普通目录→实际采用并删除raw/Folder或复用陈旧缓存 | **本轮**普通同名目录仍可清理→控制组成功 | round-9-skillspace.log、round-9-skillspace-paths.log |
| 技能消费必须保留来源交集/C3 | **本轮**copy-owner原生Folder Read被拒绝，技能Update/重索引也应先拒绝→实际改名成功，受限文本进入假Embed及假索引 | **本轮**移除注册来源事实的普通技能→重索引正常成功；后台FileService拒绝删除源树 | round-9-skill-reindex-frontmatter.log、round-9-background-confirmed.log |
| 零真实模型与安全运行 | 仅合成输入、假driver，真实API0；四运行模式SHA一致、5依赖原ID/image/mount | 没有独立抓包/审查者DB直连，不能凭摘要声称全网沙箱；旧POC不重跑 | round-9-verification.json、各新probe日志 |

同类解绑追踪：FileService.DeleteFiles先做整树授权/受管树门禁；File2Document.convertFiles在replace之前先检查目标和源；Connector Rebuild通过CheckDatasetAccess(OperationRun)在重建/删除前被C3阻止；Document status变更保留row/map，origins无status过滤。这些入口未确认同类释放问题。RemoveDocumentKeepFile与deleteDocumentFull共享硬删事实路径，修复AR-28须同时覆盖，不能只挡batch HTTP。注册域的原生完整清理留阶段2，可通过阶段1明确拒绝破坏性操作收口，无需新增永久删除引擎。

既有NotAnIssue保持证据边界：DAO.Query已筛status1，delete_all不额外选归档域；register幂等与archive重叠可线性化且不会重新激活，不强加未承诺隔离级别；普通成员注册他人私有库的例子不可达。无注册/历史/marker且已有普通Doc关联的目录不能按名称反推旧私有归属。普通原生版本delete写不存在File.status是已知基线限制，完整生命周期归阶段2；任意复制粘贴不在本轮DLP范围。

## 3. 全部14操作与schema

所有操作核对原生认证、版本头、request_id、真实HTTP成功/字符串错误code、隐藏404及500不泄露内部错误；成功外层schema和错误schema全部核对，126引用均解析。schemas包括Policy/Domain/Group/Source/Node/Content/Decision/Snapshot、五种请求、Error/Capabilities及四种Page，共19个。

| 方法/相对/api/v1路径 | 参数、输出与拥有者 |
| --- | --- |
| GET /knowledge/contract | 无query；Capabilities；handler:181，只声明C1/C3 |
| GET /knowledge/domains | cursor/limit→DomainPage；当前owner有效域 |
| PUT /datasets/{dataset_id}/knowledge/domain | RegisterDomain→Domain；空me注册及category冻结 |
| GET …/knowledge/domain | Domain；owner/状态/注册复核 |
| POST …/knowledge/groups | CreateGroup→Group/201；同域父项、kind/side/深度 |
| GET …/knowledge/groups | parent/cursor/limit→GroupPage；省略parent列根 |
| POST …/knowledge/sources | SnapshotSource→Snapshot；200幂等/201创建；CAS/原生commit |
| GET …/knowledge/sources | group/cursor/limit→SourcePage；省略group列全域current |
| GET …/knowledge/sources/{revision_id} | 固定Source；不跳当前版，失效Doc隐藏 |
| POST …/knowledge/nodes | CreateNode→Node/200或201；同版parent及不可变范围 |
| GET …/knowledge/nodes | 必填source_revision_id，parent/cursor/limit→NodePage |
| GET …/knowledge/nodes/{node_id} | 固定Node及所属来源状态 |
| GET …/knowledge/nodes/{node_id}/content | 可缩小绝对start/end→Content；UTF-8/SHA/64KiB |
| POST …/knowledge/policy/check | 六用途→Decision；始终false，仅预检 |

完整操作/参数/请求/响应记录保存在round-9-verification.json.operations及round-9-hashes-before/after.json。没有把schema文件路径宣称为HTTP下载路由。Go增量命名沿原包/拥有者、CamelCase与ID/SHA/JSON缩写规则，无新结构或命名改动要求。

## 4. 新确认问题

### AR-28（P1）：删除旧源Document移除最后的来源事实

位置：internal/service/document/document_crud.go:221、266、307、448、487；约束种子位于internal/permission/knowledge_sources.go:12。

复现：一个已注册域的旧未快照Document关联普通外部目录中的File，该File另有普通team库副本。域的primary Folder是另一个稳定ID；外部Parent非knowledgebase孤立目录、无marker、无revision。先用真实permission checker确认成员的File/目标库Read被拒绝，六用途模型guard拒绝。域owner经真实DocumentService.DeleteDocuments删除源Doc，返回deleted=1。当前版只硬删Doc和源map，不保存来源tombstone。

实际：sourceRows=0、copyMappings=1、fileRows=1；成员File/目标库Read均成功，embedding/rerank/generation/remote_parse/wiki/memory全部guard返回nil。local及knowledgebase两种File.SourceType都复现。对照：File在primary内或已有SourceRevision时，删除后限制仍在。此处已有明确来源事实被本次原生操作抹除，不是按名称猜归属。

预期：删除/解绑不得使已知私有副本重新获得权限或模型用途；阶段1可拒绝注册源的破坏性清理，完整保留协议留阶段2。不能只保护域DELETE而遗漏源Doc的两个底层清理入口。

证据：round9-document-counterexamples.reference.txt；round-9-document-deletion.log。执行bash build.sh --test -json -run TestReviewRound9DocumentDeletionRetainsExplicitOrigins ./internal/service/document；两个安全反例失败、两组对照通过。

### AR-29（P1）：独立SkillSpace名称/缓存路径仍破坏受管树

位置：internal/service/skill_space.go:79（84/97缓存直接返回）、109—124名称采用、210—219同名清理、532—572自行递归硬删。FileService.initSkillsFolder的AR-27修复未被该服务使用。

复现：a）普通skills根下已有同名受管domain Folder和raw子文件；b）root下已有受管Folder名为skills，raw位于其同名子目录；均保留旧raw→普通team目标Doc映射。用真实CreateSpace创建Collision。另独立热缓存旧skills根后建立受限来源，再次获取根。

实际：两种CreateSpace都code0，rawRows=0、oldFolderRows=0；旧target模型guard由拒绝变nil；a）的域绑定因此悬空。缓存获取仍返回受管旧ID且无错误。普通目录同名清理控制组成功。该操作经另一套DAO递归绕过FileService整树门禁。

预期：当前来源约束在采用、缓存命中和删除前都有效；受管目录/祖先不能因系统名称、空间名称或陈旧缓存被接管/清理。复用原FileService普通目录选择和树保护；不再维护第二套递归删除。

证据：round9-skillspace-counterexamples.reference.txt；round-9-skillspace.log的managed_child/managed_root_alias两失败及ordinary_control通过；round-9-skillspace-paths.log的CacheRevalidatesOrigin失败。

### AR-30（P1）：SkillSpace授权和重索引绕过已知来源所有者及C3

位置：internal/service/skill_space_permission.go:27；internal/permission/database_source.go:389—403、skillSpaceResource:1164；internal/service/skill_space.go:335/379；internal/service/skill_indexer.go:435、488、528、616、700、796及模型helper949/977/1015。

复现：合法旧映射把copy-owner租户中空间根的anchor File关联到source-owner的注册私有域，来源约束因此传播整个空间树。原生Folder检查已拒绝copy-owner Read。仍保留旧SkillSpace.FolderID和正常SKILL.md版本树。无真实凭据：注册OpenAI目录的driver在测试进程内替换为捕获假driver，所有Embedding向量由Go常量生成；MemoryStorage及假DocEngine无外部连接。

实际：SkillSpace.Update返回code0并把已无权原生Folder的name写成Changed private folder。新隔离实例调用真实SkillIndexer.ReindexAll，返回indexed_count=1/failed_count=0，4次假Embed调用，private_payload_seen=true、indexed_documents=1、indexed_body_seen=true。默认字段配置已启用name/description；正常SKILL.md frontmatter中的合成受限标记实际到达假driver，正文进入假索引。只做技能空间租户授权、再按名称选树及直接Storage.Get，绕过File/来源交集和固定禁模型策略。

预期：在模型构造、配置/版本修改和内容读取前，技能空间及实际所读树必须受已知来源所有者与C3约束；持久FolderID不能被同名树替换。单独加FileService删除门禁不能修复读取/模型旁路。

正例：同fixture移除注册域约束后，普通技能ReindexAll仍成功、假driver4次/假索引1条。后台删除另单独复验：真实asyncDeleteSpace经FileService拒绝两次，5个受限ID和来源约束全部保留；它不是第二条物理删除旁路。空间status仍按上游后台逻辑变0，未把它误报为来源事实已删。

证据：round9-skillspace-counterexamples.reference.txt及round9-skillspace-counterexamples-final.reference.txt；round-9-skillspace-paths.log的Update反例；round-9-skill-reindex-frontmatter.log的完整生产编排与普通控制组；round-9-background-confirmed.log。

## 5. 前27项处置独立复核

下面Fixed指旧问题的原复现场景在本固定交付已修复，不覆盖本轮发现的新入口；相关开放项明确列出，不能据此宣布全局闭环。

| 原ID | 处置 | 本轮核对的实现及正反证据 |
| --- | --- | --- |
| AR-01 | Fixed | Agent adapter在模型构造/增强前guard，替身构造/增强0；普通检索保留；skills新入口归AR-30。 |
| AR-02 | Fixed | 精确字段与重复键检查；合法对象200，别名/重复/未知400。 |
| AR-03 | Fixed | 路径512/末级256分别验证；长合法目录支持，过长basename先400。 |
| AR-04 | Fixed | 三写请求关联ID小写32/64hex；正常成功，大写拒绝，不借MySQL宽松匹配写入。 |
| AR-05 | Fixed | snapshot/native共用Folder锁、锁定读头及递增时间；实际MySQL单链、树保留。 |
| AR-06 | Fixed | Link前台/convert后台同时检查目标和源；受限0映射，普通正例。 |
| AR-07 | Fixed | browse在分页/关键词/路径/聚合前过滤；成员0、owner保留，实际MySQL越权行0。 |
| AR-08 | Fixed | ParseQuery错误/空整数400；正常默认/合法整数保留。 |
| AR-09 | Fixed | KnowledgeFolderID注册事务绑定与唯一索引，改名不变；空Folder仍隐藏。 |
| AR-10 | Fixed | Dataset.Update先授权后策略错误，锁内复检；无权/不存在同结果。 |
| AR-11 | Fixed | C4检索/证据/复用、C5 MCP授权工具与35.6一致，仅延后声明。 |
| AR-12 | Fixed | 未认领旧空Folder按创建者隐藏；同名普通库不认领。 |
| AR-13 | Fixed | 多历史Folder确定性选主，迁移可重跑，历史原地址保留。 |
| AR-14 | Fixed | FileService move/delete在写前查整树；自身/祖先受管拒绝，普通/改名保留。独立技能清理归AR-29。 |
| AR-15 | Fixed | 普通上传/InitDatasetDocs/版本resolver/去重/Dataset删除排除受管Folder；技能独立名称入口归AR-29/30。 |
| AR-16 | Fixed | 无名称认领，未快照孤立目录持续私有。 |
| AR-17 | Fixed，存在新旁路 | File/Dataset当前/历史owner交集、scope与六用途成立，普通副本删除保留源；源Doc删除遗忘归AR-28、skills消费归AR-30。 |
| AR-18 | Fixed | commit整批租户/持久子树/操作校验先于对象；嵌套正例与整批0对象/版本。 |
| AR-19 | Fixed | 恢复/Load/Execute/生产Embed/WikiDirty/consumer依赖前拒绝，普通任务继续；技能索引独立入口归AR-30。 |
| AR-20 | Fixed | 普通上传不接管孤立目录、内部owner持久；普通自身目录清理可用。 |
| AR-21 | Fixed | .knowledgebase例外核对真实self-parent租户根；同名下级仍拒绝。 |
| AR-22 | Fixed，存在新旁路 | origins递归raw子孙，读/browse/link/目标/六用途一致；破坏最后种子/物理树后的释放归AR-28/29。 |
| AR-23 | Fixed | 注册域DELETE锁内复核后逻辑归档保事实，三模式正反通过；不同的源Doc清理归AR-28。 |
| AR-24 | Fixed | CreateDataset本地ResolveInfo/driver解析无Encode；当前交接/OpenAPI正确，原a1报告原字节保留。 |
| AR-25 | Fixed | README当前顺序35章/A已实现/B核对和真实页面待完成，历史章节不改写。 |
| AR-26 | Fixed | 新history guard覆盖当前子树、历史parent/file/item及单File所属Folder；9版本面/无历史changes/普通已删File正例，已保存12组实际验证。 |
| AR-27 | Fixed，存在独立同类入口 | FileService.initSkillsFolder首次排除受管/孤立/root/非Folder，候选整树检查；普通去重保留。SkillSpace不共用该实现，归AR-29。 |

## 6. 工程证据、真实性与不变性

全89文件开始/结束逐一SHA，不抽样；82 Go逐一与build-src比较，全部相同。开始核对记录22:45:52.459Z，首次结束核对22:55:35.490Z，临时文件清理后的最终核对2026-10-23T22:58:10.394Z；subject前后均80cd9f…d0391d，完整request_snapshot的UTF-8摘要前后均a62c5b…db403，before.json文件摘要前后均837d5b…6f87c。HEAD始终dce3efa，git status为空；完整逐文件对照保存在round-9-hashes-before.json与round-9-hashes-after.json。原阶段1-A底座验证报告与a1a171c字节一致。

独立解析当前review-history-guard-final.log：19包、3481顶层/5668含子用例pass、0fail、10个既有skip（名称完整保存在round-9-verification.json）。native和croptest两包ok，原build.sh成功构建CLI/server；这不是复用720输入的老结果，也未谎称本审查重跑19包。四个help日志各有真实模式标志和Usage，原生help exit1约定保留。

本轮直接读取安全Docker字段、只读挂载文件及4个/proc/*/exe摘要：应用running/healthy；binary只读，SHA=7c241a535264887fc9507e7a700fe9070d8eb7663de5b2020f9a23d98d0531d7；api/admin/ingestor/syncer各1，4个进程全部匹配。5个POC依赖ID/image/mount与原baseline相同。两外部AICS容器此前已改变，当前报告如实保留5+2范围；没有归因或操作它们。

脱敏HTTP证据：原28组；fixed6+query3+emptyRename1+folder6+provenance8+directoryOrigin6+archive3=33组；history/skills再12组，共45附加、73总组。读取摘要及对应公开断言，未执行native-api.mjs或读取私有原始HTTP/错误。真实MySQL并发commit-fixed为无分叉单链，scope查询越权行0。合成账号已移除，域逻辑归档保留历史对象，不声称测试数据物理清空。

current-safe-fingerprint来自主执行者既有只读DB通道；本审查只读安全摘要，四摘要与baseline/smoke一致，6份文档/42块/active0。publication-verification在22:44:17.854Z记录12秘密值、index+history5446blob、0命中、6保护文件与10导入根文件不变，私有资料/向量/二进制未入暂存。没有读取密钥，没有自称独立DB连接或全网抓包。

本轮新的运行证据均通过bash build.sh --test。第一次误用构建容器默认/ragflow目录只得到build.sh不存在，已保存round-9-initial-workdir-error.txt；改用/work后才执行真实测试。首次Reindex fixture是纯正文，默认Content字段不进入Embedding，但已进入索引且4次假调用；该事实保留。改用正常SKILL.md frontmatter后private_payload_seen=true，普通对照成功。背景首次探针在测试日志设施未初始化的条件下走recover，源树保留；关闭该测试日志设施后单独重跑，真实两次FileService拒绝后源树仍5项、status0，见confirmed日志，不用早先recover替代正常路径证据。

新测试中的安全断言失败是产品反例，不是通过；普通/primary/history控制通过分别记录。实际进程内假Embed总12次（初始受限4、frontmatter受限4、普通4），没有网络或真实模型推理。所有临时_test.go结束后移出build-src，保存为reference.txt；不留能混入下一次构建的测试。仅忽略目录内证据写入。

## 7. 结论边界

完整约定范围已经检查，当前无未决候选或未检查项；3项确认问题尚未修复。coverage=Complete不等于实现全部通过；本轮结果为FindingsRequireRepair。A人的验收、B确认/页面/整体阶段1及后续推送保持各自真实状态，主执行者完成返修复验后再进入下一轮。未宣称绝对无缺陷、百万级性能、原七体验问题或阶段2—6完成。

最终归档：2026-10-23T22:58:35.470Z。第三次全量绑定复核见round-9-hashes-final.json；新probe的逐例实际时间/结果/输出/SHA见round-9-probes.json。两处临时_test.go均已移出，跟踪工作区仍为空。
