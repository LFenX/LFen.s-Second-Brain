# 阶段1 A → B 接续说明

版本：v0.19；契约：C1／C3 `1.0.0`；日期：2026-10-24（Asia/Shanghai）。

用户已批准阶段0，并授权本机A实施阶段1、验证后推送。本文交接A的领域底座；**B的页面接入和双方纵向验收仍待完成**。工程结果以 [阶段1-A底座验证报告](阶段1-A底座验证报告.json) 为准。开发继续在 `LFenX/LFen.s-Second-Brain`，不建立另一套后端或节点库。

## 1. B先执行什么

1. 拉取本仓库 `main`，从它建立自己的 `codex/` 短分支。不要沿用旧 Fork 的独立领域实现。
2. 核对 [OpenAPI](docs/knowledge-foundation-v1.openapi.json)、[合成示例](docs/knowledge-foundation-v1.example.json) 和本文的身份、权限、修订语义。此版本是A交付的冻结基线；**没有把B尚未作出的核对记为通过**。
3. 先实现 `1B.1`：在现有 React／Dataset 页面准备“域 → 项目／主题 → 模块 → 来源 → 节点”的浏览与原文面板；组件侧标签必须分别显示前端、后端、共享、CLI、非代码。
4. 再做 `1B.2`：接真实C1／C3接口，覆盖加载、空目录、失败重试、无权、版本冲突；一份合成来源完成页面回读。Mock只用于开发，不作为阶段退出证据。
5. 共同核对“一域＋一来源＋一节点＋原文＋页面”。A的API验证通过不代表B页面已完成，也不代表完整阶段1已验收。

**本轮禁止调用模型。** B使用本机原生登录身份，不接收A的登录令牌、租户Key、百炼或DeepSeek Key；本轮不发放临时共享凭据。

## 2. 复用与自研边界

| 能力 | 直接复用RAGFlow | A本轮新增／适配 | 留待后续 |
| --- | --- | --- | --- |
| 身份与数据库 | 原生认证、Knowledgebase／Document／File、MySQL、GORM | 数据集三个领域字段；`knowledge_group`、`knowledge_source_revision`、`knowledge_node` 三张扩展表；加法迁移 | 精细模块Grant、期限／撤销／额度 |
| 原文与修订 | 原存储、FileCommit／FileCommitItem、原生版本回读 | 目录登记事务把观察到的Document原文写入原生内容寻址对象和版本记录；目录只保存映射 | Git项目清单、监听、文件编辑同步、完整删除／保留策略 |
| 结构 | 原生文件目录与Document身份 | 组织分组、文件根节点、手工section／symbol／passage、来源修订与UTF-8字节范围 | AST自动切分、确定性代码边、PDF／PPT结构、Wiki生产 |
| 模型与权限 | 原生授权检查及服务入口 | 注册域强制所有者私有、仅目录／原文；原生检索／Chat／解析任务／chunk修改等入口接执行检查 | 策略可配置放行、词法／结构查询、可选Embedding与引用优化 |
| 页面与MCP | 既有 React Web、组件、原生认证和MCP传输 | A未修改Web或MCP工具 | B接浏览页面；检索阶段4、临时共享阶段5、人格／记忆阶段6 |

原有 `cmd`、`internal`、`web`、`docker` 和根文档结构保留。没有迁移Qdrant、部署图数据库或建设平行RAG服务。

## 3. 冻结模型与身份

