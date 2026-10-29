# 当前代码、依赖与去留事实

归一化续作最新事实：当前4968份源码，指纹`9c35bcc0b484118e5d36206c25988c05e6ba914aaf6a38ea4ff5e14b04a53675`；JSON/Markdown/Text共用NormalizeParseResult，原component重复Markdown归一化已删除，12项共享源码定向检查通过。下列4966为前一内部/HTTP观察时点，其读取类型和schema保持不变。

2026-10-29基础续作当前事实：源码4966份，指纹`8ff351cec9ffee83f69cf3cc263a18ddb3b5310516d4ef7936ab349dca470217`；新增统一强类型、C1/R1/R2同服务四读取、ParseResult临时转换、16类单一parser登记与独立0.2 HTTP读取。内部28项、parser注释后20项、HTTP4项和真实Go返回/schema20项定向检查通过。固定parser发布、类型分块、R2检索和新版本真实消费者尚未贯通，质量未测。以下2026-10-28计数、运行版本和后置项保留首片时点；当前工作次序读取foundation-current和计划第11节。

日期：2026-10-28；主控复核版本：后端 snapshot06 + Web/MCP租约交回。Git HEAD 7233c495311f1c40c67254582138169d5ed0e205，分支 codex/stage3r2-document；4953份源码指纹 51397b380ba4ed87729467a9d2d1a9cedf6f098a2e281c22fe615356e9f01f0d。这是已构建的工作区版本，尚未替换当前POC。旧研究版本原字节已保存在[基线03快照](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-03.json)，不再作为当前开发指引。

## 1. 当前实现

KCLS-20261028-01 后端已实现并完成组件验证。旧 C1/R1/R2 创建时的 catalogue-only / not_created 字段保持原义；新增本地许可与发布载体表达 opt-in 词法能力。Web已应用11路径并完成组件/完整构建验证；知识MCP已在同一API listener接入三只读工具并完成组件/构建验证，两具名标准客户端实际三工具业务、拒绝及篡改错误已验证。人的最终验收仍Pending。完整当前事实见[执行记录](knowledge-core-execution-record.json)的 backend_component。

| 拥有层 | 当前路径与职责 | 已验证边界 |
| --- | --- | --- |
| 原文/修订/权限 | internal/service/dataset/knowledge.go、knowledge_source.go及现有生命周期实现 | 复用原生owner、活跃成员、固定修订和FileCommit；不重写旧ID |
| 新七项操作 | internal/service/dataset/knowledge_local_lexical*.go、internal/handler/knowledge_local_lexical.go、internal/router/knowledge_local_lexical.go | 默认关闭；仅真实原生session；索引/搜索需独立许可，回读按foundation owner权限 |
| 策略/任务/发布 | internal/entity/knowledge_local_lexical_store.go、internal/dao/knowledge_local_lexical.go | 四张独立SQL表；CAS、幂等、任务租约、不可变发布及逐行清单；不声称跨库事务 |
| 分词/投影 | internal/ingestion/component/tokenizer.go、internal/ingestion/task/indexdoc/process.go | 显式full_text、模型nil；使用已有R1 passage或≤8192字节C1完整文本根，不再split/merge或物化parent/mom |
| 词法引擎 | internal/engine/elasticsearch/knowledge_local_lexical.go、chunk.go；internal/service/nlp/retrieval.go | 原生Search及词法排序；knowledge_ref字符串和knowledge_binding_kwd；create-only、完整行集合核对、exact available_int=0 |
| 普通写/启停隔离 | internal/engine/elasticsearch/chunk.go、internal/service/document/wiki_refresh.go | 普通bulk脚本原子拒绝覆盖知识行，批量更新/文档启用排除知识标记；旧available=0的lt1语义保留 |
| producer | internal/tokenizer/producer.go、tokenizer.go | 实际二进制及已加载rag/wordnet/opencc资源摘要；语言取dataset.language原值，nil保留空串 |
| 原生服务 | cmd/ragflow_server.go；internal/server/api/server.go | API启动显式任务runtime；API/Admin/Ingestor/Syncer保留；已在专用internal网络合成实例运行API/Admin，未替换原POC |
| Web | web/src/pages/dataset/knowledge-catalogue/local-lexical*.tsx及interfaces/services/hooks/utils对应文件 | 真实源码已应用；固定选择/六态/幂等重试/准确全文与引用；23项Mock/jsdom与完整生产构建通过，浏览器默认关闭/启用/固定索引/检索/三类准确回源已验证 |
| MCP | internal/mcp/knowledge.go、knowledge_tools.json；internal/handler/knowledge_mcp.go；router.go | 同一API listener的POST /api/v1/knowledge/mcp，静态三只读工具，直接保留同服务session证明；15项组件测试和TS 1.30.0 / Go v1.8.0实际三工具业务通过 |

