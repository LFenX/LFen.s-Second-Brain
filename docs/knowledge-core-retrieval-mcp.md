# 当前原生检索、MCP与权限链路

日期：2026-10-28；主控复核后端 snapshot06 + Web/MCP交回，源码指纹 51397b380ba4ed87729467a9d2d1a9cedf6f098a2e281c22fe615356e9f01f0d（4953份）。旧研究原字节见[基线03快照](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-03.json)。本文件区分当前源码、早期组件证据与后续真实服务证据。

## 1. 已实现的本地词法链

当前真实session → Dataset知识拥有层 → 明确固定1–32来源/修订/表示 → R1已发布passage或≤8192字节C1完整文本根 → 显式full_text Tokenizer（模型nil）→ indexdoc → Elasticsearch create-only → refresh及逐行全集核对 → SQL CAS发布 → 原生RetrievalService.Search词法排序 → 同一固定单元回读。

七项HTTP后缀已在Go路由注册，统一前缀 /api/v1/datasets/{domain_id}/knowledge：

| 方法 | 后缀 | 拥有方法 |
| --- | --- | --- |
| GET | /local-lexical-policy | GetKnowledgeLocalPolicy |
| POST | /local-lexical-policy/changes | ChangeKnowledgeLocalPolicy |
| POST | /local-index/readiness | InspectKnowledgeLocalIndex |
| POST | /local-index/jobs | CreateKnowledgeLocalIndex |
| GET | /local-index/jobs/{job_id} | GetKnowledgeLocalIndexJob |
| POST | /local-search | SearchKnowledgeLocal |
| POST | /local-read | ReadKnowledgeLocal |

新入口只接受真实原生签名session；AuthenticateKnowledgeLocalSession返回的context携带会话校验证明，适配器必须保留。每次检查当前用户有效状态、owner和活跃成员；任务执行、发布事务及输出前复核。API/beta token的TenantID→User.ID假设、self-host配置用户和客户端自报owner均不能替代主体证明。

新索引行始终available_int=0；新检索增加严格term=0、完整published binding和授权row预过滤。引用逐行绑定source/revision/representation/node/range/摘要，不进入普通metadata。旧available=0仍为lt1，旧普通搜索和模型/入库守卫不改。read按foundation owner权限，关闭本地词法许可不撤销其已有原文读取权。

unindexed/building/coverage_gap/stale/failed/ready分开。历史固定选择不自动跟随最新头；历史/混合元数据不等同stale。published job的ready描述发布任务结果快照，实时许可/可搜索性以inspect和每次search检查为准。

## 2. 原生广义路径仍保留

常规上传保存、IngestionTask→Parser/Chunk/Tokenizer→DocEngine、SearchDatasets、重排/生成与父正文路径均保留；本片不通过旧SearchDatasets绕过OperationUse/Run→generation。Tokenizer默认包含embedding，省略模型名或向量权重0本身都不能证明零模型。只有本片显式配置链及实际测试支持当前零调用结论。

原生MCP服务端在internal/mcp，go.mod锁官方Go SDK v1.8.0；现有 /api/v1/mcp 与独立self-host listener的认证、旧ragflow三工具和动态目录描述未改变。外连MCP管理/utility客户端是系统访问外部服务器，不能冒充知识服务的入站授权。

## 3. MCP/Web当前阶段

新知识MCP已从v2及S01局部转接接入同一API listener的 POST /api/v1/knowledge/mcp，只有 knowledge_inspect_local_index、knowledge_search_local、knowledge_read_local 三只读工具，静态发现，不注册策略修改或索引创建，不新建后端/listener。该路由已注册到DatasetsHandler.KnowledgeMCP，复用同一个私有DatasetService；每次stateless请求结束关闭传输，无新增listener或第二后端。

Web已按唯一11路径租约应用并停写交回；实际工作区23项Mock/jsdom测试、lint/format、零新增类型诊断和完整Vite生产构建通过，既有全量类型基线207项。Web直接消费同一七项Go HTTP接口，固定选择、会话/域隔离、原幂等请求恢复与准确引用/纯文本回源已实现；真实浏览器/签名HTTP已按固定3来源完成启用、建索引、检索和准确回源。MCP实际接入后的15项协议/handler测试、最终Go构建和help通过；官方Go与TS SDK均已在专用原生实例完成实连；三路源码窗均已交回。

实际客户端为官方TypeScript SDK 1.30.0与Go SDK v1.8.0 Probe；握手、发现、同修订三工具、nonowner拒绝和篡改引用错误通过，14次工具调用与HTTP固定引用/原字节相同。原生会话登录旋转、旧session拒绝及注销后两SDK初始化401已观察；取消未测。Go首试2026-07-28被拒，回退2025-11-25成功；不宣称全版本兼容。Inspector未安装，不宣称任意Agent或OAuth-only客户端已兼容。部门/Grant/临时Key仍为后续设计。

## 4. 实际证据与未验

同一4953份源码和最终Go二进制已在全新合成实例运行：原生签名session七项HTTP、真实C2/MinIO保存与R1任务发布、Web浏览器及官方TypeScript SDK 1.30.0 / Go SDK v1.8.0三工具业务通过。后端共留存173份HTTP响应，其中74份按批准a04完整校验；MinIO另有8次S3对象GET，4修订的导入对象和不可变快照字节相同；MCP另有14次工具调用、44个HTTP状态观察（含空响应）。浏览器固定3份来源覆盖1580/1580字节，MD/TXT/TSX均准确回源。后置43响应验证非owner/无会话七操作拒绝、篡改/参数/CAS/幂等及启停后旧索引失效；A已恢复local policy 3并重建ready，B保持浏览器所建policy 1/ready。

故障后重试不半发布、普通写入及文档启停隔离、换plan和成员撤销证据来自真实MySQL/Elasticsearch组件层，未伪称HTTP故障注入。R1真实任务报告模型/本地推理/外抓为0，词法拥有层显式模型nil且受测尝试为0；运行API只有internal网络且无模型/代理环境变量，不声称已测量全局进程计数。取消、任意客户端/协议版本、HTTP成员撤销、销毁后耐久性、质量/性能仍未验；Go实际回退2025-11-25成功，不支持其首试2026-07-28。模型增强、部门/Grant/临时Key、R2回接、当前POC替换和发布不在本片。

执行记录backend_component/web_component/mcp_component保留组件观察时点与方法，后续实际证据统一见live_integration及9条逐项结果。旧R1/R2结果不改；最终人的验收仍Pending。
