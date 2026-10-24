# 阶段2 B：C2 0.2.0 消费者契约核对

核对日期：2026-10-25。核对版本：**C2 0.2.0 ReviewCandidate**；核对提交：**657ebd77d23a7716f8f76b8f54822f8baed821e7**（`codex/stage2a-import`）。以下全部确认、建议和页面规则均以该版本和提交为准。C1／C3 保持 1.0.0，旧 `0.1.0-draft` 不用于正式客户端。

**结论：未发现阻断当前单文件子集冻结的消费者契约缺口。B 同意按下述范围和保守错误处理进入双方冻结确认。** 有 3 项非阻断说明建议，由 A 决定是否补充。本文是 B 的核对意见，不替代双方冻结记录或用户验收；A 的共享契约仍保持候选状态。本轮不合并 main，不生成正式 C2 客户端，不接真实导入页面。

本次交付分支为 `codex/stage2b-c2-review`，从上述 A 提交创建；先切到 A 分支并确认指定提交后，才分支保存 B 反馈。A 的交接清单、共享 OpenAPI、示例、实体和后端均未修改。没有启动多轮审查。

## 1. 按用户原核对项反馈

本表“无阻断”仅指契约核对，不表示页面已经实现或联调通过。JSON 指针均指 [C2 OpenAPI](knowledge-sources-v1.openapi.json) 的 `#/components/schemas/`；HTTP 前缀为 `/api/v1`。

| 原核对项 | 字段或接口位置 | 具体场景、期望行为与 B 结论 | 阻断冻结 |
| --- | --- | --- | --- |
| 登记、预览、确认、回执页面字段 | `RegistrationRequest`、`Registration`、`PreviewRequest`、`Preview`、`CommitRequest`、`Receipt`；登记与其下 `/previews`、`/imports` | 四步需要的数据齐全：先保存范围，逐项预览，再单独明确确认，最后按业务状态展示回执。登记成功不显示“原文已保存”，预览成功不显示“已入目录”。**已确认，无缺项。** | 否 |
| 分组与五类组件标签、语言 | 上述请求／响应的 `group_id`、`component_side`、`language`；C1 `GET …/knowledge/groups` | 分组沿用 C1 分组 ID，`null` 表示未分组；五类为 frontend／backend／shared／cli／non_code。语言是≤32字符的显式元数据，可为空，不是自动语言识别结果。登记默认值只预填；每次预览三字段都提交，空语言不继承默认值。标签不授予权限。**已确认。** | 否 |
| 身份是否区分 | `registration_key`、`registration_id`、`relative_path`／`item_key`、`source_id`、`expected_revision_id`、`source_revision_id`、`root_node_id` | 登记内固定路径对应稳定原生 Document；更新保持 source_id，修订和节点由 A 返回。B 不以 SHA 建身份、跨登记去重或创建映射表。首次预览来源／预期修订可空，失败回执的发布 ID 全空。**已确认；详细映射见第2节。** | 否 |
| 过期、修订冲突、权限、大小／格式错误 | C2 全部响应；`Error`／`NativeUnauthorized`；`Preview.expires_at`／`consumed_task_id` | 401重新登录；404统一隐藏无权／不存在；413提示容量；415提示格式。409采用“预览不可继续或请求冲突”的保守处理，先核查已消费任务／原幂等请求，不依赖英文 message 猜精确原因。过期或来源变化后重新预览、再次明确确认。**首版可实现；NB-01 非阻断。** | 否 |
| 201可表示失败；保存、目录、索引分开 | `POST …/imports` 的 200／201 `ReceiptResponse`；`Receipt.oneOf` 及 item 各状态 | HTTP 201和 envelope code=0只说明回执已创建。检查 receipt.status，再展示 save_status、catalogue_status；结构始终 deferred，全文／向量始终 disabled。失败显示“导入失败”，不展示成功阅读按钮；unchanged显示“内容与分类未变，沿用修订”。**已确认；失败回执正反例校验通过。** | 否 |
| 超时同键查结果；失败重新预览再重试 | `CommitRequest`；POST／GET imports；GET previews | 确认前固定 preview_id、manifest_sha256、idempotency_key。超时／连接断开／5xx均先显示“结果待确认”，保留原请求，用同一键、同一预览和清单核查。回放可200，也可能首次真正创建201；已持久回执不因预览过期丢失。已知 failed 才提供“重新预览”，新预览、新键、再次确认。**满足首版交互；不自动改键、不自动循环重试。** | 否 |
| 回执接 C1；单文件同步是否足够 | `Receipt.items[].source_id/source_revision_id/root_node_id`；C1 `GET …/sources/{source_revision_id}`、`…/nodes/{node_id}`、`…/nodes/{node_id}/content` | ID格式和语义相容。现有 C1 service 可直接接收，Mock transport 验证通过。现有页面只从目录选择来源，回执指定修订的入口／状态适配仍是 B 冻结后工作。无需 A 新增读取接口。单文件同步表单不需要来源列表、后台任务、取消、目录扫描或二进制能力。**契约足够；真实页面接线与联调尚未完成。** | 否 |