## 2. 交回证据与限制

后端25新文件、8既有文件修改，全部33路径由主控逐字节核对。3243份公开构建输入与最终清单一致；重构原4914源码指纹后匹配启动基线，未发现无主源码漂移。47保护件和3件批准文件不变。

33项定向单元、2项真实MySQL/ES集成、1项既有真实ES字段读回通过；API/CLI构建及四模式help启动通过。help退出码1为当前入口既有行为，未为检查改变源码。三类合成来源1580字节及新旧12/18修订、换plan、部分失败与重试、六态、篡改/权限/策略拒绝已验证。

Web交回11路径与38件证据原字节由主控独立核对，Git index及其它公共路径未变；有效全量类型基线207项、零新增，不宣称全量typecheck清洁。

MCP新增6件/修改2件，最终8路径、3247公开构建输入和两binary由主控核对；实际源码指纹可在内存恢复为其启动前4947版本，窗外未漂移。最终server SHA b036e620be82d13c4fad0c6bd70c6d131cca5bf1e670da465e8545f952b9bdad；运行二进制已在专用合成实例实核，API/Admin及真实业务通过。

早期后端组件证据使用ServiceData、memory原文与R1合成SQL发布，保留该方法边界。后续已补真实服务证据：同一4953份源码和最终Go二进制已在全新合成实例运行：原生签名session七项HTTP、真实C2/MinIO保存与R1任务发布、Web浏览器及官方TypeScript SDK 1.30.0 / Go SDK v1.8.0三工具业务通过。后端共留存173份HTTP响应，其中74份按批准a04完整校验；MinIO另有8次S3对象GET，4修订的导入对象和不可变快照字节相同；MCP另有14次工具调用、44个HTTP状态观察（含空响应）。浏览器固定3份来源覆盖1580/1580字节，MD/TXT/TSX均准确回源。后置43响应验证非owner/无会话七操作拒绝、篡改/参数/CAS/幂等及启停后旧索引失效；A已恢复local policy 3并重建ready，B保持浏览器所建policy 1/ready。

故障后重试不半发布、普通写入及文档启停隔离、换plan和成员撤销证据来自真实MySQL/Elasticsearch组件层，未伪称HTTP故障注入。R1真实任务报告模型/本地推理/外抓为0，词法拥有层显式模型nil且受测尝试为0；运行API只有internal网络且无模型/代理环境变量，不声称已测量全局进程计数。取消、任意客户端/协议版本、HTTP成员撤销、销毁后耐久性、质量/性能仍未验；Go实际回退2025-11-25成功，不支持其首试2026-07-28。模型增强、部门/Grant/临时Key、R2回接、当前POC替换和发布不在本片。最终人工Acceptance仍Pending。

## 3. 保留与后置

- 根api/rag/deepdoc/agent不存在；同名internal Go包和native资源继续保留。
- tools/scripts/download_dev_deps.py、ragflow_deps/download_deps.py仍准备native/BPE/模型/Tokenizer依赖；先有可验证替代再决定删除。sdk/python、HTTP测试、manual parity工具分开处理，不批量删除。
- 旧Use/Run/generation/ingestion守卫不开放。Chat、Agent、Memory、更多复杂格式不进入本首片。
- R2 adapter与历史验收保留；R2最终退出、完整统一模型、部门/Grant/临时Key、模型增强及规模/质量验证后置，未冒称完成。
- LICENSE、上游归属、原cmd/internal/web/docker和中文计划布局不变；本轮未提交、推送或部署。

2026-10-29后续观察：Web11路径已核收，当前4976文件/`e872c0382e37ecf01cdcdc8d6f0c9c906f214bc39d3046c0b0bf39e8aec8aba0`，58项测试和完整构建通过、类型207零新增；新隔离API29390按4966构建运行，3份原创MD/TXT/TSX已真实保存并从统一四reader回读。新格式固定发布、R2同索引及浏览器继续，未继承旧首片的MCP/质量结论。
