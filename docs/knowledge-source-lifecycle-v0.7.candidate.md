# C2 0.7.0 来源生命周期候选

2026-10-26；**ReviewCandidate：未冻结、未实施、未部署**。基线为已发布R5的main `81f3a8c8bb176155e70e4150906555af0d3cac88`。本文、[OpenAPI](knowledge-source-lifecycle-v0.7.candidate.openapi.json)、[合成向量](knowledge-source-lifecycle-v0.7.candidate.examples.json)共同定义待确认候选。[验证记录](stage2r6-lifecycle-contract-verification.json)只证明文档结构、参考规则、摘要与29份保护文件，不是应用或迁移验收。

## 1. 结果与范围

使人能按稳定来源ID执行重分类、受限逻辑移动、归档和恢复，看到明确前后差异，失败后按原请求恢复；旧导入不能用过期准备覆盖这些动作。保持原生Document／File／FileCommitItem身份，不新建第二套知识库或当前头。

继续仅合成、外部模型／本地推理0、不访问真实网站或RSSHub、不启动多轮审查、不物理GC。知识域私有所有者权限沿用当前后端；本候选不开放MCP共享、临时Key或成员权限，不重建基础设施。

不含跨域／跨登记转移、分组reparent、整树批量操作、外部磁盘或对象搬移、网页URL迁址、自动追踪重命名、内容回滚、域恢复、标签体系扩展、后台扫描或索引。

## 2. 统一身份、状态与四动作

- `source_id = Document.ID`；`native_file_id = File.ID`；`source_revision_id = FileCommitItem.ID`。路径、原件名、URL、GUID和SHA均不是来源ID。分类仍为完整的`group_id/component_side/language`，组织分组不授予权限。
- 新增私有来源生命周期记录，仅保存`source_id/state/lifecycle_version`等状态；唯一当前头仍为既有`KnowledgeSourceRevision.is_current`。状态为active／archived。所有有效来源初值1，既存来源初始化1不表示历史只操作过一次。
- 世代范围为JSON安全整数`1..9007199254740991`，数据库int64。任何实际内容／头／路径／分类／active状态变化均加1；历史输入复用旧修订ID也加1。策略版本独立核对，不因域策略更新批量改所有来源世代。新来源首次发布世代1；真正no-op不增加。

| 动作 | 可执行状态 | 当前头／世代 | 字节与固定历史 |
| --- | --- | --- | --- |
| 重分类 | active | 完整三字段改变形成或复用该表示的元信息修订；世代+1 | 复用已验不可变对象，不下载或复制原件；旧节点／原件／网页／清单不重写 |
| 逻辑移动 | active且该来源支持 | 同登记精确目标；绑定及元信息修订在同事务更新，世代+1 | 只改知识逻辑位置；不调用通用rename／move，不改Document／File原名或对象位置，原件原名和媒体类型保持 |
| 归档 | active→archived | 头保留，世代+1；已archived时新键合法no-op | 保留Document有效状态及全部保护／历史；不是删除或撤权 |
| 恢复 | archived→active | 头保留，世代+1；已active时新键合法no-op | 重新核验域／登记／当前组／位置／策略，不恢复外部授权、任务、索引或旧正文 |

archived源禁止重分类／移动；归档／恢复的no-op仍先完整授权、策略与双CAS。UI只突出当前有意义的状态动作；API保留明确定义的状态no-op。世代已到上限时真正变化拒绝，合法no-op仍可返回unchanged。

| 来源 | `source_kind`／表示 | 首版移动 |
| --- | --- | --- |
| C1无登记直接快照 | kind与registration均null／text；不制造登记 | 禁止；支持重分类、归档／恢复 |
| C2 0.2 | text_file／text | 仅同登记已有IncludePaths中的精确目标 |
| C2 0.3／0.4 | controlled_directory／text；0.4是生产方式 | 禁止；其IncludePaths实际存目录范围，不是文件白名单，不能只搬知识记录冒充移动外部文件 |
| C2 0.5 | original_document／original_only | 仅同登记已有精确目标，沿用当前原件检查的媒体族约束；不借路径变化转换格式 |
| C2 0.6 | web_page或web_feed／web_capture | 禁止；GUID／Atom ID／URL及生成的web路径不能按文件改名 |

## 3. 位置、保留与旧列表

当前Binding保持一源一条，不插第二条普通Binding作旧位置别名。移动后原位置成为独立私有退役占位，绑定原source_id且不自动到期；active、archived及退役位置均阻止他源占用，同SHA也不合并、不覆盖、不加后缀。原源可通过新明确移动取回自己的退役位置：目标占位原子转为当前绑定，原当前位置转为退役；不是将位置释放给他源。旧占位事实保留在固定动作回执。