| 概念／字段 | v1实际定义 |
| --- | --- |
| `domain_id` | 原生 `Knowledgebase.ID`，32位小写十六进制；**一套域身份**。一个域只容纳访问边界相同的内容。 |
| `KnowledgeDomain` | 数据集的领域DTO；名称沿用数据集，category为family／personal／project／experience／resource／custom；category首次注册后冻结。 |
| `KnowledgeGroup` | 组织树；kind为project／topic／module，`parent_id`只指同域分组。根parent为null，树深含根≤16。 |
| `source_id` | 原生 `Document.ID`，表示同一逻辑来源；直接重复上传会创建另一Document，**不会自动合并为同一来源**。 |
| `source_revision_id` | 原生 `FileCommitItem.ID`；本轮快照绑定原文字节及目录分类。原文SHA-256单独保存，不能把它当修订ID。 |
| `KnowledgeSourceRevision` | 指向原生File／Commit的目录投影；含分组、相对路径、组件侧、语言、SHA-256、大小、当前标志和根节点。没有第二份正文存储权威。 |
| `logical_node_id` | 对 `['knowledge-logical/1', domain_id, source_id, kind, logical_key]` 按Go `encoding/json.Marshal`序列化后取SHA-256；同文件／kind内logical_key唯一。 |
| `node_id` | 对 `['knowledge-node/1', logical_node_id, source_revision_id, structure_version]` 同样取SHA-256。修订或结构版本变化产生新实例。 |
| `structure_version` | 服务端生成：文件根`source-v1`，手工节点`manual-v1`。AST版本后续另定义，不能原地改写旧节点。 |
| `parent_id`（节点） | 只表达同域、同来源修订的内容包含关系。组织分组不是符号的语法父节点；代码依赖边也不是parent_id。 |
| 组件与语言 | 来源`component_side`为frontend／backend／shared／cli／non_code，`language`单独保存；节点通过来源继承。分组也有自己的组件侧标签，**标签不授予权限**。 |

所有ID对客户端均为不透明值：B使用服务端返回值，不重写哈希算法、不从文件名／数组序号／行号生成node_id。前端组件key使用node_id，跨修订对比使用logical_node_id。手工logical_key使用明确的限定名称，避免同文件内重名；自动AST键由阶段3生产者定义。

阶段2的增量生产者必须复用同一来源的Document.ID，再发布新修订；“上传新Document、保留旧logical_node_id”不符合此契约。项目级快照清单、来源连接器、关系边和派生物不是本轮已实现的模型。

## 4. HTTP契约与消费顺序

基础路径 `/api/v1`，认证沿用现有Web客户端的 `Authorization`，保持原生值；不要无条件补`Bearer`。接口没有域临时Key认证。请求可带 `X-Knowledge-Contract-Version: 1.0.0`；不匹配返回409。

| 方法／相对路径 | 用途 |
| --- | --- |
| `GET /knowledge/contract` | 能力、契约版本、限制；只声明C1／C3。 |
| `GET /knowledge/domains` | 当前所有者可见的已注册有效领域，游标分页。 |
| `PUT /datasets/{domain_id}/knowledge/domain` | 空的原生私有数据集注册为领域；同category重复请求幂等。 |
| `GET …/knowledge/domain` | 领域与实际生效的固定策略。 |
| `POST / GET …/knowledge/groups` | 新建分组／列某父分组的直接子项；不传parent_id列根。 |
| `POST / GET …/knowledge/sources` | 显式快照来源／列当前来源；不传group_id列全域当前来源。 |
| `GET …/knowledge/sources/{revision_id}` | 读取指定来源修订元数据。 |
| `POST / GET …/knowledge/nodes` | 手工建子节点／列指定修订及父节点的直接子项。GET必须传source_revision_id。 |
| `GET …/knowledge/nodes/{node_id}` | 指定节点，包含其固定来源修订。 |
| `GET …/knowledge/nodes/{node_id}/content` | 固定修订原文，可缩小start_byte／end_byte范围。 |
| `POST …/knowledge/policy/check` | 模型用途策略预检；v1始终allowed=false，**不是授权操作**。 |

这里的省略路径写法仅用于表格；OpenAPI包含全部14个实际操作及请求／响应schema，可供B生成类型。没有把schema文件自动发布为HTTP下载路由。

### 最小创建与读取顺序

1. 原生 `POST /datasets` 建空私有数据集。请求只用 `name`、`permission: me`、`parse_type: 1`、`parser_id: general`。**不要传embedding_model**：原生创建接口传该字段会运行可用性探测，违反本轮零模型调用要求。沿用租户模型配置只涉及本地元数据解析，不调用模型。注册拒绝已有Connector关联或解析任务历史的数据集，不接管正在自动处理的旧库。
2. 对返回ID执行PUT领域注册，再创建project／module分组。
3. 原生 `POST /datasets/{id}/documents` 用multipart字段`file`上传合成UTF-8文件，**不要启动parse**。注册域的上传适配支持.tsx／.jsx等代码，保留原文件名与字节；部分扩展名在原生Document中归为txt，知识分类使用Source的language和component_side。
4. POST来源快照，传document_id、group_id、relative_path、component_side、language。首次expected_revision_id省略或空串。
5. 使用返回的root.node_id及source.source_revision_id浏览根／子节点；需要演示子层级时显式POST手工节点，不把它称作AST解析结果。
6. 原文以固定node_id读取；新修订发布后旧节点不跳到新内容。页面收到409刷新当前来源并提示冲突，不静默覆盖。

