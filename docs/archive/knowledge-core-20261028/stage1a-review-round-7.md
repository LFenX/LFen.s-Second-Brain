> 历史归档（2026-10-28）：仅保留旧阶段事实，不作为正式开发指引，不恢复旧批准、排程或写权。当前资料见[参考基线](../../knowledge-core-reference-authority.md)。

# AR-7 独立完整对抗性审查

- reviewer：`/root/review_round7`，本轮新的只读独立执行者；未启动其他审查者。
- started_at：`2026-10-23T22:08:12.336Z`（实际轮次派发记录）；首次读取根AGENTS／技能的工具时间为`2026-10-23T22:08:47Z`。
- completed_at：`2026-10-23T22:16:55.760Z`，UTC；本地时区Asia/Shanghai。
- delivery_commit：`720b88df692af0d77e352c658eaede308af7f89d`。
- subject_sha256：`8415e6faa2f995a74bf16b264c98598999246c14e549b395e13ad337914bb809`。
- requirements_sha256：`a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403`。
- 固定交付已验证时间：`2026-10-23T22:07:59.917Z`。本轮开始晚于该时间。
- 使用技能：`run-governed-product-workflow/SKILL.md`及`commands/verify.md`、`go-naming/SKILL.md`及其`named.md`；先读取根`AGENTS.md`。
- coverage：`Complete`；findings：`[]`；unreviewed_items：`[]`。本轮无确认缺陷、待决疑点或未处置产品问题；是否达到连续两轮由主Agent合并各轮事实计算，本报告不单独宣称整个审查已通过。

## 1. 原始基准、授权与审查边界

独立重读`before.json.request_snapshot`、实际审查同意、技术方案35—37章、README、B交接、当前会话接续入口、原v0.19报告与当前v0.22报告。没有用前轮修复清单代替原话，也没有把前轮结论当作本轮无问题的前提。

request_snapshot完整原文：

> 阶段0方案通过。开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。暂不调用模型，完成验证后推送远端。

逐句来源：S1=`阶段0方案通过。`；S2=`开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。`；S3=`暂不调用模型，完成验证后推送远端。`。后文每一拆条均引用相应完整原句及其限定词；完整原话UTF-8摘要为上述requirements_sha256。真实澄清记录为Skipped／Settled，无其他本轮用户问答可替代或追加；未编造A验收或B确认。

审查授权原话为`启动审查`，实际记录`2026-10-23T19:54:46.608Z`；范围是A阶段1代码、C1／C3、报告及同范围返修复验。此前拒绝的旧文档审查、B页面与阶段2—6没有重启。固定subject有86个文件（79个Go输入、机器契约与fixture、原报告及当前入口／交接文档）；追加式审查报告与轮次文件不参与产品subject。

本轮只读取公开源码、允许的脱敏结果和安全Docker字段。唯一执行性写入是忽略目录中的4组SQLite／MemoryStorage临时反例及审查证据；临时测试已移出build-src，未进入产品树。未读取env内容、access_token、Key值、DB dump、私有HTTP明细或error文件；未调用模型、导入真实内容、修改运行数据库、运行服务、共享契约、公开文档或配置。

## 2. 按原文逐条核对