## 2. 字段与页面映射

| 位置 | 字段 | 页面用途与约束 |
| --- | --- | --- |
| 登记请求 | registration_key、name、source_kind、input_mode、include_paths | 登记键1–64位字母／数字／下划线／连字符；显示名称；本版只提交 text_file／inline_text；精确相对路径1–16条，不是扫描规则或通配符。相同键同配置可回放；配置不同409，不能暗中换键创建另一份来源。 |
| 登记请求／回读 | group_id、component_side、language、registration_id、domain_id、created_at | 默认分类预填，展示所属域与登记时间；不接受前端指定 owner、tenant、服务器绝对路径。 |
| 预览请求 | relative_path、text、group_id、component_side、language | 只选登记范围内一个文件；B 保留本次提交文本及分类用于确认。文件按 UTF-8 字节计量，保留 CRLF、BOM和空文件，不用 JS 字符数当文件大小，不静默替换非法 UTF-8。 |
| 预览回读 | preview_id、policy_version、manifest_sha256、expires_at、consumed_task_id | 展示预览有效期；摘要只回传，不自行重算服务器清单；已消费时转回执核查。服务器决定是否过期，本机倒计时仅作提示。 |
| 预览单项 | action、sha256、byte_length、source_id、expected_revision_id、逐项分类 | 展示新增／更新／未变、原文摘要、字节数和预期旧修订。只读确认区；更改文件、路径、分组、组件侧或语言即废弃旧预览，必须重新预览。 |
| 确认请求 | preview_id、manifest_sha256、idempotency_key | 一次明确确认绑定一份不可变请求。提交后禁用重复确认；结果未知时保留该请求，不把取消 HTTP 当撤销导入。 |
| 回执 | import_task_id、domain_id、registration_id、preview_id、manifest_sha256、status、created_at、model_calls、items | 同步最终回执，无 running／进度轮询。对照当前域、登记、预览、清单和单项内容摘要／分类；上下文不符显示协议异常，保留恢复信息，不跳往别的来源。model_calls必须0。 |
| 回执单项 | save_status、catalogue_status、structure_status、full_text_status、vector_status、code | “已保存／未变”“已入目录”“结构处理待后续”“全文和向量索引未启用”分开显示。failed回执只提供恢复与重新预览，不暗示已生成新修订。 |

| 身份 | B 的解释 | 禁止混用 |
| --- | --- | --- |
| registration_key | 用户范围内登记的幂等创建键；实际作用域 domain_id＋key | 不是后端生成的登记 ID，也不是 Document ID |
| registration_id | A 返回的32位登记 ID | 不是具体文件身份 |
| relative_path／item_key | 登记内规范 POSIX 相对路径；本版 item_key等于路径 | 不是服务器绝对路径，也不跨登记按路径合并 |
| source_id | A 返回的32位稳定原生 Document ID | 不用 sha256、登记 ID或路径字符串代替 |
| expected_revision_id | 预览捕获的旧修订，首次导入为 null | 不作为本次成功修订；不从客户端另行选择覆盖目标 |
| source_revision_id | 成功回执返回的32位固定修订；unchanged可沿用原修订 | C1 sources详情路径使用它，不能传 source_id |
| root_node_id | 成功回执返回的64位根节点 ID，指向该固定修订 | C1节点／原文接口使用它，不能传修订 ID |
| sha256／manifest_sha256 | 原文字节完整性／服务器确认清单 | 均不是来源身份；前端不实现另一套身份算法 |

