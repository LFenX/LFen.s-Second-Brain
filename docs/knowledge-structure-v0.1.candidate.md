# 阶段3-R1：确定性结构与代码关系候选

版本：Structure 0.1.0；状态：ReviewCandidate，未冻结、未接入。基线为 main `e6452d41afd159ba73e908f7ee20c43850a01f48`，运行仍是 Go `2bfa0d9`／Web `c759990`。本候选属于 **C1 节点／范围与 C3 处理许可的新增扩展**；基础 1.0.0、C2 0.2—0.7 的35份保护文件保持原字节，C4 检索／证据／复用未在本次定义或实现。

配套：[OpenAPI](knowledge-structure-v0.1.candidate.openapi.json)、[结构与语义例](knowledge-structure-v0.1.candidate.examples.json)、[准备及核对记录](stage3r1-structure-candidate-verification.json)。后者只证明候选及隔离依赖试跑，不证明产品能力。

## 1. 结果、范围与前置

将一个已保存的**合成项目固定清单**中明确选择的 TS、TSX、Go、Markdown 转成可读的结构视图：文件→符号／章节→有界原文片段；能查看静态依赖、证据位置、有效处理许可和实际状态。仍不建立全文／向量索引，不调用远程或本地模型。

这是35.8阶段3的第一个纵向切片，不是阶段3整体。PDF／PPTX／DOCX派生阅读、至少20项文档事实、3—5页人工Wiki及编辑保护，在后续候选分别落实。R7真实 AICS／个人文档／网站退出仍未执行，阶段2整体未退出；本候选仅基于已验证的合成来源原文继续实施，不代替这些退出要求。

本批只运行合成资料。现有域权限、原生身份、原件和所有历史保持。没有物理GC、后台扫描目录、订阅网站、RSSHub部署、模型调用、索引或多轮审查。候选得到真人确认前，不修改路由、业务实体、迁移、正式客户端、界面或项目依赖。

## 2. 已核实事实与选择

| 事实／拥有路径 | 对本批的约束 |
| --- | --- |
| `internal/service/dataset/knowledge_source.go`及`internal/entity/knowledge.go`已有固定修订、原字节范围和手工节点；父级限同域／同修订 | 复用原文与鉴权服务，不把手工节点改成另一种含义；新结构节点使用独立类型和命名空间 |
| `internal/permission/knowledge.go`将已登记知识域的原生ingestion及六类模型用途拒绝；C1/C3策略为catalogue-only | 新增明确的structure-only许可和专用确定性入口，不绕过原生ingestion检查，不将其解释为Embedding许可 |
| `internal/parser/parser/parser_type.go`无TS／TSX／Go AST分支 | 新增本批语法生产器；不是配置一个现成chunker就已支持代码结构 |
| Markdown现有解析会转码／改写表格，`FetchRemoteImages`默认true，配置入口未暴露该开关 | 原文坐标直接来自固定字节；本批只调用不联网的语法解析，不经该通用格式改写／取图路径 |
| `internal/ingestion/component/knowledge_compiler/structure/compile.go`通过GenJSON调用Chat | 不复用为本批静态代码关系生产器；不能声称原生LLM结构编译是零模型AST |
| 原生Mom／MaterializeParentChunks／RetrievalByChildren属于索引层父文本组织 | 可以后续适配，不能当作来源节点身份、跨文件关系或本批发布记录 |
| `knowledge_lifecycle_fence.go`和项目清单已有域锁、来源世代、固定清单 | 新任务固定原元组，提交时重检世代及清单头；不改C2回执及历史 |

选择的解析依赖：Go标准库`go/parser`／`go/ast`；TS／TSX采用`github.com/tree-sitter/go-tree-sitter v0.24.0`与`github.com/tree-sitter/tree-sitter-typescript v0.23.2`；Markdown复用已存在的`github.com/yuin/goldmark v1.7.1`。前两者MIT，语法包的该版go.mod明确依赖该版绑定；TS与TSX语法分别处理，不把TS套到TSX。

隔离Linux Go1.27.3／CGO／Clang试跑已成功，不修改项目go.mod或运行应用。约2MiB TSX五次解析161—196ms，进程VmHWM为85,648KiB；是单一合成语法样本的依赖可行性，**不是API、结构生产、并发或生产p95结果**。该输入有34,183个顶层语法子项，会超过本候选的生产节点上限，不因语法试跑成功就保证能发布。

