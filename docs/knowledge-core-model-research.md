# 统一内容模型：本地基线与三个官方项目对照

日期：2026-10-28；本地基线`7233c495311f1c40c67254582138169d5ed0e205`。执行者`/root/content_model_research`，主会话复核关键本地守卫及三个官方固定提交页面。全部为只读源码／官方资料研究，未安装或运行外部框架，未调用项目模型。下文推荐均为候选；[主计划](knowledge-core-realignment-plan.md)负责执行顺序。

## 1. 先复用本地真实能力

| 本地实物 | 已有内容 | 缺口／边界与引用 |
| --- | --- | --- |
| `ParseResult`／`DocItem` | metadata、typed text/table/image、Markdown/Text和warnings；派发归一`[]DocItem` | `internal/parser/parser/parse_result.go:44`、`doc_item.go:36`；`internal/ingestion/component/parser_dispatch.go:314`。标题／列表／代码还依赖CKType/Extra，不能直接当完整语义树 |
| 原生表格／chunk | grid、单元格／跨度／表头／页；ChunkDoc含类型／顺序／位置／sheet等 | `internal/entity/table.go:34`；`internal/ingestion/component/schema/chunk_types.go:106,252`。规范化文字不是原XML |
| C1 | 一域一原生Dataset；分组；Document稳定source，FileCommitItem修订、SHA、原件指向 | `internal/entity/knowledge.go:15,28`、`dataset.go:102`；授权仍owner-only，不能以分类扩为部门共享 |
| R1 | 文件／符号／章节／passage、原字节及行、逻辑与修订实例身份、带证据关系 | `internal/parser/structure/types.go:39`；`internal/entity/knowledge_structure.go:194` |
| R2 | Representation、文档树、ordinal、类型属性、自然locator、文字状态、派生读取范围、producer/config | `internal/entity/knowledge_document_node.go:5`、`knowledge_document.go:299`。结构表示不是检索索引 |

当前注册域及原生入库禁止模型，R1/R2显示`not_created`索引；新链路必须先提供本地词法／结构能力与模型用途分离的具体候选。证据：`internal/permission/knowledge.go:17,28,49`、`internal/service/dataset/knowledge_document_bind.go:157`。

## 2. 三个项目与固定来源

访问日期均为2026-10-28，研究范围限定实际结构、转换结果与源定位。下述schema具有某字段，不代表每个转换器都正确产出该字段。

| 项目 | 固定提交 | 可借鉴方向 |
| --- | --- | --- |
| Docling Core | `55829160b6eb17b6796fb6649e9b43bcce94cb01` | 文档语义树、独立provenance、chunk回指元素 |
| LlamaIndex | `619d74664e8404612038bc15a1a40c686037277c` | 来源／父子／邻接关系，metadata的模型用途隔离 |
| Haystack | `71a9a7eb212a671e9b68a27c65374b692a294ab1` | 简单索引文档、切块来源／顺序及层级元数据 |