## 3. 页面状态与恢复规则

主流程：`待登记 → 已登记 → 预览中 → 待明确确认 → 提交中 → 成功回执／失败回执`。提交中遇到网络异常进入“结果待确认”；它不是导入失败回执。预览阶段和确认阶段不启动解析、索引或模型。

| 场景 | 契约／实现证据 | 页面处理 | 冻结影响 |
| --- | --- | --- | --- |
| 登记POST响应丢失 | 域＋registration_key，配置排序后比较；`knowledge_import.go:81` | 保存原键和配置后回放，200／201都可取得登记；409提示配置冲突，等待用户处理 | 无阻断 |
| 预览10分钟过期 | expires_at；确认时 `knowledge_import.go:293` 检查 | 停止沿用预览，重新预览后再确认；只有元数据回读，无暂存正文下载 | 无阻断 |
| 其他导入已发布新修订 | 预览 expected_revision_id；`knowledge_import.go:315` | 不自动覆盖；提示预览不可继续，重新预览查看新基线。英文 message不作为稳定分支条件 | 无阻断；精确原因见NB-01 |
| 预览已消费／重复窗口 | consumed_task_id；同预览唯一任务约束 | GET预览获得任务 ID后 GET回执，不能用新键再次消费；若原请求结果未知，仍按原请求核查 | 无阻断 |
| 确认超时、断开或5xx | `knowledge_import.go:279`先查已存幂等回执，再检查期限；交接说明第2节 | 不断言已撤销或失败；保留同键同请求，用户触发“确认结果”。有任务 ID时可GET回执，没有时重放原POST。仍异常则保留待确认，不自动新建预览和提交 | 无阻断；本轮未模拟真实网络中断 |
| 同键换预览／清单 | `knowledge_import.go:281`返回409 | 停止提交，核对原请求／回执；不自动换键绕过。只有已知失败后，或确认旧请求未发布后，才由用户开始新预览 | 无阻断 |
| 201＋status=failed | `Receipt.oneOf`失败分支；`knowledge_import.go:332`保存失败回执 | 红字／错误提示“导入失败”，旧来源不冒充本次成功。相同键只回放旧失败；重新预览→新键→再次明确确认才重试 | 无阻断 |
| 401／404及权限改变 | NativeUnauthorized或版本化Error；各次读取／重放重新授权 | 登录失效时清空可见私有正文及元数据；404统一资源不可用，不泄漏是否存在。恢复时先重新授权当前域，不能展示旧缓存兜底 | 无阻断 |
| 版本409 | knowledge_source_contract_version／knowledge_contract_version | 停止当前功能，提示刷新或版本不匹配；不回退 draft或自动改版本 | 无阻断 |
| 400输入／413容量／415格式 | knowledge_invalid_request／knowledge_body_too_large／knowledge_source_too_large／knowledge_source_unsupported | 保留可编辑合成输入并显示安全提示，容量按各端点限制校验；不自动转码、截断或调用模型修复 | 无阻断；NB-02、NB-03 |

页面刷新后若只恢复了预览 ID和元数据、没有可核对的本地原文，不把它伪装成完整正文预览；可以核查已提交回执，未提交则重新选择文件、重新预览和确认。恢复信息按当前登录主体和域隔离，不能把正文、登录凭据或运行配置写进 Git。

成功阅读连接为：回执域＋source_revision_id → 现有 C1 `source()`取得完整 Source → 用回执 root_node_id读取节点和原文，并校验 source_id／修订／摘要一致。超过64KiB继续使用现有 C1字节窗口读取。不得因目录刷新而自动切到新修订。**页面入口适配、Hooks和会话内恢复状态尚未实现，等待双方冻结。**

## 4. 给 A 的非阻断建议