试跑发现并保留：v0.24.0 `ParseCtx`在取消后立即Close的调用中触发`parser.go:168`空指针；本批禁用该API，采用`SetTimeoutMicros`＋`Parse`，每文件独立Parser／Tree并显式Close。1微秒限制下大样本返回nil已验证。Go／Markdown在文件处理前后和遍历时检查取消；**不宣称能在其标准库解析调用中途硬抢占**。没有修改或复制第三方绑定。

外部依据：[Go ParseFile](https://pkg.go.dev/go/parser#ParseFile)、[绑定v0.24.0](https://github.com/tree-sitter/go-tree-sitter/tree/v0.24.0)、[语法v0.23.2](https://github.com/tree-sitter/tree-sitter-typescript/tree/v0.23.2)、[该语法依赖](https://github.com/tree-sitter/tree-sitter-typescript/blob/v0.23.2/go.mod)、[goldmark v1.7.1范围载体](https://github.com/yuin/goldmark/blob/v1.7.1/ast/block.go)。版本固定，不引用浮动latest作为实施输入。

备选比较：保持现状可读原文但无自动结构；TypeScript编译器API能提供更深语义，但增加Node运行耦合和UTF-16坐标转换；本批选Go内的语法生产，先满足原字节、确定性、无执行／无联网和可控预算。它不提供完整类型检查、运行时调用图或完整GraphRAG。

## 3. 三类结构和身份

1. **组织树**沿用域→项目／模块→来源；frontend／backend／shared标签来自C2固定清单分类，不成为权限。
2. **结构树**是某次已发布视图内的文件→声明／章节→片段；不移动组织分组。
3. **关系**连接同一视图中的固定结构节点。包含关系由parent投影，其他关系保存类型与证据；不是树父级。

原生Document、File、FileCommitItem及C2 source_id／source_revision_id仍是唯一来源／修订。仅接C2目录／文本且有C1固定根的清单成员；`original_only`／`web_capture`不伪造C1根或原字节坐标。旧C1手工节点及其list／get／content语义不变。

新`StructureView`保存输入清单、精确选择、生产器／配置摘要、成员固定修订／世代、创建者和完成摘要。新`StructureNode`为派生节点，不冒用C1 node_id；统一范围服务在内部复用同一原文／授权实现，数据库不另存一份源码。首版关系与片段不进入DocEngine。

新节点返回`structure_node_id`、`logical_node_id`、`view_id`、`source_id`、`source_revision_id`、`source_node_id`、`parent_id`、`kind`、`name`、`logical_key`、范围和生产版本。`source_node_id`始终是原C1根，只用于原文回链；不得把`structure_node_id`传给旧C1节点接口。新增节点存储及成员关系由A在现有entity／dao／dataset分层内完成，不重排目录。

### 3.1 摘要和稳定性

复用C2 0.7的typed-LP编码和SHA-256；原值不trim、casefold或Unicode归一。整数限JS安全整数，UTF-8非法值、未配对代理项、重复JSON键拒绝。

- `request_sha256`投影：kind=`knowledge-structure-task/0.1.0`、owner_id、domain_id及去掉idempotency_key后的完整请求。members按source_id原字节排序；重复source拒绝，成员顺序不改变语义；aliases按prefix排序且重复／重叠prefix拒绝。原键作用域独立保存。
- `producer_sha256`投影：固定语言／语法／生产格式版本、片段规则、别名映射和本批全部限制。该投影在正式冻结件给定，不读取外部编译配置。
- `view_id`：kind=`knowledge-structure-view/0.1.0`、owner_id、domain_id、project_id、manifest_id、manifest_sha256、foundation_policy_version、structure_policy_version、producer_sha256及规范顺序成员的source／revision／generation／原文SHA／分类／路径。expected_active_view_id只控制发布CAS，不改变视图内容身份。
- `logical_node_id`：kind=`knowledge-structure-logical/0.1.0`、domain_id、source_id、node_kind、logical_key。node_kind取节点kind字段；命名空间与旧C1分开，不同来源的相同源码不合并。
- `structure_node_id`：kind=`knowledge-structure-node/0.1.0`、view_id、logical_node_id、source_revision_id、start_byte、end_byte。同次重跑相同输入和版本得到相同身份。
- `relation_id`：kind=`knowledge-structure-relation/0.1.0`、view_id、relation_kind、from_node_id、可空to_node_id、reference和完整evidence对象；relation_kind取关系kind字段，evidence含source_id／source_revision_id／start_byte／end_byte／fragment_sha256。重复语句不同位置保留，不按文本去重。reference使用返回的原文或明确截断前缀，其余字节仍由证据范围与摘要钉选。
- 策略命令摘要：kind=`knowledge-structure-policy/0.1.0`、owner_id、domain_id、expected_policy_version、expected_structure_policy_version、enabled、confirmed；取消命令摘要：kind=`knowledge-structure-cancel/0.1.0`、owner_id、domain_id、task_id、expected_task_revision、confirmed。均不把独立原键混入摘要。

logical_key使用命名声明的词法作用域／声明种类／名字／同名次序；匿名或歧义声明退回父范围内语法次序并标`position_based`。只对作用域与名字未变、无同名歧义的声明承诺连续逻辑身份；改名、插入同名声明、解析版本变更不承诺跨版匹配。文件移动不因路径改变source_id；元信息新世代仍形成新view绑定。

## 4. 坐标、父子和片段

唯一权威坐标是**保存的UTF-8原字节、0起始、半开区间[start_byte,end_byte)**；行号1起始，end_line是区间最后一个字节所在行；空文件根为[0,0)、1—1行。按LF计行，保留CR；BOM占原字节。Go使用`token.File.Offset`／未受`//line`改写的物理位置，Tree-sitter用byte而不是UTF-16列。前端需要编辑器位置时由原字节显式换算，不能把JS字符串索引直接当byte。

Tree-sitter BOM样本root起始为3；发布的文件根仍覆盖[0,len)，不能丢掉BOM或注释。语法节点不等于原文覆盖片段。每个文件另有一套有序、无交叠、无空洞的`passage`叶片覆盖[0,len)，其逐字节拼接必须等于原件；不做空白清洗、trim、摘要替换或重编码。

片段目标4KiB、硬上限8KiB，优先在声明／语句或完整行边界截断，超长单行在UTF-8边界切开；默认overlap=0，邻接／父级由引用读取，不重复保存源码。小于4KiB的一句话不强拆；千行函数保留一个symbol父节点并有多个passage。语义节点允许包含重叠，覆盖校验只针对上述单一passage序列，不能将重叠父文拼接计算覆盖率。

首版切点固定为：剩余≤8KiB时直接收尾；否则在[start+2KiB,start+8KiB]内，先选声明／语句结束边界，再选物理完整行末，各类内取距start+4KiB最近点，同距取较早byte；都没有则在start+4KiB之前最近UTF-8边界切。所有候选必须前进并位于源内。相同原文／生产版本得到相同序列，不由前后端各自选择启发式；代码围栏等完整原字节仍按这一序列覆盖，语义父节点可以跨片段。

结构父级同view、同source_revision；包含关系不可越界且无环，深度≤16。passage选择最小完整包含它的语义父节点，否则归文件根；不为强行凑父级截掉跨声明片段。检索时的parent expansion在C4另定，不自动把整份2MiB文件送给Agent。

单次content正文≤64KiB。省略start_byte从节点起点开始；省略end_byte取不超过64KiB的UTF-8安全窗口，返回next_start_byte及complete；提供end_byte必须同时提供start_byte。显式范围超过节点／预算或切断UTF-8返回400，不悄悄改范围。complete指本窗口结束已到节点末尾，next_start_byte在complete时为null。响应含原件SHA、片段SHA及绝对源坐标；JSON转义后的总包上限512KiB，不能把64KiB原文预算误称总网络包大小。跨窗口客户端必须固定view／node／revision，不能跟随当前头。

Markdown先产文件、ATX／Setext章节和完整原文片段。goldmark v1.7.1的Lines是内容段，不含全部标题标记、围栏及表格父容器位置；生产器必须结合原始行表、语法父级与围栏状态恢复边界。重复标题按源位置区分，代码围栏内伪标题不出章节。未能证明的表格／列表细粒度边界首版不发布为独立节点，内容仍完整保留在章节与片段，返回`block_navigation_not_materialized`提示；不伪造坐标。富文档结构在后续切片扩展。

## 5. 静态事实关系

| kind | 可发布的确定事实 | 边界 |
| --- | --- | --- |
| contains | 已发布parent关系的只读投影 | 不另存第二套父级，不把模块归属写成语法父子 |
| imports_file | TS／TSX语法import或re-export指向本次精确选择中的唯一文件 | 只解析相对路径及明确alias；外部／未选／有多个候选均未解析 |
| exports_symbol | 明确本地export或显式命名re-export绑定到唯一声明 | export-star不枚举猜测，循环或歧义保持未解析 |
| references_type | 语法type引用按词法作用域绑定到本地或明确导入的唯一声明 | 泛型遮蔽、namespace／成员、类型推导不明时不猜 |
| calls_local | 调用表达式的标识符按词法作用域引用某声明 | 只表述词法静态引用，不承诺运行时callee；动态调用、对象成员、反射不产确定目标 |

每条关系有from、可空to、resolution=`resolved|unresolved`、reason、原文reference及同源证据范围。未解析关系**必须保留原语句／范围与原因**，但不带猜测target ID。注释／普通字符串内的伪import不出事实；`import()`记录dynamic_import未解析，不当作静态依赖。

TS相对文件候选为精确`.ts`／`.tsx`、无后缀对应两者及`/index.ts`／`index.tsx`；多候选即ambiguous。alias只接受本次请求最多8条不重叠prefix→项目内相对目录，按原字节，不加载tsconfig、package.json脚本、node_modules或外部文件；不执行代码、npm install、编译器插件或网络解析。CSS、包依赖和范围外文件仅给未解析引用。首次Go只解析声明、方法、类型及同文件词法绑定；Go import包路径保留未解析，不假称执行了go/packages／模块下载／完整跨包分析。

局部关系接口一次一跳；方向与kind过滤，最多20条，不取全图、不做社区划分或LLM摘要。所有目标必须在同一view明确选择集合中；图边不授予任何读取权。

## 6. 处理许可和接口

基础C3的`catalogue`、`allowed_model_uses=[]`、`external_models=false`、`automatic_ingestion=false`保持。新增**独立structure-only子策略**，每域默认disabled、version=1；所有者明确开启后version递增，允许手动确定性结构任务，关掉后新任务及未激活发布拒绝。它不改基础policy_version，不授予搜索索引或任何模型用途；关闭处理不删除／撤销已保存历史的当前有效读取权。

本批只在合成域明确开启。新处理入口检查主体、域、项目、子策略及精确文件范围；既有`CheckKnowledgeIngestion`与所有模型guard仍拒绝，不借“structure”放行整个原生pipeline。页面同时展示基础策略与结构子策略，避免“关闭Embedding就不能组织资料”或“开启结构等于开启模型”的误解。

接口基址`/api/v1`，原生登录。请求发送基础`X-Knowledge-Contract-Version:1.0.0`和新`X-Knowledge-Structure-Contract-Version:0.1.0`；不使用C2来源版本头伪装新接口。新信封有contract_version、structure_contract_version、request_id、code、message、data。新路由均no-store；不同版本412。

域前缀为`/datasets/{id}/knowledge`：

| 方法 | 新路径 | 作用 |
| --- | --- | --- |
| GET | `/knowledge/structure-contract` | 固定版本／解析器／预算／实际就绪；候选本身不使runtime_ready变true |
| GET | 域前缀＋`/structure-policy` | 当前基础策略版本及structure-only子策略 |
| POST | 域前缀＋`/structure-policy/changes` | expected子策略版本、enabled、confirmed和原键；显式开启／关闭 |
| GET | 域前缀＋`/structure-commands/by-key` | kind=policy或cancel与原键查固定成功／拒绝结果 |
| POST | 域前缀＋`/structure-tasks` | 固定当前项目清单、精确成员／原世代、选定parser、alias、双策略版本及expected_active_view_id；明确确认后启动 |
| GET | 域前缀＋`/structure-tasks/by-key` | 原键读原任务或固定接收拒绝结果／原请求／摘要，不创建任务 |
| GET | 域前缀＋`/structure-tasks` | 指定project_id的有界任务页；状态是读取时状态，无全库总数 |
| GET | 域前缀＋`/structure-tasks/{task_id}` | 固定身份、原请求、版本、逐成员状态／诊断、结果view或失败 |
| POST | 上述任务＋`/cancel` | 原键＋expected_task_revision，记录取消请求或终态no-op |
| GET | 域前缀＋`/projects/{project_id}/structure-head` | 当前发布view引用及按当前清单／世代计算的current／stale／none与原因 |
| GET | 域前缀＋`/structure-views/{view_id}` | 固定输入／生产器／成员／统计，不能把它作为当前头缓存 |
| GET | 上述view＋`/nodes` | 必填source_id，省略parent_id读该文件根；带parent仅读直接子节点；分页 |
| GET | 上述view＋`/nodes/{structure_node_id}` | 固定结构节点和源回链 |
| GET | 上述node＋`/content` | 有预算的原字节窗口，不取邻接或父全文 |
| GET | 上述view＋`/relations` | 固定节点的一跳关系与证据；不跨view拼接 |

变更体≤32KiB，严格未知／重复字段和查询检查。source_id／revision／task／manifest用原生32hex，view／结构node／关系／SHA用64hex。任务选择1—32个唯一source，明确parser枚举typescript／tsx／go／markdown；文件后缀必须与parser一致（小写`.ts`／`.tsx`／`.go`／`.md`／`.markdown`），不静默降级。分类来自固定清单，不接受客户端重写标签。

## 7. 任务、原键、增量与发布

**任务、策略、取消三个写操作都**在POST前保存完整原请求／原键／摘要及主体／域恢复槽，kind分别为task／policy／cancel；task另存project_id，cancel另存路径中的task_id，策略槽不伪造项目目标。每槽≤16KiB，每主体最多8个未决槽；按owner／domain／kind／目标隔离，不存原文、Token或对象URL。本地存储失败或容量满则不发起新POST，不驱逐未知结果。已有导入恢复槽不覆盖、不复用其flow名称。固定命令回执不能写回为当前策略／任务状态，恢复后另GET当前对象。

任务原键作用域owner／domain／structure-task／key；同规范请求同键回原任务，不重置版本、预算或状态；异请求409。策略与取消命令各有kind命名空间，固定命令回执保留原请求／摘要／before／after或任务取消结果。当前读取授权必须先于回放；已成功任务／命令恢复不重施，也不对原动作重做当前世代CAS。首次接收201，恢复200，任务终态不自动重试。

original_request保存首次严格解码后的完整字段值及首次member／alias顺序，不声称保存HTTP原始空白／字段排列；摘要投影才按前述规则排序。相同语义的重排请求回原对象，客户端按规范摘要核验，不能把JSON序列化顺序当作不同业务身份。

取消的路径task_id参加原请求身份、摘要和回执；scope为owner／domain／cancel／key，**同键跨task必须409**，不能将T1的结果当T2。取消槽恢复固定命令后继续追踪原task；放弃一个取消操作槽不丢失任务跟踪槽。

对已经通过认证、域权限、严格格式和摘要核验的请求，接收阶段在域锁下先查原键，再检查语义条件；策略未启用、版本／世代／清单／head过期、任务取消版本过期或域任务容量满等确定拒绝，**同一事务持久化`RejectionReceipt`后**返回错误。它包含operation、主体／域、可空task_id、原body／原键／摘要、稳定reason和时间，不创建可执行task。原键GET返回这一固定拒绝；同body/key再POST返回原拒绝，即使当前条件已变也不重新执行。无权、格式／契约错误、事务回滚、DB不可用和未能保存的结果不伪造此回执；503或缺少固定结果继续按未知处理。

收到409／429不是清槽依据。客户端先GET原键并核对完整身份／摘要；仅拿到匹配的固定拒绝，才允许“明确放弃本操作”→重新读当前策略／清单／世代→新body/key→重新认可。任意404、超时、503、401或版本不匹配都不证明原请求未执行，保持隔离原槽；修复访问／客户端后继续原键核对。错误信封的`outcome_known`只描述本次处理，`rejection_id`仅在固定拒绝已提交时非null；不能据其独自推断之前请求。已存在异摘要原键返回idempotency_conflict，保留原固定对象；界面显示身份冲突并禁止自动替换或清除任务跟踪。

新任务接收时在域锁内核当前manifest_id、expected_active_view_id、基础／结构策略、全部选择仍属于该清单且为当前active来源及指定revision／generation；捕获原始范围。32个选择以外的清单成员不解析、不读取对象、不参与关系目标。固定清单本身是显式选择形成的非原子目录快照，**本批不把captured_atomically改为true**。

页面准备沿用旧接口而不扩字段：先读当前0.4固定清单；用户精确选择后，对每项读0.7 lifecycle detail，检查current固定revision、路径、分类与清单完全一致且state=active，再把`current.lifecycle_version`作为请求member.lifecycle_version。目录清单没有该世代字段，不从revision或时间推算；任一不匹配提示重建明确清单，不能自动换成员／预期。服务端最终仍原子重检，前端逐项读取不是授权凭据。

任务状态：queued→running→succeeded或failed；queued/running可cancel_requested→cancelled。succeeded仅在整view激活事务成功后成立。逐文件为pending／processing／staged／published／failed／cancelled／reused；staged与reused在任务成功前均不是可见节点，激活时staged转published。finished_members统计非pending／processing项，不代表已经发布；失败／取消时未处理项明确转cancelled，不留永远pending。每项诊断≤8条，每条message≤128个UTF-8字节，只包含受控错误，不回传进程日志。任何解析错误、超限、权限／策略／世代／清单／head CAS失败，整view不激活，原view不撤回；不支持首版半个项目可见。

Task.revision就是下一取消请求的expected_task_revision；每次可见状态／成员进度／租约接管结果变更递增，心跳不递增。终态有completed_at且不再变化；queued/running/cancel_requested为非终态，succeeded/failed/cancelled为终态。策略首次改变才递增子策略version，同值no-op不递增；达到安全整数上限拒绝改变。取消首次命令先核expected_task_revision，过期409并保存拒绝；已cancel_requested或终态且版本相符时返回固定unchanged回执，不倒退状态。相同原键成功回放先查原回执，不再次对其做这些CAS。提交与取消共用域锁：取消先提交则激活拒绝；激活先提交则版本已变，旧取消保存版本拒绝，重新明确取消终态只得no-op。cancel_requested确认后客户端不将它显示为cancelled，需读到真实终态。

处理在SQL锁外，按文件固定字节SHA复验；缓存只存确定性位置／事实，不复制源码，键含owner/domain/source、内容SHA、parser和producer摘要，不跨域内容去重。内容未变时可以复用语法产物，但新修订／世代／分类／view身份必须重新绑定并验证；不是原样复用旧授权结论。

激活事务沿用域锁，重新授权并比较原manifest头、每个原来源revision／单调generation／分类与位置、双策略和expected_active_view_id；一次发布全部节点／边／输入摘要及唯一active view引用。并发只一胜。已有相同view内容可复用，但只有原CAS满足才更新当前指针。内容／分类／位置／归档恢复使旧view为stale；head以稳定原因码和affected_sources中的source_id标出失配，按最多32个成员的当前事实计算，不新增第二份来源头，不要求改C2回执。

进程恢复使用持久任务、每域单执行租约（30秒，10秒续约）及单调attempt fencing；同域最多1个执行＋1个等待，超出429。正常重启仅接续已明确创建的任务，复验后复用staged成员；不是自动导入。单任务受创建后120秒期限、每次30秒处理软截止及最多3次接管限制，耗尽则failed，不悄悄换任务／键。过期worker不能发布。

取消请求立即持久回执，取消不意味着解析调用已瞬时终止；worker在文件边界、遍历检查点及提交前检测，最终cancelled不发布。TS语法调用原生5秒超时；Go／Markdown标准解析为有界输入后的软取消。硬实时终止或进程内存硬隔离不在此候选承诺。API失联后先原键恢复，404不证明未执行；核当前权限后只能用户明确重发同body／同key，不能后台换键。

## 8. 权限、分页和旧历史

首版只有域所有者。无权域、项目、view、node、task、命令、cursor一律404无数据，原生未登录保持401。关系、统计、诊断、文件名和缓存受同一主体范围约束；relation不是授权。读取view前重查所有成员的当前读取许可，存在不允许成员则整个view隐藏，不先返回计数再过滤。

来源归档沿用C2 0.7语义：不是权限撤销；所有者可以读有权固定历史，界面标记当前状态，不能把归档误叫删除。域失效或失权后旧view／原文／回执／cursor及缓存都不能继续读。structure-only关闭只禁止新处理／激活，已有有权历史仍可读。

nodes与relations每页1—20，task列表每页1—20；按元信息总包预算缩页，不能为凑足20项超出256KiB。服务端32hex cursor固定有效600秒，绑定主体／域／项目或view／source／parent／方向／kind／limit及游标位置，不随续页延长；错scope或过滤400。direction省略为both，kind省略为不限；空串／重复／未知查询拒绝。不可见对象在游标检查前完成鉴权。节点顺序为start_byte、end_byte、kind、structure_node_id；关系按relation_id；任务按created_at倒序、task_id倒序且固定首次列表上界，不声明跨页状态快照。所有读取重新授权。

## 9. 本批限制与性能验收

| 项目 | 冻结候选值 |
| --- | --- |
| 输入 | 1—32文件，单件≤2MiB，合计≤16MiB，仅固定清单中有C1根的UTF-8文本 |
| 产物 | 单文件≤8,192节点、全view≤32,768节点／65,536关系；按遍历计数拒绝超限，不截断后冒称完整 |
| 原文／分页 | 单次正文≤64KiB；片段目标4KiB、≤8KiB；结构树深度≤16；页≤20；元信息JSON≤256KiB，content JSON≤512KiB |
| 任务 | 每域1执行＋1等待；30秒租约，最多3次接管；创建起120秒总期限；每次30秒软处理截止；TS单次原生5秒限制 |
| 客户端 | 请求等待25秒；前台可见时轮询≤每2秒1次，离页停止；等待超限转原键恢复而非无限自旋；恢复槽≤16KiB×8 |
| 性能门槛 | 正确性先过；冻结8文件／≤1MiB合成基准5次：端到端最长≤10秒、metadata／局部关系／64KiB窗口每项20次串行最长≤1秒；worker相对基线RSS增长≤256MiB，实际环境／冷暖分别报告 |

这些是实施后验收目标，当前未跑。隔离依赖耗时及R6元信息118ms不能代替本批API／完整结构生产测量；五次最长值不称生产p95、百万文件扩展能力或稳定吞吐。测量超标则定位并修复同一范围，不以更换指标绕过。将来更大文件沿原件／直接读取路径，不能强迫全量结构化或Embedding。

## 10. 单写入实施顺序和验收

| 顺序／负责方 | 工作及交接 | 验收证据 |
| --- | --- | --- |
| S3-R1.0 A＋只读消费会话 | 核对本候选、身份、子策略、接口、原键和失败语义；由人确认同一候选后冻结 | 三件候选摘要／一次集中消费核对／真人确认；未确认前业务改动0 |
| S3-R1.1 A唯一写入 | 固定依赖、UTF-8范围与TS／TSX／Go／Markdown生产；原文片段和静态边 | 合成预标至少40个代码／Markdown事实，BOM／CRLF／emoji／Tab／重复声明／伪import／遮蔽／动态与歧义；passage逐字节覆盖100%，父级／关系证据正确；错误不发布 |
| S3-R1.2 A唯一写入 | 子策略、持久任务、租约恢复、原键／取消、全view激活与世代CAS；有预算读取 | 有意义Go单元及真实MySQL／S3合成集成；双API竞争、进程中断、源世代ABA／禁策略／超限、旧view仍可读；所有已冻结件不变、模型0 |
| S3-R1.3 清洁固定提交交消费会话唯一Web写窗 | 正式service→hook→目录内结构页：组织／文件／结构分开、符号与原文联动、局部依赖、子策略、任务与恢复 | 遵守web/AGENTS，复用现有树／分栏／Monaco／列表，禁改共享ui；成功／空／失败／无权／stale／刷新恢复；remote image请求0；不扩成新独立后台 |
| S3-R1.4 A收回写权 | 原六服务同版真实HTTP／浏览器；人与程序同固定节点读取、负权限及性能 | 明确选择→开启子策略→结构任务→发布→符号／片段／边→固定旧版／更新stale→恢复；上述预算实测，凭据扫描、原POC指纹及保护摘要；正常提交后推main |

不开展多轮审查。发现本范围问题集中修复并定向验证，语义变化先修候选／获必要确认。工程结果与人的验收独立保存，未回答为Pending。R7真实资料退出、富文档和人工Wiki、阶段4检索／引用／代码复用、阶段5 MCP临时授权、阶段6人格记忆分别保持未完成，不由这个切片代为验收。
