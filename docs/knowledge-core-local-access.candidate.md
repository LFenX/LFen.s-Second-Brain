# 本地索引、检索与精确范围授权 0.1 候选

日期：2026-10-28；状态：`Draft_NotFreezeReady`。字段、工具名、行为和错误均为可审阅设计，未批准、未接业务；不修改47冻结件，不删除现有OperationUse/Run检查，不将owner身份代入受限读取来冒充授权。[统一内容候选](knowledge-core-unified-model.candidate.md)负责固定证据和坐标；[原生链路盘点](knowledge-core-retrieval-mcp.md)给当前代码证据。

当前可供实施决定的最小范围另见[首片具体候选](knowledge-core-lexical-slice.candidate.md)：默认关闭的owner词法扩展、固定集合、七项操作及严格schema。本文件的部门共享、任意区间Grant和临时MCP凭据仍为后续设计，不随首片一起批准。

## 1. 本地动作与模型用途分开

共同前提：认证有效、资源有效、签发者当前仍有授权权力、策略允许、scope匹配、未到期／撤销。默认关闭新增能力。新本地入口只进入现有拥有层中可复用的无模型执行分支，不绕过旧SearchDatasets的Use检查。

| 操作 | 候选必要许可 | 不隐含的许可 |
| --- | --- | --- |
| 有界read | read | 整份原件下载、附件本体、模型调用 |
| 本地search | read+search | Embedding、模型重排／生成、全域搜索 |
| 本地index | index，首版限owner／明确索引任务主体 | 修改来源、扩权限、解析新格式、任何推理 |
| 向量入库／查询 | index+model.embedding／read+search+model.embedding | 其他来源向量化或其他模型用途 |
| 模型重排 | read+search+model.rerank | 先读无权候选再过滤 |
| 生成 | read+model.generation，检索生成另需search | 隐含向量化或扩内容范围 |
| 远程解析 | parse.remote及既有来源／转换许可 | 自动回退OCR/VLM |
| Grant签发／撤销 | owner当前权力及管理动作 | 普通MCP消费者扩权或签钥匙 |

read/search/index不触项目远程模型或本地推理。R1/R2独立结构处理开关保留，首片仅消费明确的现有确定性转换结果。原生Dataset即使绑定Embedding，也不为新本地查询自动建encoder；Extractor、metadata、翻译、关键词、编译等增强显式关闭。

## 2. 个人、部门与组织的最小映射

候选采用“原生Tenant与成员为身份边界，域归属及精确共享绑定独立”的语义。一个域仍对应一个Dataset；不新建通用组织树或ACL引擎。

| 情形 | 映射 | 必须核实 |
| --- | --- | --- |
| 已有tenant成员恰为部门协作边界 | 复用UserTenant集合；部门名称作归属说明 | 原生token租户到用户假设；跨部门仍显式授范围，不推导继承 |
| 同tenant含多个部门 | 复用原生用户／成员；最小域共享绑定解析获准成员或委托scope，交Checker | team覆盖全tenant，不能当部门子集；当前未持久化这种域共享事实 |
| 个人域 | 同一内核，默认owner，其他主体须明确授权 | 不因归属同组织、标签或关系而自动开放 |

证据：`internal/entity/user_tenant.go:20`、`dataset.go:109`、`internal/permission/database_source.go:965`、`internal/dao/user.go:49`。当前knowledge_enabled入口仍CreatedBy私有，本文不是部门共享已实现的声明。

## 3. 最小Grant与范围语义

一个Grant对应一个认证主体、一个域、一份不可变授权定义。变更授权以撤销后重签表达；读取时检查当前策略，签发时策略版本只用于追溯。

| 字段组 | 最少字段 |
| --- | --- |
| 身份 | grant_id、principal_id、issued_by_user_id |
| 域／动作 | domain_id、显式actions；首版MCP委托read/search |
| 范围 | scope.mode、固定manifest或明确动态selector；scope_digest |
| 交付／外发 | 明确返回对象／通道；model/egress用途与目标默认空 |
| 有效性 | issued_at、expires_at、revoked_at |
| 追溯 | 策略版本、契约版本 |

凭据另保存credential_id→grant/principal与凭据摘要；不复用owner JWT、宽tenant API token或模型Key。工具输入不能指定其他principal或grant切换身份。临时委托身份与存储所有者分开，现有owner-only读取方法不能通过伪造owner参数来复用授权。

范围规则：

