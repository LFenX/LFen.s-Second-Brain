# 阶段2 A → B：C2来源导入候选契约

版本：0.2.0，状态：ReviewCandidate，基础契约：C1／C3 1.0.0。阶段1交付8aa324e已获用户明确验收。本文件只定义阶段2首个小闭环，不表示B已核对或整个阶段2已完成。

## 1. 本轮范围与固定规则

只支持手动提交的UTF-8文本／代码，`source_kind=text_file`、`input_mode=inline_text`。来源登记中列出精确相对路径，最多16个；一次预检及导入只处理一个文件。单文件≤2MiB、无NUL；登记／提交JSON≤16KiB，含正文的预检JSON≤16MiB，字段长度和数量还受请求总容量限制。预检10分钟内有效。使用原生登录和同源`/api/v1`；每个操作都按当前私有域所有者授权，不接受客户端指定owner／tenant／服务器绝对路径。

原文先进入私有预检暂存，预检不创建Document、目录修订或索引；提交后才发布原生Document／File／FileCommit与C1目录。每个`registration_id + relative_path`持久映射一个Document，内容SHA不充当来源ID。登记由`domain_id + registration_key`幂等创建；相同键不同配置返回409。不同登记代表不同来源范围，不跨登记按内容去重。

处理方式固定为`catalogue_only`：结构deferred、全文／向量disabled、模型用途空、外发false、自动处理false；客户端不能开启这些能力。组件侧和语言在每项预检中显式提交，登记默认值只用于B预填，不覆盖逐项分类。标签不授予权限。

排除已知凭据／VCS／依赖／构建路径和两种私钥标记，不声称能够识别全部秘密。本轮仅合成资料；真实资料仍须后续明确允许清单。UTF-8 JSON不合法时拒绝，不静默替换原始非法字节。

## 2. HTTP操作

全部C2操作要求`X-Knowledge-Source-Contract-Version: 0.2.0`；基础头`X-Knowledge-Contract-Version`保持1.0.0。响应保留原基础envelope并增加`source_contract_version`，不修改C1响应或能力列表。完整请求／响应见[OpenAPI](knowledge-sources-v1.openapi.json)。

| 操作 | 路径（前缀/api/v1） | 返回 |
| --- | --- | --- |
| 能力与限制 | GET /knowledge/import-contract | 200，仅声明实际实现的单文件子集 |
| 登记 | POST /datasets/{domain_id}/knowledge/registrations | 新建201，相同配置回放200，配置冲突409 |
| 读取登记 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id} | 200，不返回服务器路径或凭据 |
| 预检 | POST /datasets/{domain_id}/knowledge/registrations/{registration_id}/previews | 201，固定单项、清单SHA、预期修订、有效期；不发布来源 |
| 读取预检 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id}/previews/{preview_id} | 200，不回传暂存正文；含已消费任务ID |
| 确认导入 | POST /datasets/{domain_id}/knowledge/registrations/{registration_id}/imports | 新回执201，原幂等键回放200；必须绑定preview_id、manifest_sha256及idempotency_key |
| 读取回执 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id}/imports/{import_task_id} | 200，持久回执；身份重新授权 |

提交顺序：重新授权 → 核对登记／清单／有效期／策略版本／当前来源修订 → 原生保存及C1发布 → 持久回执。一个预检只消费一次；相同幂等键、同预检和清单回放原回执，换预检或清单使用同键返回409。原回放重新授权，但不重新导入、不因原预检过期而失去回执。幂等记录随回执保留，本轮没有自动删除或回收。

这是同步单项操作，没有后台批量调度。任务完成状态仅`succeeded`／`failed`；单项为`saved`／`unchanged`／`failed`，目录为`ready`／`failed`，其他产物保持禁用。201表示回执创建，不保证导入成功，B必须检查状态。可持久化的发布失败保存失败回执并保留旧来源；相同键回放失败回执，不隐式重试，重新预检后用新键重试。无法持久化回执时返回5xx，不声称生成了任务。

存储写入与数据库不是分布式事务：数据库发布使用事务及保存点，失败不改变原Document指向／当前目录修订；内容寻址的未引用对象保留待后续受控清理，本轮不删除共享版本对象。客户端断开不代表已完成提交被撤销；先按幂等键重放确认结果。

## 3. B逐项核对清单

- [ ] 登记键、登记ID、相对路径、Document/source_id、来源修订、节点ID各自语义；禁止在前端再生成来源ID或用SHA合并文件。
- [ ] 登记默认分类预填，逐项group_id／component_side／language明确提交；前端、后端、共享、CLI、非代码分别展示。
- [ ] 预检只展示固定一项、字节数／摘要／预期修订／到期时间；确认操作与预检分开，不能自动提交。
- [ ] 检查HTTP与业务状态：201失败回执、401登录、404隐藏无权／不存在、409配置／幂等／修订／过期／已消费冲突、413容量、415格式、5xx失败；失败不沿用旧正文假装成功。
- [ ] 提交后使用回执的source_id／source_revision_id／root_node_id接现有C1原文页，不另建读取接口；`saved`不表示可检索或已向量化。
- [ ] 网络超时保持同一个幂等键回放；失败回执明确提示重新预检后新键重试，不创建自动重试风暴。
- [ ] 核对单文件同步子集是否足够开发首个表单；批量、列表／分页、异步进度、取消、目录与二进制能力需要下一增量，不从draft字段推断已经支持。

尚未收到B反馈；勾选状态全部保留待核对，不以A实现和测试代替B确认。双方确认前C2不标记Frozen，B可以按候选契约核对或准备消费者测试。

## 4. 拥有边界与下一步

| 拥有者 | 本轮负责 | 边界 |
| --- | --- | --- |
| 本机A | C2机器契约、Go实体／迁移／路由／服务、原生保存与C1发布适配、授权／禁模型、合成验证和运行加载 | 不修改B的Web／客户端，不接入真实资料 |
| B | 核对本清单，准备登记／预检／确认／回执界面和消费者合成测试，沿用原登录／查询层和C1页面 | 不修改共享实体／DAO／路由，不另建来源或任务后端，不擅自扩大C2能力 |
| 双方 | 以同一候选版本核对字段、错误和显示；语义调整先改唯一契约再同步生产／消费 | 未做真实B页面联调不得标记2B完成或阶段2整体退出 |

C1／C3原OpenAPI、B阶段1源码和历史测试报告继续保留。目录扫描／受控根、PDF／PPT原件、网页／RSS、批量任务、移动／删除／保留规则仍属后续工作；模型、解析、索引与检索不进入本次导入。A只生成并导入本轮合成资料，Git不传登录、模型Key、数据库、原件或运行配置。
