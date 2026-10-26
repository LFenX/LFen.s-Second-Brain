# 阶段3-R2：Document Structure 0.1.0具体候选

2026-10-26。**候选，未冻结、未接入应用、未验收。** 基线main `abdb141`；R1代码／Markdown结构工程已发布、人的验收Pending。配套[OpenAPI](knowledge-document-structure-v0.1.candidate.openapi.json)、[示例与向量](knowledge-document-structure-v0.1.candidate.examples.json)及[准备报告](stage3r2-document-candidate-verification.json)作为同一提交申请具体冻结；任何私有输入都不代替确认。101个schema、140合法例、148拒绝例、28个双编码器摘要及10个字串向量通过，仅证明候选机器件一致，不证明未实现的应用链路。

## 1. 结果、事实与本批边界

目标是把已保存的一份PDF／PPTX／DOCX固定原件，明确处理为可按页／幻灯片／章节块阅读的结构；保留表格、列表、媒体位置及派生文字出处。人和后续Agent使用同一固定表示、节点和文字窗口。原件、派生结构和未来检索输入分别管理，不强制Embedding。

已有事实：C2 0.5保存原名、原字节及SHA，支持固定授权下载；0.7提供单调世代；Structure 0.1提供任务、原键、租约及发布栅栏。Go本地`pdf_oxide`和`office_oxide`已有独立合成库证据，但不等于本片应用闭环。两份新Office二进制的9项源真值实测为6项保留、3项部分保留：原生IR改变PPTX形状顺序、将DOCX内联图移到段落后、不能用native list level冒充源ilvl。因此本候选修订为**Office由有界OOXML源结构定序、定位及取文字**，不把IR顺序冒充源顺序。历史代码及基准见[文档库准备](stage3-document-readiness.md)、[R1统一报告](stage3r1-structure-verification.json)。本地依赖固定`pdf_oxide/go v0.3.73`、`office_oxide/go v0.1.12`；后者仍用于既有原生路径，本片Office主链不依赖其有损投影。

本批每任务1个C2 .5 `original_only`来源的**当前active固定修订**，只用合成资料。旧C1／C3、C2 .2—.7、Structure .1共41份保护文件原字节不变。不创建项目清单、索引、模型／OCR任务、Wiki、跨文档主题关系或新基础设施，不物理删除原件／历史／卷。

先交受控React结构阅读及既有固定原件校SHA下载。PDF页、PPTX slide在派生区选择；DOCX按源结构块选择。首片不嵌原生视觉viewer，不承诺原版排版、Word物理页、bbox、复杂表格合并或PDF跨栏阅读序；这些是后续能力，不是用假坐标补足。本批不含PPTX整件plainText降级参数：主源结构不可用即失败，原件仍可下载。

## 2. 方案取舍与原生适配

| 选择 | 依据与影响 |
| --- | --- |
| 保持仅原件下载 | 能保存和核对资料，但不能满足页／块导航和结构化整理，不选作本片结果 |
| 在现有Go拥有层做固定派生，复用内部任务核心 | 与来源／权限／版本同一链路，保持旧公共契约；本片采用 |
| 新建独立文档服务／另一套任务引擎 | 可独立伸缩，但增加服务、队列、双重授权和恢复，当前不采用 |
| 一次性处理子进程 | PDF原生调用无context中途停止接口；三格式统一由父进程拥有终止与回收，避免超时后仍留处理调用。不是新增常驻服务或硬内存沙箱 |

| profile_id | 生产输入与允许产物 |
| --- | --- |
| `pdf-plain-pages/1` | 本地pdf_oxide；先核物理PageCount≤128，再逐页取文字，空页仍保留。根→page→text_block；没有OCR、bbox或段落识别 |
| `pptx-ooxml-slides/1` | Go有界读取presentation.xml／内部关系确定slide顺序，按各slide spTree中的源XML顺序投影shape／text／table／media；不按文件名数字、IR或文本分页符猜顺序。根→slide→heading／paragraph／text_box／table／list／media；源顺序不是视觉阅读序 |
| `docx-ooxml-blocks/1` | Go按word/document.xml body／table／paragraph／run源顺序投影，内联图锚定实际drawing位置；解析styles中明确outline及basedOn（有界且拒循环）和numbering的numId／ilvl。根→section／heading→paragraph／table／list／media；只按已证明level嵌套。无Word页码 |