### 响应与范围

- 新接口成功为真实HTTP 200／201，`code: 0`；错误为对应HTTP状态及字符串code，含`contract_version`、`request_id`。404同时隐藏“不存在”和“无权”；500不暴露SQL／存储细节。原生认证中间件、上传、检索等仍保留其原生响应，**HTTP 200不等于code成功**。
- JSON请求体≤16KiB，拒绝未知字段、缺失必填字段、不支持的null、多个JSON值；查询拒绝未知或重复参数。
- 分页默认50、最大200；使用next_cursor续页。游标绑定用户／域／父项／来源修订，每页重新授权；游标不是授权令牌，列表不提供事务级快照。
- 来源≤2MiB，UTF-8且无NUL；扩展名完整列表见OpenAPI的source_import。PDF／PPT／二进制／OCR在v1未支持；旧POC文档不因此变更。
- `start_byte`／`end_byte`为原始UTF-8字节的左闭右开范围，偏移相对来源全文；请求必须落在节点范围，边界不可切断UTF-8字符；每次≤65,536字节。CRLF保留。JS字符串索引不是UTF-8字节偏移，B不得直接混用。
- 行号从1开始，end_line为实际覆盖的最后一行；末尾换行不额外增加覆盖行。空文件根范围[0,0)，行号为1。
- UI按纯文本／代码组件展示text并保留空白；不要把原文作为可信HTML执行。阅读样式由B实现，不改写原始字节或源定位。

## 5. 权限、外发和原生入口

注册域v1固定为：`owner_only`、`index_modes: ['catalogue']`、`allowed_model_uses: []`、`external_models: false`、`automatic_ingestion: false`。所有者取CreatedBy；即使原生permission被其他路径改为team，新域授权仍保持私有。有效所有者通过登录身份判定，不能由请求体指定。

原生Use／Run路径、检索底层Embedding／rerank、Chat入口、解析排队和生产Embedding解析器、chunk更新，以及模型兼容性检查接了服务端执行拦截；数据集更新禁止注册域改team、挂接Connector或提交模型字段触发可用性探测。B修改显示名等元数据时只提交变更字段，不回传包含模型字段的整份原生表单。C3预检之外还存在实际拒绝测试，不能只依靠前端隐藏按钮。

原生数据集会保留继承的Parser／Tokenizer配置，其中出现embedding／full_text不表示已经执行。B的知识页面显示新接口返回的**实际生效策略**及“目录／模型禁用”，不要用休眠的原生解析配方推断索引状态。

该限制适用于**已注册领域的受控来源工作流**；不是对用户任意手工粘贴内容或整个应用网络的沙箱。现有未注册POC数据集保持原行为，本轮没有用它运行模型。阶段1不提供放行开关；以后放行某一模型用途需更新服务端策略、契约和回归。

无权资源、跨域父项／节点／修订均不能通过新接口取得。Native File／Document身份、游标和SHA都不是凭据；MCP工具、导航、缓存或未来关系扩展也不能扩大访问范围。更细模块Grant、临时密钥哈希、到期／撤销／配额在阶段5实现，不能提前使用所有者API Key充当共享Key。

## 6. 发布、并发与生命周期边界

来源快照相同输入重复提交返回同一修订；字节、路径、分组或分类发生变化时，必须用expected_revision_id做比较交换。更换当前修订与其根节点映射在同一DB事务内发布；旧修订仍可读取。节点追加只允许当前来源修订，同身份改名称／父级／范围返回409。

同域目录写入的关键发布操作锁数据集行。原生内容对象写入先于DB提交；DB回滚不发布目录／版本记录，但会留下未引用的不可变对象，**不声称对象存储和MySQL具有跨系统原子事务**。清理与保留策略在来源生命周期阶段补齐。

本轮快照观察Document已存原文；不自动同步外部文件、Git变更或原生File编辑器的新内容。删除／失效原生Document或Dataset后，新接口隐藏其快照／节点；本轮没有实现全历史物理清除、导出恢复或附件权限治理。B不要提供这些未实现按钮。

## 7. 文件归属与防分叉

