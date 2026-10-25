# 阶段3结构、代码关系与阅读接入准备

来源：只读消费会话“阶段2知识消费与前端”（01a17244-5107-7aa3-beed-2ea36062d042）一次集中调查，观察版本8048ede；A归档于2026-10-26。它未写文件、执行测试、调用业务API／模型／外网或启动审查。以下为带拥有路径的调查结论与后续建议，**不是阶段3实现／验收结果**。R4后续已获37574fb冻结批准，不代表原件派生正文接口也获批准或实现。

## 1. 可复用边界

| 范围 | 当前拥有路径与能力 | 不能据此宣称的能力 |
| --- | --- | --- |
| 固定文本／节点 | `internal/service/dataset/knowledge_source.go`的Create/Get/ListKnowledgeNode；同修订、父包含、UTF-8边界、当前修订重检，manual-v1／source-v1；Web知识目录及固定窗口 | 自动语法节点、代码事实边、原二进制的页／章节或派生文本坐标；组织分组不是AST父节点 |
| 检索父文 | `internal/ingestion/component/chunker/general.go`的Mom、`internal/ingestion/task/indexdoc/parent_child.go`的MaterializeParentChunks、`internal/service/nlp/retrieval.go`的RetrievalByChildren | Mom是索引层关系，按父文本归并；不能代替固定修订＋位置身份，同文不同位置不能因此合并 |
| 文档结构图 | `internal/service/dataset_structure_graph.go`读取已编译实体／关系，投影到模板／关键词；NER及依存关系另有提取器 | 没有据此证明TS／Go符号、import／export／type／静态call事实；也不能笼统说原生全部关系均来自LLM |
| 解析结果 | `internal/parser/parser/parse_result.go`、`doc_item.go`有heading/list/code和page/layout透传；DOCX／PPTX IR分别处理章节和slide位置 | PPTX降级plainText后没有逐slide定位；Markdown有转码／规范化，不得把输出偏移当原件偏移 |
| 本地PDF文本 | `pdf_parser_common.go`显式plain_text进入`internal/deepdoc/parser/pdf/plaintext_cgo.go`，逐页文本及1起始页号 | 无表格结构／精确裁剪坐标；默认DeepDoc涉及ORT模型，其他远端分支不在零模型范围运行 |
| 阅读／图UI | `web/src/components/document-preview/pdf-preview.tsx`有授权头及定位；compilation的nav-tree／representation-renderer、chunk representation有树／证据展示 | PPT现取整件blob／ArrayBuffer，props未提供指定slide；原生graph／claims不是C1／0.5固定原件派生契约 |
| Wiki | `dataset_artifact_service.go`的UpdateWikiPage、file_commit.go的RecordPageEdit／ListPageCommits／GetPageCommitDetail，前端编辑／版本差异；wiki_dirty.go有dirty队列 | best-effort历史与source_doc_ids不是固定source_revision/node回链；尚无人工编辑保护、派生冲突和同版证据契约 |

代码结构方面，消费方在Go生产文件及go.mod调查未找到现成TS／TSX或Go AST事实生产器／tree-sitter依赖。Go标准库AST可作后续候选；TS绑定、实际构建可用性和依赖尚未选定，不能承诺已支持。正式实施前由拥有者复核选型及相应调用路径。

通用chunk Run存在预处理／空白与换行规范化／过滤；代码保真须钉选原字节范围并验证覆盖。Markdown解析器默认会FetchRemoteImages，消费方指出现配置入口没有该开关；后续适配必须在真实拥有路径明确禁取图，不能只传未实现的配置键。Web遵守`web/AGENTS.md`，组合共享UI、service→hook及统一查询键，不改components/ui。

## 2. 后续最小顺序

1. 先界定来源／结构／派生表示版本、位置坐标、类型化关系证据、权限、预算及一致发布。组织树、来源结构和关系分开，旧C1保留；当前C3禁止自动ingestion，受控结构处理先发共享策略／契约。
2. 合成TSX＋Go＋Markdown，零模型生成原字节／符号范围，再产生可证明的import／export／type边。无法解析和动态调用明确unknown，不用字符串猜测补事实。
3. 父子chunk作为同版节点的有界读取视图，绑定来源范围与结构版本；校验非空白覆盖、重叠和同文不同位置，不能直接套Mom文本身份。
4. R4正式原件链路就绪后，再接本地Office IR与PDF plain_text；原二进制修订、页／slide／章节、派生UTF-8坐标分别保存。无裁剪坐标只承诺页级，降级明示结构不可用，不伪造字节／crop位置。
5. 复用目录／树／预览／局部图，按固定版本展示原件、阅读、节点／chunk及回链；分页、预算和人机同API一起验。Wiki先3—5篇人工合成资料验证编辑保护／来源过期，不自动生成、不引入完整GraphRAG。

## 3. 尚未执行的合成验收

- DataTable组件／函数、别名／type-only／re-export、Go结构／方法；注释和字符串的伪import不出边，动态未解析不出确定边。BOM／CRLF／中文／emoji／Tab、重复代码与UTF-16换算，非空白覆盖及固定旧修订。
- PDF页、PPT空白／仅图slide、Markdown标题／代码／列表／表格、DOCX章节／表格预标至少20个事实；原件与派生文本各自摘要／版本，DOCX不假定固定物理页，降级／失败不发布伪结构。
- 跨域／跨修订parent、错cursor及受限关系目标拒绝；目录／边／统计／错误不泄露名称或计数。大树有界分页，缓存包含主体／域／修订；不移用R3性能结果。
- 人与Agent按同节点／范围读取相同内容；来源更新有dirty提示、旧证据可读、人工编辑不被覆盖、冲突显式。HTML／Markdown／附件不自动外发取图，模型0。

这些是后续设计输入；执行记录、真实纵向证据和人类验收另行收口。原消费会话继续真人只读；当前R4实际开发由A和新客户端会话按明确写权窗口承担。