Office表格保留源XML row／cell及空cell，列表按明确编号／缩进属性保留item与嵌套，inline media保留文字间位置；来源属性缺失则unknown，不从native派生默认值猜测。行列为源XML数组位置；首片row_span／column_span／is_header为null，合并布局不在本片承诺，不能把默认值当原件事实。媒体只给已知类型／长度、内部locator和未OCR状态，不返回URL、base64或猜测caption。外部关系只保留未取得引用状态，禁止外连；内部关系须规范化且停留包内，不能用任意Target读宿主文件。未知SmartArt／图表／方程等成为unsupported节点，不能假装已提取其全部文字。

Office源结构适配在现有`internal/parser/parser`拥有层实现；提取既有有界ZIP／XML安全原语供.5入口与本片按各自原预算共用，旧.5规则及响应不变。不把现有`ExtractRawBlocks`误认为直接XML读取——它实际仍经office_oxide IR，不能修复已证实的顺序缺口。本片不调用其三视图／扁平输出作权威出处，也不让原生IR覆盖OOXML位置。PDF复用现有本地引擎，避免通用parser默认DeepDoc或外部资源路径。已知类型XML解码错误必须失败，未知源块必须成为`unsupported_block`提示及统计，不能吞错误变空成功。以上新源适配未实施，须以源真值验证后才宣称保留顺序。

首版Office语法边界固定：DOCX正文依次处理body中的paragraph／table及内容控件sdtContent，段内按run／hyperlink中的w:t、w:tab、w:br／w:cr及drawing源顺序；标明隐藏文字的run、跟踪修订ins／del／moveFrom／moveTo、复杂域所在paragraph、fldSimple、altChunk及mc:AlternateContent等整段保留为unsupported，首片不混合“最终修订／原始修订”或重复Choice／Fallback。普通字段代码instrText不进入正文；字段结果不在首片伪称已可靠还原。标题只用直接或有界style basedOn链中明确outlineLvl（0—8转1—9），不能从样式名含Heading就猜层级；样式链循环或超过32层拒绝。列表保存明确numId／ilvl（0—8）、已有numbering格式／起号，缺失／不支持为null；相邻同编号段落按真实level嵌套，跳级不伪造缺失item。

PPTX先按sldId列表和内部关系读slide，再按spTree中的shape／graphicFrame／pic／group源顺序递归；支持明确text body的a:p／a:r／a:t、tab／br、普通table及图片关系。group仅保留源分组，不推断视觉坐标顺序；bullet／autoNum仅在源属性明确时成为列表，master／theme的隐含样式首片不猜。notes／comments／隐藏slide正文不自动合并进其他slide；隐藏slide本身仍保留来源位置和hidden标记。未知图表／SmartArt／方程等为unsupported，不执行嵌入对象。外部链接正文可作为文字，目标不自动获取。

## 3. 三层载体、父子与派生字节

1. **原件**：沿用Document／File／FileCommitItem身份、C2 .5不可变字节和SHA。原件`root_node_id=null/text_available=false`等旧字段保持；不能因新增派生表示而改写旧固定回执。
2. **派生结构**：独立Representation／DocumentNode，绑定原source／revision／original SHA、当前世代与分类、producer／configuration及内容manifest。新`root_document_node_id`不是C1根。contains只由单一parent投影，不另存重复边。
3. **派生文字**：`coordinate_space=derived_utf8`，0起半开区间，SHA属于确定性reading_text；不冒充原件二进制offset。结构阅读按节点呈现，不执行任意HTML／Markdown／SVG。

`rich_document_reading_v1`规则：PDF保留引擎文字，Office保留受支持源XML字符实体解码文字，不trim、不做NFC、不把HTML／Markdown转义串当正文；inline text run按源顺序拼接，line_break为LF、显式tab为TAB，inline media为零宽锚点。普通块／列表item之间LF，表格row之间LF、cell之间TAB、cell内块之间LF；PDF页之间LF。无强制尾LF。空成员仍参加分隔，如`['a','','b']`按LF连接为`a\n\nb`。没有子块的空容器为零字节。媒体／unsupported_block贡献零字节但保留节点，不用占位文字伪造抽取结果。元信息标签不另重复写入正文；未支持部分明确标出，不能静默视为完整。