| 修改范围 | 负责人／规则 |
| --- | --- |
| `internal/entity/knowledge.go`、Knowledgebase扩展字段、`internal/dao/knowledge_migration.go`、共享OpenAPI与fixture | A维护唯一语义；B提差异，不自行定义另一套ID、Policy或Source版本。 |
| 新知识service／handler／router、原文版本适配、原生权限及禁外发入口 | A维护。B新增消费需求先给请求／响应样例及失败用例。 |
| `web`页面／类型客户端／组件与交互 | B；消费服务端opaque ID、实际策略和修订。不要从数据库直读或在浏览器放服务商Key。 |
| 后续检索／证据／MCP／人格消费编排 | B；依赖C2—C6逐次冻结，未实现时明确显示能力未就绪。 |
| 构建、启动配置、源码到运行产物、回退 | A；源码合并后必须证明运行二进制已更新。 |

双方从同一main建立分支，持续合并真实服务的小闭环；B的页面分支不同时改共享迁移和授权。破坏字段、ID、授权或修订语义的变更必须同步版本、生产者、消费者与测试；不靠Git自动合并代替契约核对。本文只是可接续说明，没有向B发送消息或代替B确认。

## 8. 构建、运行与回退

Go／原生库版本、二进制SHA、源码清单摘要和实测结果见验证报告。Linux构建遵循项目原 `AGENTS.md` 与 `build.sh`，不绕过CGO和静态ORT检查。本机在与运行镜像相同的Ubuntu24.04环境安装工具；源码构建容器只收到公开跟踪源码，没有挂入真实Key或资料。

在准备好原项目指定Go、Clang／LLD、CMake、PCRE2开发库与原生静态依赖后：

```bash
uv run tools/scripts/download_dev_deps.py
bash build.sh --cpp
bash build.sh --test -run TestKnowledge ./internal/entity ./internal/permission/... ./internal/dao ./internal/utility ./internal/service/dataset ./internal/service/document ./internal/service/nlp ./internal/service ./internal/handler ./internal/router
bash build.sh --go
```

依赖准备脚本还会下载原生解析运行资源；下载资源不等于调用百炼／DeepSeek。API Key配置继续本机保存，不提交仓库。标准Docker镜像／Compose不会因Git拉取而自动含有新Go代码：B须从该源码构建后加载，不能只启动旧官方镜像再判断接口缺失。

本机沿用 `lfen-ragflow-poc`、原端口、六个服务及卷，只在被忽略的本地Compose中以只读方式挂入本轮二进制；没有改公开Compose默认镜像或把322MB级二进制提交Git。另一机器使用自己的路径和凭据，不能照搬A的绝对路径。

回退顺序：停止当前应用服务；恢复执行前的本地Compose（移除自编译二进制挂载），启动固定原镜像。加法表／列可保留，不做破坏性回删。仅当需要恢复元数据快照时，停应用后由所有者使用本机私有DB备份恢复；备份含凭据，不上传Git。回退不恢复本轮后新增的其他用户数据，不能在继续使用后直接全库覆盖。

本机私有执行目录为 `.local-data/stage1a-20261024`；包含构建日志、合成HTTP明细、回退Compose和DB备份，受Git忽略与受限ACL保护。构建容器不是生产基础设施；实际应用二进制挂载文件在私有runtime目录，运行期间不要删除。

## 9. 验收与接续

| 范围 | 判据 |
| --- | --- |
| A `1A.1` | C1／C3机器契约可读取；域／分组／来源／节点身份与fixture一致；前后端标签分开；非法父级／范围／修订拒绝；私有隔离和禁模型执行有测试。 |
| A `1A.2` | 原生快照原文精确回读、幂等与修订冲突、旧版本和失败回滚通过；实际运行二进制SHA与构建产物一致；原配置与旧POC未被改写；有回退材料。 |
| B `1B.1—1B.2` | 按本契约接真实目录／原文，页面具备加载／空／失败／无权／409状态；不调用模型，不隐式解析；fixture只是准备材料。 |
| 整体阶段1退出 | A与B共同完成“一域→一来源→一节点→原文→页面”；随后由用户验收。A单侧完成不提前标整阶段通过。 |

阶段2开始前再确认实际知识域名称／归属、首批来源及受控目录；目前只保留合成验证数据，未导入家庭隐私、真实AICS／AOCI内容，也未重跑旧POC问答或60题实验。