位置按登记ID＋原始UTF-8路径区分，不trim、casefold或NFC/NFD转换。当前绑定路径已有`utf8mb4_bin`迁移，本机只读元数据查询也确认这一点；复用该二进制约束。退役位置以第5节position摘要做私有键，并保存原路径作字节复验。数据库对尾空格等发生唯一冲突时明确拒绝，不能将其当同源；不承诺支持并列尾空格文件名，不为本轮重建当前绑定模型或更换全库排序规则。

GET目标检查不预留、不持锁、不创建操作准备；确认时重新检查目标和数据库唯一性。跨两个API进程抢同一位置只能一胜。固定HTTP回执、历史SourceRevision／节点、原生Commit／Item／TreeState、旧项目全部清单、原件／网页三表示、任务及备份均保留。无对象或历史GC。

默认当前来源列表在SQL分页／计数前排除archived：C1文本、0.5原件、0.6网页当前头均适配，DTO原字节不扩字段。归档入口使用新0.7列表。目录外部listing仍是外部文件事实，不把“磁盘存在／缺失”变成恢复／归档许可。固定修订详情、原文／原件／网页表示、旧项目清单和回执在当前有效授权下继续读取；域失效／撤权仍拒绝，归档不构成共享授权。

## 4. 独立接口与界面

所有路径位于`/api/v1`，原生Authorization；请求同时发送`X-Knowledge-Contract-Version:1.0.0`及`X-Knowledge-Source-Contract-Version:0.7.0`。仅新增0.7命名空间，不修改C1/C3或0.2—0.6头、DTO、枚举、摘要投影和已存manifest。响应no-store，版本及request_id可核对，JSON含信封≤32KiB，无对象地址。

| 方法 | 路径（域前缀为`/datasets/{id}/knowledge`） | 作用 |
| --- | --- | --- |
| GET | `/knowledge/source-lifecycle-contract` | 版本、动作、预算、迁移就绪与禁止事项 |
| GET | 域前缀＋`/source-lifecycles` | 默认active；显式archived、组范围、组件侧和语言筛选，SQL先过滤再分页 |
| GET | 域前缀＋`/source-lifecycles/{source_id}` | 当前身份、表示、策略、完整当前快照、逐动作supported／allowed／reason |
| GET | 上述来源＋`/move-target` | 必填目标及双expected／策略；只核查本次目标可用性，不预约 |
| POST | 上述来源＋`/reclassifications` | 完整分类明确确认 |
| POST | 上述来源＋`/moves` | 精确逻辑目标明确确认 |
| POST | 上述来源＋`/archives` | 明确归档 |
| POST | 上述来源＋`/restores` | 明确恢复 |
| GET | 域前缀＋`/lifecycle-commands/by-key?action=…&idempotency_key=…` | 原动作／原键查固定结果 |
| GET | 域前缀＋`/lifecycle-commands/{command_id}` | 按ID读固定结果 |

列表每页1—20，按实际JSON预算缩页，source_id升序；cursor为私有32位ID，绑定主体／域／state／组范围／组件侧／语言／limit，10分钟固定有效。`group_scope=all|ungrouped|exact`默认all；仅exact要求group_id，其余禁止附带group_id，exact只匹配本组，不递归。语言省略表示不限，空串表示未标语言。每次重新授权；不是跨请求固定全库快照，不增加昂贵全库总数。

详情`current`包含固定修订、输入SHA、世代、状态、逻辑路径、完整分类、原生表示SHA与长度；网页的原生表示是decoded，不把它称原Feed SHA。C1／原件／网页固定阅读继续走各自既有接口，不新增正文下载接口。

四动作共同请求为原键、`expected_source_revision_id`、`expected_lifecycle_version`、`expected_policy_version`及`confirmed:true`；重分类另给完整classification，移动另给relative_path。其他字段不接受。目标仅已有允许清单；沿用既有凭据路径／控制字符／格式检查，合法Unicode标量，拒绝非法UTF-8、未配对代理项、重复键、未知字段和尾随JSON。分类语言沿用已有32字符、无NUL且首尾不带空白规则；null group和空language均明确保留，不合并为缺失。

界面在知识目录提供“当前／已归档”及来源状态详情；当前固定阅读旁读取独立生命周期详情，不把历史快照当当前状态。确认页展示前后分类／位置／状态、世代与预期修订，勾选默认false。刷新、冲突或目标变化清空认可，不自动替换expected后POST。不支持的移动显示原因；归档文案不叫删除，恢复不叫回滚内容。

## 5. CAS、原键和故障恢复