父容器范围包含其子文字及内部结构分隔；前后兄弟间的分隔属于共同父范围，不能声称它是某个原生段落字符。inline媒体锚点为表示绝对派生byte，block媒体为零宽且anchor=null。空页／仅图页统一`unsupported_or_requires_ocr/no_extractable_text`、PDF页分类unknown，不从空字符串判断空白或扫描。整件无可提取文字仍保留真实地址；`quality=complete`只表示本profile完整发布，`text_coverage=supported_profile_blocks`不承诺OCR或未知块文字。

每个payload另含必填可空`own_reading_range`：heading是其标题自身范围，paragraph／text_block／passage为自身文字范围，纯document／page／slide／section／table／row／cell／list等容器及媒体为null；heading的聚合reading_range可包括所属章节正文，不能用它重复渲染标题和全部子段落。非null自身范围必须包含于聚合范围且摘要独立核对。页面用own范围和按需子节点重建；Agent仍可显式读取聚合范围。内联媒体按绝对anchor把自身文字划成前／后段展示，不把媒体占位写进正文或重复字串。

单一passage序列覆盖完整reading_text，目标4KiB、硬8KiB、overlap0；使用R1同一确定性切点规则，将叶块／行边界作为候选，UTF-8安全，无空洞／重叠。每段挂到能完整包含它的最深语义容器，同深按原树先序；无则根。passage locator为派生父地址＋该父下chunk_index，不能叫原生页内位置。语义子节点保持源顺序，passage按start/end附加到拥有父末尾，最后分配每父0起ordinal；manifest按最终树先序。不因父内容重叠重复拼接计算覆盖率。

## 4. 许可、任务目标和增量

新增**独立document_structure_only许可**，默认enabled=false／version1；不要求开启code结构许可，也不改变其版本。基础域仍catalogue、六类模型用途禁止、external_models=false、automatic_ingestion=false。真实变更版本+1，no-op不加代；先CAS再判断no-op。关闭阻止新处理／激活，有权历史仍逐次鉴权读取。分类标签不授予权限。

新任务target明确为`original_document`：source_id、source_revision_id、original_sha256、expected_lifecycle_version。请求另有expected_policy_version、expected_document_policy_version、expected_active_representation_id、profile_id、expected_producer_sha256、confirmed=true和原键。不得添加project_id／manifest_id／C1根，不接受客户端重写路径／分类或任意原生参数。

客户端先读固定.5原件、.7当前active tuple、独立许可、对应profile能力和来源派生head；任何不符均停止准备，不自动换修订／世代／producer／head。服务端受理、取原件、缓存命中、发布及每次读取重验当前权限；发布在域锁下重检原修订、单调世代、分类／位置、双策略、producer及预期head。新元信息同样形成新表示；内容缓存只复用确定性解析产物，不能复用旧授权结论。

成功表示＋节点／manifest与唯一来源head一次激活，不发布半棵树。失败／取消／旧worker／CAS失配保留旧head与历史；staged对象不经公共读取。head当前性与固定表示分别读取；归档、世代、路径／分类、策略、producer变化明确标stale，不静默更新已打开表示。来源归档不是撤权，当前用户失权则整个表示／节点／窗口／回执／游标隐藏。

## 5. 12项公共操作

基址`/api/v1`；`P=/datasets/{domain_id}/knowledge`、`R=P/document-representations/{representation_id}`。原生登录，基础头`X-Knowledge-Contract-Version:1.0.0`及新头`X-Knowledge-Document-Structure-Contract-Version:0.1.0`；旧code／C2版本头不用于新面。

| ID | 方法／路径 | 结果 |
| --- | --- | --- |
| DC01 | GET `/knowledge/document-structure-contract` | 实际就绪、三profile／producer及固定预算 |
| DC02 | GET `P/document-structure-policy` | 当前独立许可和基础策略版本 |
| DC03 | POST `P/document-structure-policy/changes` | 明确许可变更固定回执 |
| DC04 | POST `P/document-tasks` | 单原件持久任务 |
| DC05 | GET `P/document-operations/by-key` | 必填kind=task／policy／cancel及原键，读原Task／命令／固定拒绝 |
| DC06 | GET `P/document-tasks/{task_id}` | 当前Task，不返回跨协议任务 |
| DC07 | POST `P/document-tasks/{task_id}/cancel` | 原键、expected_task_revision和明确确认的固定回执 |
| DC08 | GET `P/sources/{source_id}/document-head` | none／current／stale及最近已受理task_id；不新建任务列表 |
| DC09 | GET `R` | 固定已发布表示／原件出处／生产／内容manifest |
| DC10 | GET `R/nodes` | 必填parent_id，limit1—20默认20，opaque cursor；只直接子节点 |
| DC11 | GET `R/nodes/{document_node_id}` | 固定节点／自然locator／父级及派生范围 |
| DC12 | GET `R/nodes/{document_node_id}/content` | ≤64KiB派生UTF-8文字窗 |

