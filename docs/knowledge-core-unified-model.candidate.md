# 统一内容基础 0.2：当前结构与实施边界

日期：2026-10-29。状态：固定 parsed 表示、类型分块、显式上传和统一读取已完成工程实现与定向验证；统一新检索、MCP 和新运行整体尚未贯通。版本为 `0.2.0-candidate`，没有据此冻结接口或批准业务接线。最新源码为4999份，SHA `378f5cd7c4da198aa153a7beb49313034c151529b17a11fcf61dbce55ed4f8bb`。执行与证据见[主线记录](knowledge-core-execution-record.json)的 `foundation_continuation`。

## 当前数据架构

```mermaid
flowchart TD
  A[当前访问身份与知识域] --> B[原生 Document / File / FileCommit]
  B --> S[固定 SourceRef：来源 + 修订 + 原件 SHA]
  S --> C1[原文 C1：source_utf8]
  S --> R1[代码与文本结构 R1：source_utf8]
  S --> R2[文档结构 R2：derived_utf8]
  S --> P[已有本地解析器：可信任务与 owned child]
  P --> T[临时类型结果：item_utf8]
  T --> K[绑定 producer / 类型分块 / 原子固定发布]
  K --> PR[parsed 表示：derived_utf8]
  C1 --> U[同一 DatasetService 的四个统一读取]
  R1 --> U
  R2 --> U
  PR --> U
  U --> W[Web：C1/R1/DOCX 已真实验证]
  U -. 新 parsed 界面待接 .-> WP[手动上传 / 解析 / 固定结果]
  C1 --> L[既有本地纯词法核心]
  R1 --> L
  U -. 四类混合检索候选待贯通 .-> L
  L -. 新0.2只读消费待接 .-> M[MCP：检查索引 / 搜索 / 证据回读]
```

原件和修订继续由原生文件链拥有。表示是固定内容的读法；索引是可重建派生物。统一投影不复制原件，不改旧 C1/R1/R2 身份算法。临时 parser item 不能直接成为持久节点或索引证据。

| 持久层或投影 | 实际拥有对象 | 当前职责 |
| --- | --- | --- |
| 原件与文件关系 | Document、File、File2Document、FileCommit、FileCommitItem | 保存原字节及文件归属；上传不自动启动解析 |
| 来源与修订 | `knowledge_source_revision`，及 lifecycle / fence | source_id 使用原 Document ID；revision 使用原提交项；原件 SHA 和当前性分开 |
| C1 节点 | 现有 `knowledge_node` 根与子节点，使用确定性 ID | SourceRevision.RootNodeID 是响应投影字段；不能因此说 C1 节点不存库。统一层不复制现有节点存储 |
| R1 结构 | `knowledge_structure_view/node/relation` | 项目固定 view 按来源绑定；节点保存原文区间 |
| R2 与 parsed | 共用 `knowledge_document_representation/node` | 固定 payload、reading_text、manifest；protocol 区分 `document_structure/0.1.0` 与 `content_parser/0.2.0` |
| 表示头 | `knowledge_document_head` 继续由 R2 拥有 | parsed 发布不覆盖 R2 head；parsed 发现只列一个完整固定 SourceRef 的已发布目标 |
| 任务与权限 | 现有 structure task / quota / lease / operation，加 `knowledge_content_policy` | 同一任务执行与回收路径；独立默认关闭的本地解析/词法开关；没有模型或部门授权 |
| 本地索引 | 既有 `knowledge_local_lexical_job/publication` 与 ES | 新0.2四类混合消费仍为候选；不建立第二套引擎 |

## 实际类型

类型拥有路径为 `internal/entity/knowledge_content.go`、`knowledge_content_validation.go`、`knowledge_parsed.go`、`knowledge_content_task.go`。可空字段保留键，以 `null` 表达未知。