新命令依次执行：当前授权与契约→域锁→原键固定回执核对→未有成功回执时检查策略和双CAS→状态／目标／组／登记检查→原生元信息／绑定／当前头／世代／回执同一事务。锁顺序固定为域→原键→来源ID→位置键→原生folder；读取对象正文或网络操作不进入此元信息事务。元信息命令服务端10秒上下文、客户端25秒等待；不是吞吐或锁等待保证。

原键作用域为`owner_id/domain_id/action/idempotency_key`；键为1—128个ASCII字母数字、下划线或连字符，区分大小写，不trim。同键同原元组在当前读取授权后回原固定回执，不重施旧动作、不对其再做当前头CAS；策略版本变化但仍有当前读取权时也不重施。异来源／异目标／异expected同键均409 idempotency_conflict。首次成功（含no-op）201，GET／成功回放200。

回执固定保存command_id、主体／域／源／action、原键、完整原请求与request_sha256、before／after快照、applied或unchanged、当时策略、UTC完成时间、model_calls=0。当前状态另GET。回放旧archive回执不得将已恢复来源重新标成archived。

摘要编码复用typed-LP规则而非JSON字段顺序：每个token编码为UTF-8字节长度十进制＋冒号＋token原字节。null产生`n`；string为`s,原值`；bool为`b,1或0`；安全整数为`i,十进制`；array为`a,长度,各元素`；object为`o,键数,按键UTF-8字节升序的键token和对应值token`。对整个编码做SHA-256。

- request投影为对象：`kind=knowledge-lifecycle-command/0.7.0`、owner_id、domain_id、source_id、action、三个expected、confirmed、target。target为完整分类对象／`{relative_path}`／状态动作的null。原键已独立参与作用域，不进入摘要，回执仍保存并核对原键。
- position投影为对象：`kind=knowledge-lifecycle-position/0.7.0`、registration_id、relative_path。不基于正文SHA合并；摘要相同但原路径不同视为完整性错误。

POST前先持久化完整恢复槽；槽版本1，含会话scope／主体／域／源／动作／完整原请求／请求摘要／可空command_id，不存原文、令牌或对象地址。每主体／域只保留一份未决请求，单槽≤8KiB、最多8个未决域；容量满或本地存储失败则禁止新POST，不驱逐未知请求。成功后可由下一次明确操作替换本域最近槽，服务器全部回执仍持久保留。

刷新、网络失败、超时及503先GET原键；404／失权不证明此前未执行。核实当前访问后只允许明确重发同body／同key，不后台重试、不自动换键。401、失权404及策略／契约冲突清可见敏感状态但保留主体隔离槽。已知拒绝后先核查原结果，用户明确放弃旧准备后才读新详情、换新键、重新认可。新0.7错误有machine code（code字段）、request_id和outcome_known；其含义是本次尝试的已知拒绝边界，不替代原键结果，错误映射见OpenAPI。

## 6. 统一世代与旧生产者栅栏

三个已有发布核心`publishKnowledgeSource`、`publishOriginalSource`、`publishWebSources`与R6四动作共用私有状态核对／推进。检查必须在unchanged早返回之前。新增状态不是第二个头；元信息修订复用现有`RecordSourceSnapshotObject(s)`，不重写另一套原生版本算法。原件／网页继续没有C1根节点；Web元信息修订保留原Capture／EntryRecord及三表示固定对象。

| 入口 | 私有世代钉选与最终重检 |
| --- | --- |
| C1直接快照 | 调用开始捕获本次来源世代，最终事务重检；已登记text源的请求路径／分类必须等于当前Binding／归类，不得借C1移动／重分类／恢复；original_only／web_capture继续拒绝C1文本 |
| 0.2 | Preview在域锁内固定；确认和unchanged分支前重检 |
| 0.3 | Observation逐项固定；Preview继承，不能重采当前世代；确认再查 |
| 0.4 | 每项从Observation继承；Accept和worker每项实际提交均查，成功项保留；未提交旧世代按既有revision_changed失败 |
| 0.5 | CreateUpload核expected head时固定，接收完成不重采；Confirm再查 |
| 0.6 | **明确创建Preview时固定当前世代**，确认对全部选择一次重检并保持全原子；Observation旧头不强迫重抓，Target刷新／GET／恢复不会重钉旧Preview |
| 0.4新项目清单 | Prepare为各项固定当前世代，Publish核状态与组织位置；旧固定manifest不改 |

私有记录严格区分“没有栅栏的旧准备”“当时无来源”“固定源ID＋世代”。观察时无源、提交时出现新源必须冲突，不能按新SHA认成同源。既存同输入历史ID复用也推进世代，挡住正文A→B→A、路径A→B→A和归档→恢复；旧成功原键先按当前授权回固定旧结果，不进入重新提交。