新对象严格拒绝未知／重复字段和非法Unicode；所有查询拒绝未知、重复或空参数。域／来源／修订／任务／命令／请求及cursor用32位小写十六进制；representation_id／document_node_id及SHA摘要用64位小写十六进制，不截断内容身份。原键1—128字符`[A-Za-z0-9_-]`大小写敏感。整数为JS安全整数，版本≥1。信封含code／message／contract_version／document_structure_contract_version／request_id／data；成功code=0。全部no-store、响应版本头与X-Request-ID匹配。

首次新写201、原键重放200；读取200；400格式／范围，401未登录（同有界新错误信封、不含数据），404不存在或不可见，412版本，409语义／CAS／完整性，429域容量，503未知或不可用。权限检查先于对象／统计／游标返回。固定语义拒绝错误须已提交完整拒绝记录，outcome_known=true；事务失败／未知不能假造拒绝。信封hint不能代替DC05完整核验。

## 6. 身份、locator和无循环manifest

沿用typed-LP和SHA256原语，不改旧投影。null、字符串、boolean、安全整数、array、object均有类型标签及UTF-8字节长度前缀；object键按UTF-8排序，数组保持原顺序。原值不trim／casefold／NFC，重复JSON键及未配对代理项拒绝。

| 摘要 | 新前缀／投影 |
| --- | --- |
| configuration | `{kind:'knowledge-document-configuration/0.1.0',configuration}` |
| producer | `{kind:'knowledge-document-producer/0.1.0',producer}`；含profile、引擎及实际构建摘要、适配版本、reading格式和配置SHA |
| task request | `{kind:'knowledge-document-task/0.1.0',owner_id,domain_id,request}`，request去idempotency_key |
| policy request | `{kind:'knowledge-document-policy/0.1.0',owner_id,domain_id,request}`，request去key |
| cancel request | `{kind:'knowledge-document-cancel/0.1.0',owner_id,domain_id,task_id,request}`；task_id取路径，request去key |
| node payload | `{kind:'knowledge-document-node-payload/0.1.0',payload}`，覆盖kind／locator／自然parent_ref／ordinal／属性／文字状态／范围 |
| structure manifest | `{kind:'knowledge-document-structure-manifest/0.1.0',nodes}`；nodes为最终树先序`{natural_ref,node_payload_sha256}`，不含派生ID |
| representation | `{kind:'knowledge-document-representation/0.1.0',owner_id,domain_id,original,source_snapshot,foundation_policy_version,document_policy_version,format,profile_id,producer_sha256,quality,reading_format,reading_text_sha256,reading_byte_length,structure_manifest_sha256}` |
| document node | `{kind:'knowledge-document-node/0.1.0',representation_id,source_revision_id,node_payload_sha256}` |

先计算自然节点payload和manifest，再表示ID，最后节点／父ID；不把根ID塞进表示投影制造循环。Task ID、原键、时间、cursor及预期旧head不进表示身份；预期head只控制任务摘要与发布CAS。不同来源、修订或空页保持不同身份。源元素ordinal只承诺同原件／同producer稳定，不承诺插入段落后的跨修订连续性。

自然locator为document_v1、pdf_page_v1的1起物理page，或office_package_v1的包内part及有序`{namespace,name,index}`源元素路径；index为相同展开名兄弟中的0起次序，根index0，最多32段，namespace／name分别保留展开名，不能用易变XML前缀作身份。part是经过校验的包内相对路径，不是服务器路径；同一slide的物理序号另在original_position中表达，不从part文件名推算。passage是derived_passage_v1、自然parent_locator与chunk_index；不含二进制offset。完整例与UTF-8／空块／身份向量在配套examples给出，运行caps的实际构建摘要不能填合成示例占位。

## 7. 原键、取消与恢复

三个写操作均在POST前持久保存完整首次body、原键、规范摘要和owner／domain／协议／操作／目标；文档家族每owner8槽、每槽≤16KiB。坏／满／存储失败禁新POST，不驱逐未知，不覆盖或迁移旧协议槽。取消路径task_id参加身份，同键换任务409。固定成功／拒绝在当前授权后回放，不重施原动作，不重新做原CAS。