1. 固定manifest列具体来源修订、表示／完整节点或明确字节窗口；不自动获得新版本／新增成员。首条切片优先完整修订和完整节点，任意区间搜索不进入关键路径。
2. 动态selector仅考虑明确当前域、Group或来源集合，include_descendants显式；意味着新匹配内容会进入，签发时须显式告知。动态能力需要独立验收，不能暗含在固定Grant里。
3. 不把旧版本字节窗口自动继承到新修订。R1 source_utf8与R2 derived_utf8分开。一次操作解析固定scope snapshot；续页／引用读取重查授权和版本，失效明确返回snapshot_stale，不悄悄换head。
4. cursor、scope snapshot和evidence_ref绑定主体、域、grant及具体修订，均不是独立授权凭据。父节点、关系、metadata、计数、附件与缓存都受同一过滤。
5. 只有完整证据范围在scope内的检索单元参与召回／输出。授权切开chunk且无范围内独立单元时，明确scope_not_searchable，不返整chunk，也不以空结果冒称完整搜索；有界read仍可使用允许窗口。
6. 归档和撤销分别处理；固定历史能否读取按当前域／资源策略判断，显式撤销立即停止新输出。Grant在召回后撤销，输出前检查仍拒绝。

服务端能控制其向模型／客户端交付的内容；收到文本后的远端Agent后续转发不受本服务强制控制。设计不得把交付许可写成能保证客户端不再外发。

## 4. 三个MCP工具的最小消费语义

名称是候选，不是已存在或冻结工具；三个工具均委托同一知识服务，不提供索引／Grant管理。

| 工具候选 | 输入 | 输出 |
| --- | --- | --- |
| knowledge_browse | 域、可选范围内父节点／来源、有界limit/cursor | 许可内目录字段、固定scope snapshot、允许动作及索引覆盖；计数只覆盖许可内容 |
| knowledge_search | query、scope snapshot或更窄筛选、词法／结构模式、有界limit/cursor | 片段、evidence_ref、固定source revision/representation/node、匹配类型／分数、精确位置、覆盖／就绪状态 |
| knowledge_read | evidence_ref、可选更窄窗口 | 固定修订文本、坐标系／实际范围、源与片段摘要、完成标志／下一窗口 |

不接受owner身份、模型Key、任意model_id或隐含“全部可见库”。tools/list的名称／描述和默认域选择也受Grant约束。只读工具不能因搜索命中自动获得原件或附件下载许可。标准MCP协商、发现、调用和错误／取消继续复用官方SDK，客户端互通按具名版本／传输／认证实测。

## 5. 保持旧契约的接入与影响

| 拥有路径 | 拟议接入 | 保留与验证 |
| --- | --- | --- |
| `dataset_permission.go:20`旧Use/Run | 保持原逻辑；新local动作另行鉴权 | C1及旧REST/MCP的catalogue-only行为不变 |
| 现有Dataset/NLP搜索拥有层 | 抽取复用查询准备／执行，由两入口各自授权 | 不复制第二后端；显式nil embedding/rerank与禁增强 |
| 原生索引任务／DocEngine | 统一表示→原生chunk，携修订／策略／索引版本 | 接收、构建、发布重查；旧任务不能覆盖新头；身份/持久字段方案须补齐 |
| 固定内容读取 | 复用底层校验和字节读取，以新核定scope进入 | 不把存储owner当请求主体；旧错误/固定引用不变 |
| 受限MCP凭据 | 仅进入新工具／受限入口 | 原生宽入口不能接受并提升成owner；撤销影响发现、调用、续读和缓存 |

本候选尚缺：公开HTTP／MCP严格schema及错误结构、索引行身份与knowledge_ref持久映射、授权持久事实及原生Checker装配的精确实现影响。这些由主控继续准备，完成后才交具体冻结，不把本设计文档的保存视为批准。

## 6. 待执行正反例

| 输入 | 候选预期 |
| --- | --- |
| 仅read请求窗口／请求search | 前者成功，后者拒绝 |
| read/search域已绑Embedding | 仍纯本地；任何模型或推理尝试为失败 |
| 授权[100,200)，读[150,190)／[90,110) | 范围内成功，越界整请求拒绝 |
| chunk证据[0,512)，仅授权[100,200) | 不返整块；无独立单元则scope_not_searchable |
| 固定R1授权后来源更新R2 | 不自动取得R2 |
| 移出动态Group后使用旧cursor | 拒绝或scope snapshot失效 |
| 召回后、输出前撤销Grant | 不返回正文、目录、聚合或引用 |
| 使用他人cursor/evidence_ref | 拒绝跨主体读取 |
| 本地工具请求rerank_id/生成开关 | schema拒绝 |
| 仅read取得附件引用后请求本体 | 无单独许可则拒绝 |
| owner使用旧冻结读取 | 原契约、修订和错误保持 |

上述用例均NotRun；未来工程验收另需实际持久索引、固定引用逐字节、权限与撤销竞争、项目模型／本地推理0、Web与MCP同服务结果。不能用此表或Mock冒充真实通过。
