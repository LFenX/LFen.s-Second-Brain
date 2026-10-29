# 无模型知识检索首片候选

日期：2026-10-28；候选ID：`KCLS-20261028-01`；状态：`ReviewCandidate_ImplementationDecisionPending`。本文件由主会话统一维护；是可供决定的首片实施候选，未接业务、未改变权限，未批准模型或部署。完整模型与部门共享仍按各自范围推进。严格字段见[候选 schema](knowledge-core-local-lexical-v0.1.candidate.schema.json)，身份、配置、持久字段及合成向量见[机器候选包](knowledge-core-lexical-slice.candidate.json)。

## 1. 首片行为

在现有 RAGFlow 的 Go 知识服务内，增加默认关闭、由域所有者显式启用的本地词法能力。沿用原生 Dataset、Document、FileCommit、不可变来源修订、Tokenizer、indexdoc、Elasticsearch 与 NLP Retrieval；Web 和 MCP 消费同一拥有层，不新建第二套检索后端。

| 项目 | 本片具体范围 |
| --- | --- |
| 身份 | 一个原生知识域；当前域所有者及有效原生成员身份；每次请求重新校验 |
| 内容 | 显式指定固定来源修订与表示；不隐含全域、最新头或动态范围 |
| R1 单元 | 已发布 `structure_view` 内的既有 `passage`，读取其精确 `source_utf8` 范围 |
| C1 单元 | 已有 text file 根，完整原字节不超过既有 `PassageMax=8192` 时作为一块；不是原件专用的二进制来源；超限明确拒绝 |
| 文本处理 | 不重新分割、合并或修饰这些单元；真实固定范围与原文逐字节核对后进入原生 `ChunkDoc` |
| 索引与查询 | 仅 Elasticsearch 与 `full_text`；模型对象为 nil；词法权重明确；禁模型增强与远程取图 |
| 回源 | 绑定固定 source/revision/representation/node/UTF-8 范围与摘要，输出前重新授权 |
| 后续能力 | 部门共享、精确区间 Grant、临时 MCP 凭据、R2 回接、向量/重排/生成分别推进；本片不宣称完成 |

R1 的 passage 已由当前 producer 在结构/行界与 UTF-8 边界下产生，`PassageTarget=4096`、`PassageMax=8192` 是已有常量。复用这项事实不代表本轮测出了最优分块参数；类型感知分块和内容提取的质量仍须独立验。依据：[生成单元](../internal/parser/structure/build.go)、[已有界限](../internal/parser/structure/types.go)。C1 的有界根适配是本候选新增取舍，未冒充现有索引行为。

## 2. 原生链路的具体接线

1. 在现有 Dataset 知识拥有层解析当前身份、独立本地策略与显式固定选择，核对既有修订/表示/节点；客户端不能提交可信正文、节点身份或授权声明。
2. 从既有固定原文读口获取精确单元，验证字节范围、UTF-8 边界和摘要。将最终原文作为唯一 `ChunkDoc.Text`，由纯 `full_text` Tokenizer 产出词法字段。
3. 新索引行使用单独命名空间的完整绑定身份，区分来源、修订、表示、producer、分块计划及成员位置；旧原生 ID 和 C1/R1/R2 节点 ID 保持原义。
4. 索引边界显式持久 `knowledge_ref` 的规范 JSON 字符串及完整绑定过滤键。现有 ES `GetFields` 不保证对象原样返回，不能直接塞一个 map 或塞入会被删除的 metadata。
5. 新知识行始终保持 `available_int=0`。只发布完整写入、核对行集合且引擎可见的不可变绑定；失败/未完成行不进入查询集合。查询使用完整绑定集合及允许行集合，不能以独立 revision 列表和 representation 列表产生交叉组合。
6. 新入口在当前拥有层授权后复用 `RetrievalService.Search` 的原生查询构造及引擎词法排序，显式关闭模型增强；不通过旧 `SearchDatasets` 入口绕过 `OperationUse` 闸门，不另写检索引擎。
7. 首片不物化 `mom` 父正文，不自动调用 `RetrievalByChildren`。语义父节点继续通过固定 R1 读取；普通原生检索与父正文机制不改。
8. SQL 每行清单绑定 `row_id→引用摘要/正文摘要/字节长度/固定成员`。写前与读取均核对，防同一 binding 内 A 行借用 B 节点引用或正文；不一致即报告证据失配，不悄悄丢行后返回“检索完整”。

新行使用 create-only，冲突仅在完整清单等价时视为幂等。普通写入和状态修改不能覆盖或激活该类行。已核实的两处接线要求：ES 旧 `Filter[available_int]=0` 实际是 `<1`，新用途须构造严格 `term=0`；文档启用 helper 当前会把无 `compile_kwd` 的新行当普通来源行，须读取并排除知识绑定标记。旧普通过滤语义不改，也不靠 ID 长度区分新行。以上均是实施及真实验证要求，尚未实现。

原生边界事实见 [indexdoc](../internal/ingestion/task/indexdoc/process.go)、[父正文物化](../internal/ingestion/task/indexdoc/parent_child.go)、[ES 字段读取](../internal/engine/elasticsearch/chunk.go)、[NLP 检索](../internal/service/nlp/retrieval.go)。新增字段、发布记录和路由仍是拟实施内容。

## 3. 策略与旧契约关系

独立本地词法策略默认关闭，带单调策略版本；明确启用后才允许新入口执行 index/search。旧 C1 catalogue-only 策略、旧原生模型/入库闸门和 R1 不可变 view 中创建时的 `not_created` 保持原义。当前索引 readiness 使用新增状态载体，不能改写旧 view，也不能用旧 catalogue 字段隐瞒新能力。