| 编号／位置 | 具体场景与已核实事实 | 期望行为／A可选调整 | 阻断冻结 |
| --- | --- | --- | --- |
| NB-01；`Error.code`；`knowledge_import.go:282,293,317`；`knowledge.go:42` | 同一确认端点的幂等冲突、过期／已消费／策略变化、修订冲突都返回 knowledge_conflict；Error.code仅string，message未承诺稳定。仅凭响应无法给出可靠的精确原因文案 | 建议补公开错误码／统一恢复表；将来如要分别显示“已过期”“来源已更新”，由A增加稳定reason/code并统一修订。当前首版明确接受通用冲突提示＋回执核查＋重新预览，不要求B匹配英文message | **否**；精确原因文案后置 |
| NB-02；`Capabilities.request_bytes`、`x-scope-limits.request_bytes`；`handler/knowledge.go:60` | 能力返回16MiB，但登记与确认实际16KiB；16MiB只用于含正文预览。交接说明第1节已写清，所以没有行为矛盾；单看机器能力容易把所有请求都按16MiB预检 | 建议在端点／字段description写明单位与作用域，或未来提供分端点容量。B首版依交接说明分别校验：原文2MiB、预览JSON16MiB、登记／确认JSON16KiB | **否**；现有说明足以实现 |
| NB-03；`PreviewRequest.relative_path/text`；`utility/knowledge.go:12`、`dataset/knowledge_import.go:29` | 实际按扩展名白名单、规范路径和排除目录共同判断，不是所有合法UTF-8文件都接受；如合成 `notes.csv`即使UTF-8也会415。白名单只在实现可见，正文Schema也不验证UTF-8字节容量／路径排除规则 | 建议A补充允许格式和路径规范的公开说明，方便文件选择提示。B首版不维护第二份后端白名单，不宣称“任意文本均可”；服务端400／413／415为最终判定，拒绝时不转换或上传替代内容 | **否**；格式提示完善可后置 |

不要求为本轮新增批量进度、来源列表／分页、目录扫描、PDF／PPT、网站／RSS、取消、移动或删除接口。它们均是后续范围，不据旧draft推断已支持。

## 5. 本次证据、限制与后续

本地实际执行：新增 [knowledge-sources-candidate.test.ts](../web/src/services/knowledge-sources-candidate.test.ts)，使用项目现有 Ajv 2020 配置直接消费 A 的 schema，未另写 C2生产类型或接口。**新增14项通过；连同现有C1服务和阅读器29项，共3套43项通过。** 定向格式和lint结果见 [机器记录](stage2b-c2-review-verification.json)。

- 编译13个候选schema，验证A的3份请求示例、7个操作的版本头、5类组件标签、必填分类与禁止客户端指定身份／处理策略。
- 合成预览覆盖首次空来源身份、已消费任务和私有正文不回传；回执覆盖201业务失败、unchanged、失败不得携带发布ID、不得声明索引成功／模型调用或异步running。
- A的HTTP记录共76条，本地检查其中49条C2记录的方法／路径／响应状态在候选中存在。记录没有响应体或请求体，**没有将它冒充49次响应体回放或B实机请求**。
- Mock现有C1 transport，确认成功回执的修订和根节点ID直接进入既有source／node／content方法且继续只用C1 1.0.0头；它证明消费者接法可行，不证明真实后端／新页面已联调。
- A报告中的MySQL／MinIO、持久回执、重启、回滚、CAS、禁模型验证是A提交的工程证据。本轮B只核对对应路径及测试源码，未在B机器重跑这些服务，没有将A证据改记为B实测。

候选OpenAPI、示例和HTTP记录的Git对象SHA256与A报告完全一致，具体值记录在机器报告。测试在公开源码镜像中使用已有锁文件依赖缓存执行，未复制私有配置。未运行全量Web类型检查／构建、真实导入浏览器流程、网络断开实验或并发压力测试；没有新增业务代码、模型调用、真实资料或凭据。

后续顺序：A记录本反馈与双方冻结决定（若调整语义则给出新的候选提交）→ 双方确认同一冻结契约 → B正式客户端／Hooks、四步页面、同键恢复与C1入口适配 → 合成真实联调 → 验收后安排main合并。本报告不提前勾选A交接清单、不标记Frozen、不替代阶段2页面验收。