| 编号／完整原句来源 | 拆出的要求和条件 | 当前状态／证据／未完成差异 |
| --- | --- | --- |
| R7-01／S1 | 阶段0已经通过，阶段1以已批准的35章宏观设计为基础 | 已体现。36.1及原报告保留bd42c40与真实批准原话；不把此批准替代新A验收。 |
| R7-02／S2 | 开始推进的是本机A的阶段1 | 已实现。1A.1／1A.2映射到Go原包，保留cmd/internal/web/docker；未新建后端或目录体系。79个Go变更逐个检查，现有路径只做领域、原生适配与必要本地测试准备。 |
| R7-03／S2 | 先冻结双方共享领域模型 | 已实现A交付。Knowledgebase一套域身份、三个目录投影表；组织树与来源结构树分开，组件侧与language分开。实体、迁移、DTO、OpenAPI及fixture一致。B的共同核对尚Pending，文档明确未代为确认。 |
| R7-04／S2 | 先冻结节点身份 | 已实现。logical为域／Document／kind／logical_key的确定性SHA，instance另含FileCommitItem修订及structure_version；客户端opaque。fixture独立重算均一致。新反例证明分类往返仍复用固定历史ID，单一current且节点内容不可原地改写。 |
| R7-05／S2 | 先冻结权限规则 | 已实现。CreatedBy owner_only与原生授权取交集，标签不授权；当前／历史／目录子孙来源同时约束精确读取、列表、计数、旧普通副本。跨域父级、修订和节点隐藏404。代码／正反测试及脱敏MySQL作用域结果对应。 |
| R7-06／S2 | 先冻结外发规则 | 已实现v1固定禁止。六类模型用途、自动入库、原生Use／Run、Chat／Agent增强、检索Embedding／rerank、排队恢复、Load／Execute、生产Embedding、WikiDirty及consumer均在依赖前拒绝；普通无来源链路径由本地替身正例保留。没有配置放行接口。 |
| R7-07／S2 | 先冻结接口契约 | 已实现A基线C1／C3 1.0.0。完整核查14操作、所有schema、请求必填／空值／错误／范围／游标及路由认证；无失效schema引用。C2／C4—C6明确未实现，C4／C5职责与35章一致。 |
| R7-08／S2 | 冻结后完成最小底座适配 | 已实现。加法迁移、原生Dataset multipart、Document稳定来源、FileCommit／Item及原存储、不可变范围、事务发布／CAS／回滚、当前二进制加载与回退材料齐备。PDF／PPT、AST／关系、自动同步与全生命周期未冒充已完成。 |
| R7-09／S2 | 给B可直接接续的交接说明 | 已实现A交付。B说明包含同main／短分支、所有14操作、最小创建顺序、opaque身份／字节坐标、实际策略、原生错误边界、构建加载／回退及真实纵向验收；不传Key或登录令牌。README当前入口指35章与A已实现，不再让B按旧S0重启。B实现及整体页面闭环尚Pending。 |
| R7-10／S3 | 暂不调用模型 | 已遵守。本轮四组反例只用本地替身，未import需要凭据的native-api模块，未执行真实HTTP工作流；模型API调用0。历史工程替身及脱敏用量证据可核对；主Agent新提取的当前安全摘要亦一致。不据此宣称有独立网络抓包。 |
| R7-11／S3 | 完成验证后才推送 | 验证已完成并保存；当前推送未执行，仍PendingReviewCompletion，由主Agent在授权审查完成后执行。不存在把测试成功当用户验收或提前发布的记录。此处是已检查的待后续动作，不是未检查项或本轮新增缺陷。 |

明确未实现／部分实现：推送尚未执行；B共同核对、1B.1／1B.2、整体阶段1纵向验收尚未完成；A的用户Acceptance仍Pending。它们均有明确状态与后续负责人，本轮不代为执行，也不作为A产品缺陷。阶段2—6及60题／规模／恢复演练继续保留未实施边界。

## 3. 条件、正反与例外覆盖

下面将完整原句S2、S3在已冻结C1／C3及35章阶段1承诺中的限定条件逐项展开。实际执行结果复用同一79个Go输入的原始测试／脱敏HTTP证据；新增执行单列，未把源码阅读冒充新HTTP重跑。