**C1边界**：旧C1请求没有客户端世代字段或持久准备。因此只能阻止本次调用期间的变化，不能宣称识别客户端此前读到A后，服务端A→B→A的旧意图；需要完整客户端ABA保护的动作使用新0.7或受栅栏的C2确认。本文不为此扩C1 DTO。

0.4新清单仍允许明确选择准备前已存在的历史正文版本，只要其路径／分类与当前组织一致且来源active；Prepare后任何世代变化都需重做准备。归档、移动、重分类使旧基版沿用项失配时拒绝新发布，不自动剔除／修正；既有head及非head存档Payload保持。

## 7. 部署影响，须随本候选一起确认

1. 停止所有仍能写当前头的旧API／batch worker，不能新旧无栅栏二进制同时写。沿用现有Compose／卷／凭据，不删数据。
2. 创建私有生命周期／退役占位／动作回执和准备栅栏数据结构；为现有有效单头来源初始化active／世代1。多头或完整性异常阻止ready，不擅自选头或改历史。
3. **部署前没有私有世代的未消费0.2 Preview、0.3观察／Preview、0.4未成功项及项目准备、0.5 Upload／确认准备、0.6 Preview拒绝消费**，按各冻结版本已有revision_changed等既定错误提示重新明确准备；不加旧字段、不回填“当前世代”冒充旧观察。已经成功的项／回执／manifest保留。
4. 仍有效的旧0.6 Capture／Observation保留内容证据，可在明确刷新Target后新建Preview钉当前世代并重新逐项认可，不再次Acquire；原旧Preview仍不能消费。0.5需新明确会话及完整原件上传，旧对象／历史不GC。
5. 只有迁移和全部拥有路径栅栏就绪，0.7 runtime_ready才true；新API／worker按新栅栏恢复，已成功结果不重施。0.4已受理任务的成功项不撤销，缺栅栏／过时代的未成功项按原状态机失败，不伪造整批成功。

这是新增共享语义及真实部署影响，当前均未执行；确认候选不是工程验收。

## 8. 开发交替与验收

| 批次 | 唯一写入者与范围 | 必须通过的验收 |
| --- | --- | --- |
| R6.0候选 | A文档／契约，消费会话一次集中只读输入 | 20schema／10操作、正反例、typed-LP向量、身份／CAS／原键／栅栏规则、29保护文件；真人具体确认 |
| R6.1底座 | A Go实体／迁移／共用世代／三个发布核心及旧准备 | 初值、safe整数与溢出、旧私有栅栏缺失／无源区分、全部旧生产者双方先赢、C1边界、项目历史沿用、旧DTO及manifest原字节 |
| R6.2动作 | A四动作／位置／回执／分页／当前列表 | 四动作各noop／CAS／授权，active／archived／退役抢位、同SHA不合并、Unicode精确、SQL各阶段失败全回滚、对象GET／PUT均0、历史与保护不变 |
| R6.3客户端 | A交清洁固定提交后，消费会话唯一Web写窗 | service→Hooks→状态页／明确差异确认→原键恢复；默认不认可、冲突不自动追头、主体切换清可见状态、历史回执与当前状态分离 |
| R6.4同版联调 | 清洁回交后A收回，定向修复并发布 | 真实MySQL／既有S3与浏览器、两个API抢位／同键、进程／连接中断、旧pending过渡、旧固定读取／项目／原生保护、模型与旧POC指纹、容量五次，再按具体授权推main |

重点反例：两源抢目标只一胜；源正文／位置／状态ABA均拒绝旧世代；归档后旧任务不复活；原源取回退役位需新CAS；回放旧归档键不重归档；撤权下历史／列表／回执均不泄漏；0.6同Capture新明确预览可继续而旧预览不可；C1无登记不被遗漏；原件／网页不生成C1根；已成功项目存档不重写；新项目发布不静默修复旧组织。

候选性能门槛：同一暖后端含1000个合成来源，重分类／移动／归档／恢复及20项列表各五次，记录中位／最大，单次元信息HTTP最大≤2秒；64MiB原件重分类／移动的对象正文GET／PUT计数必须0。该门槛待实施实测，不是本候选已达标，不外推百万库或生产p95。

## 9. 已复用、待自研与待验证

复用原生登录／私有域、分组与三字段分类、现有二进制位置约束、Document／File／FileCommitItem和已验对象元数据核心、固定阅读与保护、既有客户端分层和主体恢复槽做法。自研统一来源世代、退役位置、四动作固定回执及列表、旧准备私有栅栏、R6状态界面和恢复语义。元信息变更不等于解析／结构／索引完成。

本轮仅候选。迁移、四动作、旧生产者并发、浏览器和性能均NotRun；R5工程人的决定独立Pending，阶段2未退出。之后仍为R7→阶段3结构／代码关系与文档阅读→阶段4可选检索引用→阶段5MCP→阶段6人格记忆。