**Docling。** `DoclingDocument`区分body/groups/texts/pictures/tables/pages；NodeItem用引用表达父子。DocItem把类型与`prov`分开，TextItem保留orig/text。[文档结构](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/document.py#L172)、[节点](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/items/node.py#L20)、[文本](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/items/text.py#L18)

位置包含页号、bbox、charspan；表格单元格有行列跨度、表头和可选bbox。已查通用类型不直接表达本项目OOXML part／元素路径。chunk元数据可回指原语义元素及标题。origin的Uint64 binary_hash不能替代本地完整SHA-256或修订ID。[位置](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/common/reference.py#L189)、[表格](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/items/table/table_data.py#L20)、[chunk](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/transforms/chunker/doc_chunk.py#L26)、[origin](https://github.com/docling-project/docling-core/blob/55829160b6eb17b6796fb6649e9b43bcce94cb01/docling_core/types/doc/common/origin.py#L12)

**LlamaIndex。** BaseNode含ID、可选embedding、metadata和关系；关系包括SOURCE/PREVIOUS/NEXT/PARENT/CHILD。资源可指向文字、字节、路径或URL。metadata可按embedding／模型上下文排除，但排除字段不是ACL或撤权机制。[官方schema](https://github.com/run-llama/llama_index/blob/619d74664e8404612038bc15a1a40c686037277c/llama-index-core/llama_index/core/schema.py#L191)

parser以`parent_doc.text.find`回找chunk文字填写字符位置；这是输入文本坐标，不能当PDF、OOXML或原UTF-8字节定位。由此借鉴“关系及资源引用”，不照搬字符偏移为统一源定位。[实际回填](https://github.com/run-llama/llama_index/blob/619d74664e8404612038bc15a1a40c686037277c/llama-index-core/llama_index/core/node_parser/interface.py#L78)

**Haystack。** Document含ID、正文、二进制、meta、分数与可选稠密／稀疏向量，适合作检索投影；其自动ID使用meta／向量等，不适合作本项目稳定source或revision身份。[Document实现](https://github.com/deepset-ai/haystack/blob/71a9a7eb212a671e9b68a27c65374b692a294ab1/haystack/dataclasses/document.py#L30)

DocumentSplitter保留source_id、页号、块序号、字符起点与重叠；页号依赖输入分页符。自定义切分后的偏移存在回退。Hierarchical splitter按大小组织parent/children/level，它不是解析出的源语义树。[切块](https://github.com/deepset-ai/haystack/blob/71a9a7eb212a671e9b68a27c65374b692a294ab1/haystack/components/preprocessors/document_splitter.py#L306)、[层级](https://github.com/deepset-ai/haystack/blob/71a9a7eb212a671e9b68a27c65374b692a294ab1/haystack/components/preprocessors/hierarchical_document_splitter.py#L98)

## 3. 差异与采用决定

“未提供”仅指已读schema，不推断整个外部产品不存在扩展能力。

| 维度 | 本地与外部差异 | 采用决定 |
| --- | --- | --- |
| 组织归属／主题 | 本地Tenant/Dataset/Group可复用，但知识域强制创建者；三外部内容模型不提供可直接替代的部门授权 | 沿用原生身份，补最小域共享绑定；主题不授权 |
| 来源与不可变修订 | 本地Document/FileCommit/SHA与生命周期比通用origin/source关系更贴近当前需求 | 保留本地权威身份，不改成外部自动ID |
| 父子与关系 | Docling语义树及chunk回指可借；R1已有带范围／摘要的事实关系 | 一个父子事实只存一份，不复制一套contains关系；事实边保留证据 |
| 自然位置 | 本地有代码原字节、PDF物理页、PPT slide、Office XML及派生UTF-8 | 分离原locator和派生读取范围；bbox仅在未来有可靠生产器时增加 |
| 表格 | 本地grid/R2 row-cell及源locator；Docling类型更细 | 可借鉴单元格语义，未知跨度／表头保持未知，不填虚假默认 |
| 附件 | Docling ImageRef／Llama资源说明节点可指向外部资源；本地R2还缺独立附件授权资源ID | 复用原对象引用，附件读取仍需服务端scope；不自动下载或OCR |
| 处理策略 | 本地有策略、独立结构许可和producer/config；三项目不保证节点都须向量化 | 目录、词法、结构、向量分开；metadata用途选择不等于外发许可 |
| 权限追溯 | 本地有原生身份与固定来源；外部通用metadata没有完整动作/范围/期限/撤销内核 | 权限留同一业务服务，禁止自由metadata充当可信ACL |

统一最少保留六个业务概念：域、分组、来源修订、内容表示、语义节点、可检索投影；事实关系按需存在，有效授权是服务端上下文，均不要求每节点建一套ACL。模型候选与调用影响见[主计划第3—4节](knowledge-core-realignment-plan.md#3-最小统一模型候选)。

## 4. R2存量适配器映射

| 格式 | 统一读取映射 | 保留的能力边界 |
| --- | --- | --- |
| PDF | 固定原件→物理page→文字块→派生UTF-8读取范围 | `internal/parser/parser/document_structure_pdf_cgo.go:13`使用ExtractText(page)；无bbox/OCR则不生成 |
| PPTX | `sldIdLst`顺序→slide→shape／表格／媒体；保留part＋路径locator | `document_structure_pptx.go:12`；源slide顺序不是视觉阅读序，不能按文件名猜序 |
| DOCX | body／段落／run／标题／列表／表格／媒体，保留own/aggregate读取范围 | `document_structure_docx.go:14`；没有Word物理页，未OCR/unknown/unsupported原样保留 |

不扩格式、OCR、版面或新边角性能。已有R2运行结果保持原输入／版本／失败；统一时增加映射，不能用新模型覆盖旧固定源定位。

## 5. 解析与chunk独立衡量

1. 固定Markdown／CRLF文本／TSX三类原创合成原字节及一份更新修订，人工预标文字、类型、顺序、父子、自然位置、附件与缺失。
2. 先关chunk验转换：逐项比预标事实与位置，规范化损失单列；不能用“能检索到”掩盖内容抽取错误。
3. 再用正确节点验chunk：UTF-8界、覆盖／重叠、短符号／章节完整、长节点有界拆分、表头上下文、节点回指和修订一致。参数试验后与人商定，不在研究中冻结。
4. 接实际原生对象／SQL／DocEngine，验证精确符号、中文事实、重复文字和过期修订检索及回源。当前本报告未执行。
5. 个人／部门甲／部门乙用合成主体验权限，候选／父节点／关系／附件／最终引用一起验。新部门权限未实施。
6. 模型与真实资料质量独立待授权；合成通过和工程报告不代表人的验收。

原生Markdown构造器当前启用远程取图，最小离线链必须显式关闭或沿已验证R1源码路径进入适配层，见`internal/parser/parser/markdown_parser.go:66`。这一条件与禁模型同属必须核查的真实分支。