| 条件／例外 | 条件成立输入→预期／实际 | 条件不成立或例外→预期／实际 | 证据 |
| --- | --- | --- | --- |
| 新域须空私有，且无Connector／任务历史；既注册同category可重放 | 空me库注册→成功且策略固定；同category重放→同域 | team／有文档块／Connector／历史→409；变category→409；归档后独立重登→404 | knowledge.go；TestKnowledgeDomainRegistrationAndIsolation；smoke28；archive3 |
| 组织与来源父子同域、节点还须同修订，含根深度≤16 | 同域父子及第16层→成功；空文件[0,0)根／叶可读且行1 | 跨域或错修订→404；第17层→400；历史修订追加→409 | knowledge_source.go、groups服务；新反例4；原节点／分组回归 |
| 原始来源为≤2MiB UTF-8无NUL允许代码文本 | .tsx／.jsx及CRLF／多字节原文→字节保留，无parse | PPT／PDF、不合法UTF-8、NUL或超限→拒绝，不排队 | document/knowledge_test.go、utility测试、source限制测试；HTTP上传快照 |
| 路径正规化，长度512／末级256；语言和组件标签不混为授权 | 正常目录路径、独立frontend／backend／language→保留 | ../、绝对、反斜线／冒号、非正规路径、末级超限→400；改标签不放开任何读权或模型用途 | knowledgeRelativePath、isKnowledgeID及分类回归；contract-fixed／id-fixed |
| JSON为单对象≤16KiB，键精确且不重复 | 合法必填、可空parent_id／group_id→成功 | 别名／重复／未知键、缺必填、不支持null、尾部对象／超限→400／413 | handler完整代码及AmbiguousBodies；真实严格协议汇总 |
| Query只支持一次允许参数，整数不可显式空 | 正常缺省、有效limit及范围→正常 | malformed编码、分号、重复／未知、limit=、start/end空或非法→400 | handler完整代码；round2-fixed3组及对应测试 |
| UTF-8来源绝对字节左闭右开，须位于节点且每次≤64KiB | CRLF原文、合法多字节边界、65,536字节与空范围→精确返回 | 切开UTF-8、越父范围、反向、65,537字节→400／413；存储SHA／长度异常→409 | TestKnowledgeNodeRangesIdentityAndIntegrity、SourceLimits；新反例4；smoke原文 |
| 相同当前输入幂等；更改须CAS，历史不可原地重写 | 相同输入→同修订；匹配expected→新实例，旧内容仍读；分类A→B→A复用先前不可变修订 | 过时expected或同节点ID改名／父级／范围→409；不同Document不承诺共logical | CAS及旧版单元／MySQL1胜1冲突；新反例1 |
| Folder提交头须线性追加，失败不能发布目录／版本 | 原生→快照→原生均链到最新头且保留TreeState；正确重试可恢复 | 损坏前驱TreeState→事务回滚，4张记录仍各1，修复后重试成功 | source_snapshot.go／file_commit.go；commit-fixed真实MySQL；新反例2 |
| 对象先写而DB回滚，可留下未引用对象 | 成功发布→原文SHA回读 | DB失败→零目录／版本发布；无跨MySQL＋对象存储原子承诺；不把残留blob当新缺陷 | RollbackDoesNotPublishCatalogue；新反例2；交接6节 |
| 所有者与原生权限取交集，File／Document／历史来源不能扩大授权 | 来源所有者在原生允许范围内读，普通无来源链成员路径保留 | 非所有者、目标team库／其所有者、历史源、raw子孙→拒绝；列表在count／分页前裁去 | permission两套origin、communityPolicy、databaseSource；权限全回归；provenance8、directory6、mysql-scope |
| 原生link、版本提交、move／delete不得绕过 | 普通文件link、同目录／嵌套commit与纯改名→保留 | 注册域目标／受限源link→拒绝；异租户／异子树整批commit→0写；受管树及祖先move／delete→拒绝 | 原生服务／handler完整受影响路径；File回归、folder6、provenance8 |
| 上传Folder按ID，普通名称不能认领受限／孤立目录 | 改名后同ID；多历史目录确定性选新主目录，旧地址保留 | 同名普通库上传／初始化／版本／去重／删除不采用受管目录；没有明确事实的普通旧目录不凭名称反推 | EnsureKnowledgeFolder、KnowledgeFolderIDs、迁移全回归；empty-rename／folder6／directory6 |
| 旧孤立目录固定owner，公共容器例外仅真实tenant root下 | 真实根的公共.knowledgebase保持原生浏览 | 同名下级／改名／后增普通映射不能去掉内部owner；缺创建者保守回退tenant不新增读权 | UnclaimedKnowledgeFolderIDs、ProtectUnclaimed；迁移及保留名测试、directory6 |
| C3必须在所有模型／自动处理依赖前执行 | 普通无来源链任务／本地替身路径继续；无模型模型预检返回固定allowed=false | 六用途／旧副本、CREATED恢复、重投Load／Execute、生产Embedding／Wiki／consumer拒绝且依赖／发布0 | service、agent/tool、task、consumer新增知识策略测试；源码逐入口检查 |
| 注册域删除为逻辑归档，保留来源限制；普通库仍原生删除 | raw／uploaded／snapshot3模式归档保留KB／Folder／Doc／映射／版本／对象；旧副本仍限；普通无来源库删除成功 | 归档域新目录读取／重登404，无恢复／物理清除承诺；无权删除锁内复检拒绝 | crud.go及archive全测试；archive-live-fixed3；新反例3 |
| 本地模型配置解析与远程模型推理有别 | CreateDataset→verifyEmbeddingAvailability→ResolveInfo仅解析配置／驱动 | 注册域或受限副本更新模型字段在该解析前拒绝；不以本地检查存在推断远程请求 | embedding.go、model_factory.go；当前OpenAPI／B说明；AR24证据 |
| 测试和运行证据绑定固定版本，历史报告不可改写 | 79输入逐个与build-src一致，四实际exe同SHA，历史报告原字节一致 | 旧a1测试数字与binary不能被称当前代码重跑；当前文档明确复用同输入结果；没有独立网络抓包 | round-7-verification.json、原始log与help、当前报告、manifest前后 |