索引就绪、覆盖缺口、陈旧、失败与“就绪但零命中”分别表达。显式固定修订不会在来源更新后自动换成新版本；历史修订的可访问性仍服从当前生命周期与策略。范围、身份或策略发生变化时，请求必须重新核定。

现有统一模型草案中的 `parser_result` 只代表解析草稿，不是本片的持久引用；R2 的派生 UTF-8 和自然定位也不被改成原文字节范围。整个统一模型、部门授权与 MCP Grant 不因本片候选而一起冻结。

## 4. 具体接口与身份

公共前缀候选：`/api/v1/datasets/{domain_id}/knowledge`。严格 schema 的 operation/path/body 是离线校验信封；实际 HTTP 用路由参数和请求体，不要求客户端再套一层。

| 操作 | 路由后缀 |
| --- | --- |
| 查看独立策略 | GET `/local-lexical-policy` |
| 显式修改策略 | POST `/local-lexical-policy/changes` |
| 检查固定集合就绪状态 | POST `/local-index/readiness` |
| 创建索引任务 | POST `/local-index/jobs` |
| 查看任务 | GET `/local-index/jobs/{job_id}` |
| 查询固定发布集合 | POST `/local-search` |
| 回读整个固定单元 | POST `/local-read` |

策略修改和建任务带预期版本与幂等键，同键异载荷拒绝。每个固定集合 1–32 来源，每个来源只选一个修订/表示；旧修订单独查询，不自动换最新。搜索显式给 `index_ref+selection`，read 显式给完整证据；引用不产生授权。新 read 只读整个 ≤8192 字节单元，旧大范围读口不改。

状态区分 unindexed/building/coverage_gap/stale/failed/ready。覆盖完整与返回全部命中分别表达；仅成功实际检索的零命中才返回空 hits。首片不承诺翻页或总匹配数。query ≤4096 UTF-8 字节；一次最多8条全文命中，是8192×8的负载上限，未作召回质量调优。错误、返回对象和关闭未知字段的约束以 schema 为准。

`binding_id` 和 `index_row_id` 是带命名空间、类型标记和 uint64be 长度的 UTF-8 编码后 SHA-256，均为64位小写十六进制；不产生语义节点。binding 绑定完整来源/修订/表示/producer、单元策略与文字存储摘要；row 绑定单元成员/位置/正文。公共 `profile_sha256` 是 C1/R1 各策略 plan 摘要的聚合，不能把两种 plan 强作同一值。单元只用一个成员、无 parent/mom；紧凑存储引用不带实验多成员的 output_range 扩展。

语言绑定可信 dataset.language 原值，允许原生空字符串默认值；不为哈希将其改写成 English。tokenizer producer 须由实施端提供可信实际摘要，不能拿样例占位值运行。`index_ref` 的 SQL 发布身份/manifest 摘要及 selection 摘要由服务签发，客户端不自行构造权威发布记录。

## 5. 已完成与未验边界

三名子代理均已交回研究写窗。主控独立复跑并完成跨产物核对：

| 检查 | 实际结果 | 证明范围 |
| --- | --- | --- |
| 身份/范围候选 | 36/36 | 重复位置、跨修订、摘要与成员约束；39事实为人工预标 |
| 首片精确单元 profile | 11/11 | 4原件、C1/R1合成占位引用、单成员原字节及策略隔离 |
| 严格接口 a04 | 96/96 | 完整 Ajv Draft-07 schema、有限约束向量和合成字节摘要 |
| 持久字段 a02 | 15/15 | Node 序列化、内存 mapping、引用/正文/范围清单约束 |
| 主控交叉映射 | 11/11 | 身份向量→紧凑引用→a04公开 EvidenceRef/ReadResponse、聚合profile与空语言编码 |

重复运行不合并计数；首轮接口88/89的真实失败、被纠正的 q_0_vec 误判、旧 available=1 示例和主控工具/衔接错误均在[本轮记录](knowledge-core-execution-record.json)保留。机器候选包中的来源/节点/表示与 producer 是合成占位，未实际发布。

Go 草稿未编译；真实 Tokenizer、SQL、对象存储、ES mapping/写入/查询/发布/写隔离、Web/MCP、权限集成、性能和检索质量均未验。候选完整不代表上述实现已通过。47保护件、源码指纹与 LICENSE 不变；没有模型/推理/真实资料外发、提交、推送或部署。

## 6. 实施交接与需要的决定

建议按本候选进入 owner-only 无模型业务实现和合成验证。新增 SQL 发布/任务/策略记录及 ES 两字段是本片影响；遇已有字段类型冲突，交主控处理，不删除索引或卷。原始结构、47保护契约、旧域/模型/入库闸门保留。

持续开发采用独立会话：后端负责同一服务及定向验证；Web 消费已固定 DTO；MCP 先准备同服务适配/真实身份核对/具体工具候选和具名客户端验证，受限 Grant 不在本片。主控逐次分配共享源码写窗并统一集成。真实引擎闭环后才能报告 Web/MCP 联通，不以 Mock 或离线结果代替。

实施验收至少覆盖：三类合成来源的真实保存/单元/索引/回源；新旧修订和换 plan 不覆盖；部分失败与重试不半发布；逐行引用和原文完全一致；启停文档不激活新行；原生普通写入不能覆盖新行；当前身份/策略变化立即限制 index/search；错误状态不冒充零命中；全流程模型和远程抓取尝试为0。质量参数另用样本讨论。

人的接口/实现决定为 `Pending`。此处请求的决定只涵盖本片实现及合成验证，不等于整个统一模型冻结、部门/Grant授权、模型调用、当前POC替换、发布或最终验收。