认证／格式／摘要通过后，在域锁中先原键查重，再检查语义。许可关闭、版本／世代／head／producer失配、容量等确定拒绝先提交RejectionReceipt再返回，DC05回完整原body／key／digest／reason。503／超时／404／401／412或单独rejection_id都不能清槽；只有完整匹配固定拒绝才允许明确放弃，之后重读、构造新请求／键并重新认可。允许明确重发保存的同body／同key，不后台换键。

Task状态queued→running→succeeded／failed；queued／running可cancel_requested→cancelled。phase表达reading_original／extracting／projecting／staged／publishing／stopping／cleaning／done；未知native细粒度进度用null，不伪造百分比。可见状态、phase／进度和接管变更使revision+1，纯心跳不加代。取消先验expected_task_revision再判断no-op；已请求或终态且CAS相符回unchanged。取消回执只表动作被接收，另读Task终态；取消槽不能删除原Task跟踪。

正常取消必须确认自有处理子进程退出并清理后才能cancelled。期限／停止信号不是退出证明；OS层退出或清理结果未知时保持stopping／cleaning及非终态，保留域执行栅栏、禁止发布与新执行，不自动释放配额。120秒是处理／新尝试／发布期限，不虚构异常OS一定在该时刻完成回收。恢复仍原Task／原键；已过期worker无权激活。前台轮询最多2秒一次，隐藏／离页／终态停止；客户端25秒等待到期给恢复入口，不无限自旋。

## 8. 内部复用与一次性原生执行

不复制另一套worker。现有KnowledgeStructureTaskRecord以加法字段明确protocol／target_kind／target_id；旧行迁移为code协议／project／原project_id，新任务为document协议／original_document／source_id，修订／SHA／世代留在真实载荷。一个扫描器、claim／续约／fence／域容量核心分派两类生产器；旧.1 DTO、原键和任务读取原样，跨协议对象404。来源派生head和派生坐标载体在现有entity／dao／dataset内拥有，不造project／C1根，不迁移目录或改既有命名。

原生子模式建议同`ragflow_server`的私有`--document-structure-child`，在常规服务器装配前分派；不启动HTTP、SQL、队列或模型注册。父进程只执行自身路径，使用最小环境、只读原件fd和有界控制帧，不传SQL／模型Key、原件URL或任意文件路径。父进程流式SHA核对后写自有0700目录／0600临时原件；子进程输出≤16MiB受控pack，stderr保留≤4KiB供受控诊断，不回传原生日志／凭据。

本片运行支持Linux现有app容器；父进程拥有独立进程组，取消／失租约／尝试30秒期限后终止自有组并Wait，设置父死亡信号，单子模式不再执行外部子程序。记录任务／attempt／PID／start-time所有权，不能根据复用PID杀无关进程。父异常后新worker先确认旧子模式退出，再复用stage或重试；不能把API重启当子进程已退出。无法证明退出时保持恢复等待及域栅栏。最终清理只针对核验所有权的临时目录；不可变原件／历史不GC。此机制、CLI和适配尚未实施，须连同本候选确认。

首片没有硬内存隔离或完整系统调用沙箱；输入／输出预算和RSS采样不等于原生内部峰值硬上限。无模型／网络提取路径；Go HTTP拒绝哨兵及子进程网络行为须分别留证，不凭本地引擎名称宣称网络隔离。

## 9. 读取与正式页面

固定表示与当前head独立缓存；新原件／策略变化只标stale，不改变已打开representation。先取表示元数据的root_document_node_id再懒读直接子节点，页≤20。cursor32hex绑定主体／域／source／representation／parent／limit和首次600秒截止，不续期；错scope／过滤400，失权404，无published表示不暴露节点／计数。

content默认从节点start取≤64KiB UTF-8安全窗口；显式end须同时给start。越界、过预算、切断UTF-8返回400，不缩窗猜测；complete时next=null，否则next=end且前进。响应原件SHA、整个派生文字SHA、节点与fragment SHA分开；空节点可200／空字串／complete，但必须显示text_status。逐窗校验不要求为自验先拉全树。

复用现有knowledge-catalogue懒入口和tree／分栏／列表，新增正式service→hook→局部结构阅读组件；不改共享UI或新建独立后台。原件已保存、处理Task、表示currentness和节点文字可用四层状态分别显示。表格按源行列／空cell、列表按item、媒体按未OCR占位重建；层级／原文和文件出处联动，证据默认收起。