保留的例外还包括：无注册／历史／内部owner标记而已有普通Document映射的目录不能被反推为旧私有域；任意手工复制粘贴DLP、raw的C1导入、原生普通版本delete的既有缺列问题、完整恢复／物理清理都不在本轮已实现承诺内。核对当前代码与明确范围后，这些没有被加固偏好转成新增缺陷。

## 4. 本轮实际新增反例及结果

命令：在无真实Key／真实资料挂载的现有builder中，从`/work`执行`bash build.sh --test -json -run TestReviewRound7 ./internal/service/dataset`。先gofmt临时测试；未直接执行go test。日志为`round-7-probes.log`，参考源码为`round7-counterexamples.reference.txt`。4个顶层测试全pass，失败0；build.sh附带native／croptest两包也ok。执行时间在日志中逐项可查（`2026-10-23T22:11:37Z`）。临时测试已从build-src移出。

1. `ClassificationReversionKeepsIdentities`：保留相同字节，将frontend→shared→frontend，途中历史追加必须拒绝；回到旧输入后ID应相同、current只能1份、先前节点仍读固定原文且改名冲突。实际全部符合。此轮用“元数据回退触发旧修订重新current”检验身份与不可变性，没有仅重复前轮CAS反例。
2. `MalformedNativeTreeRollsBackPublication`：前驱TreeState损坏后修改Document内容再快照；应拒绝，SourceRevision／Commit／Item／Node各仍1份。恢复原前驱后相同expected重试应成功，旧节点仍返回first。实际全部符合；保留对象先写与DB回滚的既定边界。
3. `ArchivedRawOriginSurvivesOrdinaryCopyDeletion`：受管目录下再嵌套一层的raw文件保留旧team副本，先归档来源，再删除普通目标。应在所有6用途保持拒绝，删除副本后成员仍不可读／不可转移，所有者可读，原始raw对象不变。实际全部符合。
4. `DepthAndEmptyRangeBoundaries`：零字节来源建立包含层数16的group与node链，验证[0,0)及行1；第17层必须400，最深合法叶仍可读空串。实际全部符合。

这4组测试是新增运行证据；其余完整范围通过独立源码审查、此前同输入测试原件与脱敏真实服务结果逐项核对。没有用4个测试的数量代替完整需求覆盖。

## 5. 前25项处置的独立核对

以下处置均绑定本轮固定交付，已检查实际实现和正反证据；不是复制报告的Fixed标签。

