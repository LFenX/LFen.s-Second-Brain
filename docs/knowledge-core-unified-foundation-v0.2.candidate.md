# 统一知识基础 0.2：具体接口决定候选

候选 ID：KCF-020-20261030-01。
状态：具体接口与验证范围已形成，待真人决定；当前不是实施契约、最终验收或发布批准。

## 本轮决定的范围

- 用同一知识服务贯通原件／固定修订、C1、R1、R2、通用解析表示、类型节点、分块、词法索引和准确回源。
- 默认关闭解析与检索；仅当前原生登录所有者，固定修订和固定表示，不自动调用模型。
- 手动单文件保存、快照、解析、实际阶段／取消、固定结果选择；任务或写入结果不明时保留原请求，不伪造成功。
- 四类表示可在一份选择、一个索引任务、一次发布和一次检索中混合使用；消费者不分类型自行合并排名。
- Web 使用实际服务字段；MCP 仅提供下列三个只读工具，直接委托同一 DatasetService。

## 已具体形成的接口

统一内容入口为 `/api/v1/datasets/{dataset_id}/knowledge/content`：独立许可、原件保存／快照、解析任务、固定表示发现，以及 representation／node／nodes／read。具体闭合请求、响应及错误见 parsed schema，SHA-256：`85adc864e54d2fa564b5e1ccb398c24644cc995cb25d36ed29fbfa3c882a5948`。

统一词法入口为上述路径下的 `/lexical`：`POST inspect`、`POST indexes`、`POST jobs/get`、`POST search`、`POST evidence/read`。协议候选为 backend/lexical-unified-candidate01/protocol04.json，SHA-256：`811b507e69e6751d17b28ebcd777acdb50c50c47ceb8ae11accdbe8bfc704a6f`；schema SHA-256：`2007881d6653b8f18f5ac8873342f4c5174d4a3ce6440e3687b9a620b763d4c7`。

MCP 使用独立入口 `/api/v1/knowledge/content/mcp`，显式 profile 为 `knowledge-content-lexical/0.2.0-candidate`，请求头 `X-Knowledge-MCP-Profile` 和工具参数 `profile_id` 均须匹配；保留标准 SDK 协议处理。

| 工具 | 同一拥有层方法 | 用途 |
| --- | --- | --- |
| `knowledge_inspect_local_index` | `InspectKnowledgeContentLexical` | 核对许可、选择、覆盖与固定索引状态 |
| `knowledge_search_local` | `SearchKnowledgeContentLexical` | 对同一混合选择执行一次词法检索 |
| `knowledge_read_local` | `ReadKnowledgeContentLexical` | 按固定证据准确回读，并返回已验证的类型信息 |

原件 SHA、阅读文本 SHA、片段 SHA、原件坐标和派生坐标分别保留。读取结果的 `context` 只包含实际节点和片段内完整成员；保留空单元格、合并范围和显式省略原因，不展开父级文本或裁剪表格后宣称完整。

## 已完成的工程验证和当前限制

- parsed 发布／上传已有定向测试和 64 份实际注册路由响应；12 份原创原件已通过实际解析子进程检查。
- 统一词法候选已形成 26 个拥有层文件；已核对 38 份原始 HTTP 响应、190 项 schema 检查和 15 项语义检查；实际 DOCX 空单元格保留。
- 上述新接口验证使用 SQLite／内存存储／ES 替身，不能当作真实 MySQL、MinIO、ES、MCP 和新 Web 联调通过。
- 既有独立运行环境已验证 C1、R1、DOCX、PDF、PPTX 固定读取。原件确认的首次 401 已保留并按原键恢复，原因仍在排查。
- 新 MCP 正在修复实际 DOCX 嵌套定位器的校验问题并完成候选编译，完整响应上限为 1 MiB（包含 JSON-RPC 包装及转义）；Web 正在完成统一检索与手动解析流程的组合验证，并修复历史切换时取消请求及迟到回执的绑定问题。这些修复沿用本页固定接口，不因此放宽结构校验。
- 上传没有按键查询回执；响应不明时不自动重传。跨不完整分段的表格合并范围会明确拒绝。
- 召回质量和 MCP 实际作用尚未评分；独立盲集已封存，先通过完整基础，再执行真实工具调用与独立评分。

## 决定通过后的动作

按固定候选实施消费者及薄适配，修复已发现问题并通过定向验证后，串行核收源码；构建同版二进制，在新的独立合成环境验证原生保存、解析、分块、索引、检索、回源、Web 和两个标准 MCP SDK。基础通过后，再测复杂合成数据召回与主会话逐步选择 MCP 工具的实际作用。任何改变本页固定接口的调整须重新提交具体差异。

本候选不包含模型／OCR／转录、真实 AICS 内容、部门／Grant、临时 MCP Key、原 POC 替换、发布或上线。原 POC 的 19380／19381／18080 和现有卷保持原状。

## 固定决定对象与验收边界

决定对象是本页所述具体接口、同一服务职责及独立合成验证范围；以 subject.json 中逐件 SHA-256 为准。MCP 仅增加 profile_id／operation／path.domain_id 包装，body 和输出 data 分别使用固定统一词法 schema 中对应的请求及结果，不新造业务 DTO。其 schema 引擎投影仅允许等价改写，不改变合法值集合。

决定不表示实现测试、真实联调或质量已经通过。MCP、Web 最终构建包和上传连接修复仍由主控逐件核收；失败原件和证据保留。实现和验证可在批准范围内继续，最终产品验收独立记录。