| 类型 | 关键字段与规则 |
| --- | --- |
| SourceRef | `domain_id/source_id/source_revision_id/source_sha256`；SHA 始终标识原件字节 |
| RepresentationRef | 固定 `source_revision/structure_view/document_representation/parsed_representation`；C1 producer 为空，其余固定表示绑定 producer SHA |
| Target | 完整 SourceRef + RepresentationRef；外部固定读取不接受临时 parser_result |
| NodeRef | 固定 `c1_node/r1_node/r2_node/parsed_node` 使用固定 ID；临时 `parser_item` 使用 item_index，不能伪装固定节点 |
| Address | Target + NodeRef；不得混合来源、修订、表示或节点命名空间 |
| Representation | binding、source、表示引用、根、state/reason、coordinate_space、node_count |
| Node | ref/parent、kind/name/ordinal、locator、original_position、coordinate_precision、text、attributes、attachment_ref |
| Text | range/own_range/status/reason/value；半开 UTF-8 字节区间，原件 SHA、阅读文本 SHA 与片段 SHA 分开 |
| Attributes | 闭合 source/structure/document/parser 四臂；表格几何、sheet、positions、code、alt 等按真实 producer 保留 |
| Parsed 持久载体 | 原件身份 + producer + 阅读文本摘要 + 定序 manifest + 类型 payload 摘要；新身份采用显式0.2命名空间 |
| Relationship | 已定义固定两端地址；统一关系读取未接，原关系 API 仍由原拥有层处理 |

TableData 保留表头、空单元格、Grid、RowSpan、ColSpan 和 Covered。OriginRow/OriginCol 是不序列化的内部诊断值，不能作为持久几何的必填依据。producer positions 原值保留，允许数组或 null；没有依据时不能把它解释成物理页或原件字节位置。

## 坐标与支持状态

| 坐标 | 使用范围 | 不代表的事实 |
| --- | --- | --- |
| source_utf8 | C1/R1 原始 UTF-8 字节 | 二进制原件文字或 DOCX 物理页 |
| derived_utf8 | R2/parsed 固定阅读文本 | 二进制原件字节偏移；自然位置与精度另列 |
| item_utf8 | 单次 ParseResult 临时元素 | 固定节点、原件坐标或发布权限 |

表示区分 ready、empty、partial、unavailable、normalization_required；文字区分 available、empty、unsupported、unsupported_or_requires_ocr、unavailable。空 CSV 无输出不能成功发布为空；显式空 JSON 可保留为空表示。图片 alt 不是 OCR，音频配置不是转写。

Markdown/JSON/Text 由单一 `parser.NormalizeParseResult` 归一化，ingestion component 委托同一函数；失败、无输出、fallback_text 不能充当可信发布成功。parsed 阅读串使用明确的非空 item LF 组装，类型分块保留表格行/合并关系、代码和标题边界，并保护 UTF-8 与 CRLF。分块不裁切表格单元格来制造成功。

新本地解析原件最多2 MiB，派生阅读文本最多4 MiB，固定节点最多32768。统一读取每窗口最多65536字节；列表请求上限100，拥有层有效页最多20，0.2游标绑定完整目标、父节点与原请求 limit。读取续接按真实字节复原；不把分块边界规则误写成每个阅读窗口都不拆 CRLF。

## 当前解析器登记与接入差距

`internal/parser/parser/catalog.go` 是 GetParser 与能力发现共用的唯一登记表，登记16类。登记不等于提取成功、索引就绪或质量达标。

| 文件家族 | 当前路径与输出边界 | 当前证据 |
| --- | --- | --- |
| PDF、DOCX、PPTX | 固定 R2 本地 profile；不为新 parsed 再开一套解析路径 | DOCX15节点，PDF6节点和PPTX8节点/隐藏标记在旧隔离运行版完成准确读取；75份PDF/PPTX请求保留两次首次确认401，不能外推全面质量 |
| XLSX、XLS、CSV | 已有表格 producer；TableData、sheet、空格与合并几何按实际结果保留 | 本地 child 测试含 XLSX、真 BIFF8 XLS；新上传工程 HTTP 探针实际覆盖 XLSX |
| HTML、Markdown | 已有结构 producer；文字、表格、列表/代码等只按实际产生的 item 保留 | child 窄测覆盖；HTML 新上传工程探针实际给出 positions:null |
| TXT、JSON、Email | 文本/记录/邮件头与正文；Email 本地 profile 不递归附件 | child 窄测覆盖；JSON 对象、数组、JSONL 的大整数和长小数保真已单独复现并修复，5项目标测试通过 |
| EPUB、DOC、PPT | 现有本地 producer；EPUB 按 spine、DOC 读取原生文字；普通 PPT producer 表格文字折叠到逐页文本 | EPUB、真合成 OLE DOC 有 child 证据；PPT 仅以 OOXML 容器走登记路径，不宣称真二进制 PPT 已验证 |
| 图片 visual | image_payload 与 alt | 未启用 OCR 或全文提取 |
| 音频 aural | configuration_only | 未启用转写 |

以上新 parsed 工程复用现有已登记格式，不添加新格式能力。视频不在这16类普通 parser 中，当前统一知识路径没有接通视频处理。逐格式语义保真、复杂数据召回与工具效果仍需后续实际验证。