| 原ID | 本轮处置 | 独立核对锚点／正反结果 |
| --- | --- | --- |
| AR-01 | Fixed | NLPRetrievalAdapter在模型构造／增强前guard；Agent探针构造与增强0，普通既有检索回归保留。 |
| AR-02 | Fixed | knowledgeObjectFields先验证精确唯一键；合法对象200，重复／大小写别名400。 |
| AR-03 | Fixed | 相对路径与basename分别校验，正常路径仍可用；末级超限不进入存储，返回400。 |
| AR-04 | Fixed | 三写入请求的关联ID校验小写十六进制；canonical正例，非法／大写关联400，不依赖MySQL排序规则。 |
| AR-05 | Fixed | 两写入路径共用Folder锁与锁定读头，时间递增；真实MySQLbranched=false，前后树保留；新故障重试未分叉。 |
| AR-06 | Fixed | link入口及后台convert均检查目标与源；受限拒绝／0映射，普通文件允许。 |
| AR-07 | Fixed | browse的GetByPfID、关键词、parent、大小／子目录统一过滤；成员0、所有者保留，真实MySQL0越权行。 |
| AR-08 | Fixed | ParseQuery错误／分号与显式空整数400；正常缺省／有效整数有正例。 |
| AR-09 | Fixed | unique KnowledgeFolderID在注册事务绑定；域与Folder改名仍按ID，空Folder不泄露。 |
| AR-10 | Fixed | UpdateDataset先CheckAccess再策略错误，锁内复检；无权／不存在同code与message。 |
| AR-11 | Fixed | OpenAPI及B说明的C4检索／证据／复用，C5 MCP授权工具与35.6一致；没有冒充已实现。 |
| AR-12 | Fixed | 不靠当前名称认领旧空Folder；成员隐藏／所有者可见，后增同名普通库仍不放开。 |
| AR-13 | Fixed | 多历史目录优先当前名否则最小ID，迁移可重跑，所有历史folder_id保留。 |
| AR-14 | Fixed | move／delete在写入前检查整棵受管子树；自身／祖先拒绝，纯改名／普通路径保留。 |
| AR-15 | Fixed | 上传、InitDatasetDocs、版本resolver、去重、普通Dataset删除均排除KnowledgeFolderIDs。 |
| AR-16 | Fixed | Unclaimed不含名称认领；从未快照的孤立Folder也不能经普通同名库公开。 |
| AR-17 | Fixed | File／Dataset origins与原生授权取交集，目标库scope及所有模型用途同一来源规则；副本删除保留其他引用源。 |
| AR-18 | Fixed | 原生commit整批验证租户、持久子树及操作，写对象前失败；同目录嵌套正例仍可提交。 |
| AR-19 | Fixed | ScheduleCreatedTasks、LoadFromIngestionTask、Execute、resolver、WikiDirty、consumer都在解析／模型依赖前guard；普通恢复发布1条、受限0。 |
| AR-20 | Fixed | 普通名称入口排除孤立目录；启动迁移固定内部owner，随后链接／改名不清除；普通自身目录删除仍成功。 |
| AR-21 | Fixed | 根例外核对self-parent tenant root且不得是自身；同名下级仍隐藏，公共容器正例可见。 |
| AR-22 | Fixed | origins通过持久ParentID递归到raw子孙；精确读／browse／link／目标／6用途都受限，目录外普通正例保留。 |
| AR-23 | Fixed | DELETE注册域锁内重读／复核后status0保留全部事实；3模式真实正反与新嵌套raw副本删除序列均符合。 |
| AR-24 | Fixed | 独立检查本地ResolveInfo调用链无Encode／Chat／Rerank；当前机器契约／B说明明确边界，旧报告保持历史字节。 |
| AR-25 | Fixed | README页首、接续步骤、尾部当前交付统一35章阶段0—6，A已实现／B待接；不再指令旧S0或称仅方案。 |

既有NotAnIssue也重新检查：DAO.Query对Dataset查询本来筛status1，delete_all不会因已归档域受阻；幂等register与归档重叠可线性化且不会重新激活，后续独立请求404。不要求契约未承诺的更强事务读。普通成员先登记他人租户me库被原生Read／Update拒绝，不能拿不可达输入扩大实现；Files本地上传再链接来源并非本轮Dataset multipart的C1承诺。上述均非本轮新发现。

Go命名复核针对本次增量：新文件与包沿原拥有者、标识符CamelCase、ID／SHA／JSON缩写一致、错误有Err前缀，未另建通用包或兼容API；没有为风格偏好要求重命名既有目录。

## 6. 工程证据、运行时与外部环境观察

独立解析`build-src/review-archive-final.log`：19包、3,478顶层／5,660含子用例pass，0fail；10个既有skip名称已逐一保存至`round-7-verification.json`。native与croptest两包ok。该日志有build.sh头，79个Go输入与工作区逐个一致；没有声称本轮重跑整套。四模式help日志各有Usage与正确模式，保留原生--help exit1约定。