固定阅读参数为document_structure=1、document_source、document_revision、document_original_sha、document_representation，document_node可省；身份缺失／不符报错，不退回当前head。原件回链复用`.5 originalRevisionLink`，先核同domain／source／revision／SHA／长度／MIME，再走既有完整SHA下载。不上当前文档preview或匿名URL，不把新根送C1。失权／换session先隐藏可见私有内容、终止请求并丢弃迟到响应，保留原owner隔离恢复槽。

## 10. 冻结预算与实测口径

| 项目 | 候选值 |
| --- | --- |
| 输入 | 1原件≤8MiB；C2 .5保存≤64MiB保持 |
| Office容器／中间产物 | 复验有界ZIP／XML；包项≤4096、扩展内容≤16MiB、单XML≤4MiB／全部解析XML≤16MiB、XML深度≤64；中间源结构及输出pack各≤16MiB |
| 结构／文字 | PDF≤128物理页、PPTX≤128 slides；节点≤8192、深度≤16；reading_text≤4MiB |
| 分页／响应 | children≤20；cursor600秒；正文窗≤64KiB；metadata JSON≤256KiB、content JSON≤512KiB；变更body≤32KiB |
| 任务 | code／document共用每域1执行＋1等待；租约30秒／10秒续；最多3attempt；创建起120秒处理期限、每次30秒原生期限 |
| 客户端 | 等待25秒；前台轮询≥2秒；恢复槽≤16KiB×8；派生阅读首屏目标≤3秒 |
| 实测门槛 | 冻结三格式各1套公开描述的合成基准，各5次任务最长≤10秒；meta／children／content每类各20次串行最长≤1秒 |
| 内存 | 50ms名义采样，父Go与自有子进程分别及合计报告，合计相对基线增长≤256MiB；不以API单PID代替child，不以VmHWM累计峰代替本轮增量 |

所有阈值是拟冻结验收值，不是本批已测结果。分别报告首次解析／实际缓存复用、子进程启动、API暖态、采样实际间隔／漏短峰和浏览器首屏；不称生产p95或百万规模能力。超预算受控失败，不截断后宣称完整；失败不影响合法已保存原件。

## 11. 事实标注、执行顺序与验收

沿用准备阶段RD01—20的目标与源真值，按新源结构profile修订执行断言：PDF物理1／2／3、空／仅图页unknown、不同原件空文字不合并；PPTX标题／正文不串slide、空slide2仍存在、2×2表空cell和b、源shape顺序及PNG位置；DOCX真实标题level、段落、表／list／inline图顺序、字符实体、`a—b`为5 UTF-8字节。RD12改为主结构不可用时明确失败／result=null／无伪slide，不保留旧plainText降级断言。原生IR的6保留／3部分保留作为反例原样留证，不能改称新OOXML方案已经通过。源XML、原生IR、后续源投影、应用发布和浏览器分别留证；源真值先于解析输出，禁止反向自标。

| 顺序 | 唯一写入及验收 |
| --- | --- |
| R2.0 A＋只读消费输入 | 完成本文／OpenAPI／examples、身份与字串向量及源标注，核41保护件，真人对同提交具体确认后冻结；确认前业务0 |
| R2.1 A | 完善拥有层OOXML源结构和PDF适配、派生范围／完整覆盖／manifest；至少20预标事实分层验证，源顺序反例必须通过，未知能力明确标unsupported |
| R2.2 A | 共用任务核心、独立许可、子模式拥有／停止／恢复、原子表示及有界读取；Go定向、实际MySQL／S3、取消／崩溃／ABA／跨协议容量／负权限通过，旧契约不变 |
| R2.3 清洁固定提交后交客户端唯一Web窗 | 正式12操作、三类槽、精确原件准备、受控块阅读、固定下载回链、空／错／stale／无权／迟到与恢复；Mock不能代替真实服务 |
| R2.4 A收回写权 | 原六服务同版合成HTTP／浏览器、三格式各5次及各类20读取、父子RSS／原生挂起终止／清理证明；针对性修复，正常提交检查后按授权推main，独立留人的决定 |

不启动多轮审查。R1工程验收、R7真实资料退出、阶段3整体与后续检索／引用／复用、MCP临时授权、人格记忆分别保持原范围；本候选不提前放开模型／真实资料。完整具体候选仍须真人确认，本文件自身不授业务实施权。