## 当前读取方法与 HTTP

同一 DatasetService 提供 GetKnowledgeContentRepresentation、GetKnowledgeContentNode、ListKnowledgeContentNodes、ReadKnowledgeContentNode。每次核对当前原生签名 session、owner/成员/域、固定来源与完整性，输出前再核；API token 不能代替当前用户主体。

候选空间为 `/api/v1/datasets/{dataset_id}/knowledge/content`：capabilities、representation/node/nodes/read；新增 policy、parsed-source/upload、parsed-source/snapshot、parsed-source、parsed-task（创建）、parsed-task/get、parsed-task/cancel 和 parsed-representations 的显式操作。除 GET 和 multipart 文件部分，请求使用显式 `contract_version=0.2.0-candidate` 和闭合 JSON；保留所有可空键，拒绝未知/重复字段、坏 Unicode 和混合身份。

multipart 上传只接受一个 request JSON 和一个 file，原件2 MiB、总包2 MiB+64 KiB。保存原件、快照、手动解析、建索引和检索是独立状态。普通 upload 与旧 C2 协议的限制保持，新增入口在现有 DocumentService 内复用存储、文档计数、文件关联和回滚。

parsed/upload严格设计候选位于 `docs/knowledge-core-content-v0.2.parsed.candidate.schema.json`，SHA 为 `85adc864e54d2fa564b5e1ccb398c24644cc995cb25d36ed29fbfa3c882a5948`；它描述本0.2内容/上传协议，不包含尚未完成的新词法与 MCP 业务契约。

## 实证与接续工作

| 证据层 | 已完成 | 不能推出 |
| --- | --- | --- |
| parsed 发布代码 | 40路径、159项分层定向检查；窗外/保护件/index独立核验 | 整体新运行或新索引完成 |
| 上传闭环工程 | 7路径、21项共享输入窄测；真实注册路由/签名认证/owned child，SQLite+内存对象存储64份响应 | MySQL/MinIO/NATS部署验证 |
| 独立 schema | 424项离线检查；64份实际 Go HTTP 响应全量交叉通过 | 人的接口冻结或质量验收 |
| 旧隔离运行版 | C1三原件、R1的MD/TSX、DOCX真实C2/R2读取与浏览器验证 | 新 parsed 或新 MCP 已部署 |
| 新二进制 | 4999/d3cc 源已构建并完成四模式 help；随后 JSON 精度修复为4999/c9f5并通过5项解析器窄测 | 修复后的最终运行版本已构建/替换；不得混写版本 |

架构页面已完成真实中文滚动与来源回读。真人批准的具体0.2对象为KCF-020-20261030-01，须通过foundation-interface-decision读取原决定及绑定subject；本文广义Draft_NotFreezeReady状态不变。同一核心四类混合检索26路径已合入，复用已验3264同字节输入；上传连接7路径修复已合入并保留离线红绿与53拥有层检查；MCP三只读10路径已合入并通过准备版14Go检查；Web最终17路径已合入，新原生基础与质量未通过。

模型调用、真实 AICS 外发、部门/Grant、临时 MCP key、原 POC 替换和发布不在本次扩权内。旧首片 a04、47保护件和既有批准3件保持原字节；旧设计与证据随参考基线归档。

当前Web仍从原schema路径直接导入b0，用于已存在的C1/R1/R2消费者。该文件只登记为code_dependency；新parsed设计按独立85adc条目读取。新消费者整体接线前，不把85adc替换到旧消费者构建输入。

当前基础探针：12份原件经实际owned child重验通过；邮件附件排除、XLSX及BIFF8 XLS合并几何由8个拥有层路径修复，首轮失败证据保留。原有类型化TableData保存跨度/covered/空格；完整片段无法承载的合并范围和损坏元数据明确拒绝。CSV/TXT派生LF是既有规则，原件SHA/字节独立保存。此结果是解析基础检查，不是新运行联调或召回质量验收。
<!-- reviewed-source-current -->
Web手动原件/快照/解析/取消/固定回读和四类统一词法消费的17最终路径已独核并合入；历史切换与终态再解析回归关闭。102测试、38原body/status回放、lint/format通过，类型基线207且新增0；2025构建输入和1673产物131830790字节逐件摘要核实。原架构/滚动和旧schema字节保留。统一26、上传7、MCP10及Web17已组合源码，最终Go构建、真实MySQL/MinIO/NATS/ES/Web/两个标准SDK尚待；先基础后复杂召回及独立盲集，质量未启动。