脱敏真实服务证据逐项检查：`smoke-result.private.json`28组、`fixed-probes-summary.json`6组、`round2-fixed.json`3组、`empty-rename-fixed.json`1组、`folder-boundaries-fixed.json`6组、`provenance-live-fixed.json`8组、`directory-origin-live-fixed.json`6组、`archive-live-fixed.json`3模式，合计33附加组。读取对应断言脚本核对正反、清理语义和计数，不执行脚本，不读取其私有原始HTTP或错误文件。`commit-fixed.json`是原生与snapshot父链连贯；`mysql-scope.json`实际递归SQL有效且越权行0。fixture独立重算logical／instance／正文SHA，均一致，25字节2行CRLF。

`publication-verification.json`于`2026-10-23T22:08:02.776Z`记录12真实秘密值、index＋history 5,431 blob、0命中，保护文件与原导入根文件摘要未变、无原文／向量／二进制暂存。审查读取该脱敏结果及其范围；不重新读取秘密作扫描。原`阶段1-A底座验证报告.json`与a1a171c逐字节相同，保留2,435／4,144历史数字和旧binary，不把它当当前代码证明。

本轮直接读取安全Docker字段于`2026-10-23T22:13:57.378Z`确认：应用running／healthy，只读挂载SHA=`234dc7959f050cbcd733239bc26ab47a41d320cd50494f8159a32d4948e213d1`；进一步读取4个实际`/proc/*/exe`摘要均同SHA，api／admin／ingestor／syncer各1。五个RAGFlow依赖容器的ID、image、mount均与原baseline相同。

外部环境实测有新变化：旧`ai-complaint-web`的415bbac…不存在，同名当前e6f4f941…；旧mailbox的6a7c9ced…改名为old-v124-for-v127且退出，同名当前b1e56273…。二者当前healthy，image变化，mailbox挂载也变化。首次按旧ID读取web失败保留为真实核验失败，后改成按服务名比较；未重启／重建／回退这些容器，也无证据将其变化归因本任务。`round-7-verification.json`与主Agent的`external-environment-observation.json`安全字段一致。原工程时点“其他7容器未变”保持其历史绑定；本轮明确只称5依赖未变、2外部AICS发生变化。

主Agent另于`2026-10-23T22:14:15.380Z`经既有只读DB通道提取`current-safe-fingerprint.json`。本审查只读该脱敏文件，对照runtime-before及smoke.after的4摘要：旧POC KB／文档、provider_config、native_usage全部一致，仍6文档／42块／active0。没有由审查者import会读取env／access_token的native-api模块。当前数据摘要与当前binary证明外部容器变化未改变本次冻结产品、RAGFlow运行binary或所核对的POC／模型元数据；不宣称有全网络抓包或知道外部变更主体。

## 7. 前后绑定与结论限制

开始时实际逐个计算86文件摘要并保存`round-7-hashes-before.json`；结束再次逐个计算保存`round-7-hashes-after.json`，全86项无差异。79个Go文件与build-src也全匹配。requirements开始、结束均独立由完整request_snapshot UTF-8计算为`a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403`；subject前后均`8415e6faa2f995a74bf16b264c98598999246c14e549b395e13ad337914bb809`。HEAD保持720b88d…；原报告与a1a171c字节一致。

本轮完整约定范围已检查，无新增确认缺陷、未解决项或未检查项。没有真实模型API调用，真实数据修改0；仅忽略目录证据和隔离本地替身写入。A用户验收、B确认／页面／整体阶段1、后续推送仍由各自真实决定和动作推进。本报告不替代用户Acceptance或发布结果，不宣称绝对无缺陷、真实语义质量／百万规模、远程调用稳定性或破坏性恢复已通过。

```json
{"coverage":"Complete","findings":[],"unreviewed_items":[],"model_api_calls":0,"real_data_mutations":0}
```

86项完整逐文件SHA均保存在`round-7-hashes-before.json`与`round-7-hashes-after.json`；`round-7-verification.json.manifest_checks`保存预期／实际对照。上述为全量清单，不是抽样；本报告不重复展开同一清单。
