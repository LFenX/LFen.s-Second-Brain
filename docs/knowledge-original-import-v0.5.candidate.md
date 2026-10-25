# C2 0.5.0 文档原件与大文件候选

日期：2026-10-26。状态：**ReviewCandidate，未冻结、未实现、未部署**。基线为main `38ce003`，进入S2-R4。本文、[OpenAPI](knowledge-original-import-v0.5.candidate.openapi.json)、[合成向量](knowledge-original-import-v0.5.candidate.examples.json)共同定义候选；[核对记录](stage2r4-original-contract-verification.json)只证明文档与向量验证，不代表运行已实现。既有C1/C3 1.0.0、C2 0.2.0／0.3.0／0.4.0及历史候选17文件保持原字节。

## 1. 结果、事实与取舍

目标是把PDF、PPTX、DOCX和超过2MiB的UTF-8文本／代码原件收进已有知识域和分组，查看确切大小／SHA与分类，明确确认后形成固定修订，授权下载时仍能取得同一份原始字节。保存原件与解析、阅读表示、分块、全文索引、Embedding分开；本项只完成原件收录，不把“保存成功”显示成“解析成功”。

已核实的代码事实：

| 位置 | 现有能力 | 本项处理 |
| --- | --- | --- |
| `internal/storage/types.go`、`minio.go` | 存储凭据／桶前缀与MinIO客户端已存在；公开`Put/Get`参数为`[]byte`，内部读取整个对象 | 复用实例、配置和对象命名边界；补MinIO流式写、stat和固定范围读能力，不走整件`[]byte`回退；其他存储后端不冒充已支持 |
| `internal/service/file/source_snapshot.go` | 原生FileCommit／Item、SHA地址、文件夹行锁和前驱 | 复用版本记录及事务边界；从已验证不可变对象记录快照，抽出同一个元数据核心供小文本与原件使用，不复制第二套版本算法 |
| `internal/service/file/source_snapshot_preparation_test.go` | 合成PDF／Office两版字节与失败回滚已测 | 只作原语证据；未证明流式HTTP、Office渲染或R4授权 |
| `internal/entity/knowledge_import.go` | 登记＋路径绑定同一原生Document／File，原键回执 | 复用同一登记／绑定／来源身份体系；增加独立`original_upload`入口和暂存模型，不把正文塞旧`inline_text` |
| `internal/service/dataset/knowledge_source.go` | C1假定≤2MiB UTF-8、文本行号和file根节点；原文读取会整件加载 | 原件修订进入同一来源修订体系，但标为`original_only`；C1仍只消费原来的`text`表示，不伪造文本节点或行号 |
| `internal/service/file/file_upload.go`、`internal/handler/file.go` | 原生上传／下载会整件读入内存；通用上传不能替代R4固定确认 | 不直接把通用按钮接到知识域；复用认证、文件名编码等内部能力，新增受控业务适配 |
| `internal/permission/knowledge.go`及原生来源保护查询 | 已有私有域、文件／祖先保护、禁模型和禁自动ingestion执行检查 | 在新增入口、原件修订与原生旁路上逐一验证；不把“已有guard”当R4已测 |

比较后的选择：保持现状无法接收二进制与大文件；直接放开通用上传会绕过固定预览／确认并占用整件内存；给原件建第二套知识库会拆散身份和权限。因此采用**单文件上传暂存→固定元信息预览→明确确认→原生固定快照**，保留原知识域／分组／Document。首期不加入分片续传、目录自动扫描、批量原件任务、远端URL或新的存储服务。

## 2. 容量、格式与外发默认值

以下是待确认的POC默认值，不是已测性能或生产容量。并发／暂存配额不等于累计对象存储限额；当前仅验证有限合成样本，长期保留与孤儿对象容量由R6规定，本版不承诺长期自动回收：

| 项目 | 候选值 |
| --- | --- |
| 单文件 | 1—67,108,864字节（64MiB）；空文本继续走旧入口，不接空二进制 |
| 每用户／域 | 最多2个未结束上传，按声明长度预留总额≤128MiB；正在接收最多1个；全部署正在接收最多2个，由SQL协调 |
| 上传／预览有效期 | 创建起30分钟固定，不因GET或重试续期；上传已ready后保留同一截止时间 |
| 单次传输 | 最长120秒；30秒租约、10秒续约，数据库时间；临时落盘及MinIO流式复制不在SQL长事务内 |
| 缓冲 | 网络／摘要按64KiB块处理，应用显式数据缓冲≤1MiB；临时文件≤64MiB／接收项，权限0700目录／0600文件；总磁盘预留与全部署槽位一致 |
| 元信息JSON | ≤32KiB，未知字段／重复键／尾随JSON／非法UTF-8拒绝；正文只通过二进制PUT |
| 读取 | 单个合法`bytes`区间；任意二进制边界；最多完整64MiB，流式返回；不沿用C1的UTF-8窗口规则 |
| 保留 | 暂存到期／取消不再可用；回执、原键映射、固定修订持久保留；当前不物理GC对象和历史 |

支持`.pdf/.pptx/.docx`及旧文本类型白名单中的UTF-8代码／文本；文本继续检查NUL、既有凭据路径和两类私钥标记（跨缓冲边界）。这是有限拒绝规则，不宣称完整密钥／恶意文档扫描。原件不发百炼、DeepSeek或其他外部服务，不调用原生解析／OCR／ingestor／模型。图片、压缩包、可执行文件和其他二进制附件本轮不上传；仍保留既有“附件指向”方向。

媒体类型不能只信文件名或浏览器：声明为明确类型或`application/octet-stream`；PDF校验文件头版本与末尾EOF标记，Office校验ZIP中央目录、`[Content_Types].xml`及相应`word/document.xml`／`ppt/presentation.xml`关系与内容类型，不执行文档或抓外部关系。OOXML最多2048条、声明解压总量≤256MiB、单条压缩比≤100，拒绝加密、越级／绝对／重复成员、链接成员和已知宏入口；只读取校验用XML，每份≤2MiB、合计≤8MiB，不把整个包解压落盘。这些是接纳检查，不是完整格式合法性、病毒检查、可渲染性或正文解析验收。

文本扩展沿用基线`internal/utility/knowledge.go`的固定白名单：`.txt/.md/.markdown/.ts/.tsx/.js/.jsx/.go/.css/.scss/.json/.yaml/.yml/.html/.htm/.sql/.sh/.c/.cpp/.h/.hpp/.java/.rs/.py`；全部识别为`text/plain`，文件原字节不变。PDF为`application/pdf`，PPTX／DOCX为各自标准OOXML类型，明确枚举在OpenAPI。声明类型只接枚举的小写完整值，不带charset等参数；浏览器File.type为空／不在枚举时声明octet-stream，不能由此跳过服务端接纳。最终媒体类型不为octet-stream。后续增加扩展需单独变更候选，不默许任意文本扩展。

## 3. 版本、登记和身份

新增接口使用原生登录，头为`X-Knowledge-Contract-Version: 1.0.0`及`X-Knowledge-Source-Contract-Version: 0.5.0`；版本不符409。所有响应`Cache-Control: no-store`；成功JSON继续`code=0/data/contract_version/source_contract_version/request_id`包络。旧端点继续各自版本，禁止改全局版本头或自动降级。

- 新登记固定`source_kind=original_document/input_mode=original_upload`，显式`include_paths`为1—32个规范相对POSIX文件路径，UTF-8字节排序且唯一，不接受目录、通配符、绝对路径、反斜杠、`..`或凭据路径。分类沿用`group_id/component_side/language`词汇；分组必须属于本域，标签不授权。
- 同域`registration_key`继续唯一；同键同规范配置返回原登记，异配置409。不改变现有0.2／0.3登记，也不允许把新登记交给旧文本／目录／批次导入端点。
- `source_id`仍等于原生Document ID，绑定登记＋精确路径；不同登记／路径即使字节相同也不合并。上传ID、请求键、内容哈希均不是来源身份。
- `source_revision_id`仍是原生FileCommitItem ID；原件修订以同一来源修订表内部`representation_kind=original_only`及原件元信息扩展保存，旧行默认`text`。不新建第二套Document／版本头。扩展字段不直接序列化进冻结C1 DTO。
- `original_only`不建立C1文本根节点；新DTO的`root_node_id=null/text_available=false`，不生成虚假行号／页数。原件列表／详情使用0.5新端点，页面按现有分组树展示原件卡片。C1在SQL过滤后分页，只列／读`text`表示，对原件ID沿用既有415 `knowledge_source_unsupported`，不添加冻结C1字段或新错误码；冻结0.4清单也不能加入原件。若以后要混合项目清单、原件派生正文／页节点，须另发共享契约。
- `original_filename`是上传时原始basename，1—256个Unicode字符，不带目录、控制字符、冒号或反斜杠；`relative_path`是用户选择的知识定位，两者分别显示。服务端同时校验原名与逻辑路径的格式族一致（PDF／PPTX／DOCX／UTF-8文本代码），下载按固定修订的原名安全编码，不把原名当存储路径。名称不做隐式截断或规范化。
- 每次确认核对当前预期修订。相同字节、原名、最终媒体类型和完整分类才是`unchanged`；改上述元信息或字节为`updated`，第一次为`created`。原件输入摘要采用同一长度前缀编码，首token `knowledge-original-source/0.5`，顺序为SHA、原名、最终媒体类型、逻辑路径、group_id、组件侧、语言；在本域／Document内唯一。不能复用旧C1只含文本分类的投影；同内容不同时间的请求不制造新修订。回到历史相同输入按既有来源版本复用规则恢复固定修订，不把时间戳、上传ID或幂等键混入来源输入摘要。

## 4. 接口与最小流程

路径均在`/api/v1`下，精确定义见OpenAPI。ID为32位小写十六进制，SHA为64位；请求键1—128位既有安全字符。

| 操作 | 路径末段／结果 | 行为 |
| --- | --- | --- |
| 能力 | `GET /knowledge/original-import-contract` | 返回本版限制和格式；未实施时不能宣称ready |
| 登记 | `POST /datasets/{domain_id}/knowledge/original-registrations`；`GET .../{registration_id}` | 201新建／200同配置；显式路径及默认分类 |
| 恢复登记 | `GET .../original-registrations/by-key?registration_key=...` | 按配置键查询原登记，重新授权；不自动创建 |
| 更新目标 | `GET .../{registration_id}/target?relative_path=...` | 必须是登记内精确路径；`source=null`表示当前查询未绑定来源，否则返回本登记原件当前修订，供创建时带期望头；不扫描全域猜测 |
| 创建上传 | `POST .../{registration_id}/uploads` | JSON只含路径、完整分类、声明长度／媒体类型、可空客户端SHA、预期来源修订、原键；201新会话／200原请求回放 |
| 恢复上传 | `GET .../uploads/by-key`；`GET .../uploads/{upload_id}` | 重新授权取得状态；读状态不触发上传或确认 |
| 上传原件 | `PUT .../uploads/{upload_id}/content` | `application/octet-stream`原始请求体；可带Content-Length且必须匹配声明，缺少时仍逐字节限制；200返回ready元信息 |
| 放弃暂存 | `DELETE .../uploads/{upload_id}` | 显式、幂等；已确认返回原结果而不删除历史 |
| 确认 | `POST .../{registration_id}/imports` | 原上传ID＋manifest SHA＋原确认键＋`confirmed=true`；201固定回执／200同请求回放，不复用0.2路径 |
| 恢复确认 | `GET .../imports/by-key`；`GET .../imports/{import_task_id}` | 固定回执查询；未知不能自动换键重发 |
| 目录／原件详情 | `GET /datasets/{domain_id}/knowledge/original-revisions`及`.../{source_revision_id}` | 列表只含当前原件，20条／页；详情读任意固定修订，与当前头分开 |
| 原件读取 | `GET/HEAD .../original-revisions/{source_revision_id}/content` | 每次授权；200整件／206单范围／416不满足；不返回桶、对象key、临时路径或预签名URL |

用户流程：选择本地文件与既有域／分组→登记明确路径→创建会话→上传完成→展示服务端真实SHA、类型、大小、原名／路径、分类、created/updated/unchanged及“未解析／未索引”→初始未选中的明确确认→固定回执→固定修订元信息／原件下载。不能把选择文件、上传成功、恢复或打开预览当确认。二进制不显示伪Markdown；超2MiB文本也不自动塞入C1阅读器。

前端复用现有`FileUploader`的选择／进度外观，但显式单文件、关闭目录、设置64MiB，不传会自动上传的`onUpload`。传输仍走`registerNextServer→next-request`同一认证实例：0.5局部service用Axios `transformRequest`在拦截器之后返回原`File/Blob`，避免`common-util`把二进制展开成普通对象；实施须用真实适配器／浏览器验证，不假设配置即通过。不新建第二套请求栈，不改全局转换或共享UI。

## 5. 上传状态、幂等与失败恢复

状态为`awaiting_bytes/receiving/ready/interrupted/rejected/cancelled/expired/committed`。只有`awaiting_bytes/interrupted`接受新一次完整PUT；`ready`字节和预览不可更改，换文件或分类必须新建上传；`rejected/cancelled/expired/committed`终态不可重开。上传至同一会话的并发PUT只有一方能取得数据库槽位／epoch，其余409 `upload_busy`。

| 事件 | HTTP／机器码 | 状态及下一动作 |
| --- | --- | --- |
| 正常EOF但长度不足／超声明长度；客户端SHA不符 | 400 `upload_length_mismatch/upload_sha256_mismatch` | `rejected`，修正后新建上传 |
| 实际超64MiB；格式不符 | 413 `upload_too_large`；415 `upload_format_unsupported` | `rejected`，不保留可确认预览 |
| 连接中断／进程崩溃 | 未取得确定HTTP结果；`upload_interrupted` | 租约结束后`interrupted`；先GET，明确从0重传原会话 |
| 单次接收超120秒 | 409 `upload_timeout` | `interrupted`，先GET再明确重传 |
| 存储／公布ready事务失败 | 503 `original_storage_unavailable` | 未公布ready时为`interrupted`；无法确认事务结果时先GET，不覆盖可能已ready的数据 |
| 写入时到期 | 410 `upload_expired` | `expired`；新建上传，旧会话不复活 |
| ready再PUT | 409 `upload_already_ready` | 保持ready，GET恢复；不读取新正文 |
| 确认预期头／摘要冲突 | 409 `revision_changed/manifest_changed` | 保持ready且头不变；固定输入不能改，取消后重新准备 |
| 取消已committed会话 | 200 `UploadEnvelope` | 保持committed，携带原`import_task_id`；不返回另一DTO或删除修订 |

授权仍有效时，GET状态／按创建键GET始终200返回包括expired／rejected等终态的`UploadEnvelope`；410仅用于对过期会话的新写请求。创建同键同请求回放也是200原会话，不受TTL／配额阻断；同键异请求409。`revision`为状态与可见进度的单调正整数，`updated_at`为数据库时间；同会话迟到的低revision响应不能盖掉较新状态，同revision不同关键内容视为无效响应。401／404／版本或策略冲突立即停止轮询并清敏感显示；网络／503按三次失败规则转手动核查。

创建会话时锁域与请求键，再检查权限／当前来源头、精确路径、容量与配额，持久化请求身份。首次POST丢响应先按原创建键GET；同键异配置409，回放优先于到期／配额检查，绝不自动换键。请求键、原始请求身份和终态元信息在暂存到期后仍保留，不能删除键再重复创建。

接收开始先授权、核对租约和有效期，再流式读请求；缺字节、多字节、客户端SHA不匹配、格式不符均不进入ready。租约内每10秒续约，失去epoch或超过120秒立即中断。断网／进程崩溃留下`receiving`时，在30秒租约过期后由状态查询／下一次显式PUT的短事务安全转为`interrupted`；读请求不触发重新传输。重传从0开始，不支持`Content-Range`断点上传、不拼接半截正文；`Content-Encoding`仅允许缺省／identity。

每个epoch写独立临时文件／暂存对象；边读边算SHA、实际长度及类型。验证后把字节存入不可变内容地址对象，核对存储长度与摘要元信息，再用当前epoch的短事务公布ready及预览摘要。旧epoch不能覆盖ready，暂存对象存在不代表已收录。已经ready的PUT重发不再读取请求正文，返回409 `upload_already_ready`并指向GET恢复，不用未验证请求体冒充同内容回放。

暂存是SQL状态＋现有MinIO对象，跨API进程可恢复；接收中的本地临时文件不作为持久权威，重启后只允许完整重传。用户／域撤权后404且不继续接收／转ready；策略变动409，终止旧上传；到期410。取消与ready／确认共用域→上传／幂等记录的锁序，取消先提交则确认不能发布；确认先提交则取消返回已committed，不撤销修订。物理删除暂存／孤儿对象归R6保留策略，本轮只释放逻辑配额与临时文件，不删已保存对象。

`rejected/cancelled/expired/committed`释放未结束名额与声明字节预留，`interrupted/ready`仍持有至终态。接收中的取消先中止epoch；部署接收槽等该epoch停止或租约过期才释放，不允许取消后旧传输与新传输绕过槽位叠加。暂存过期的物理对象不因逻辑名额释放而被宣称已清理。

## 6. 固定预览与原件确认

`ready`返回服务端真实SHA／长度／媒体类型、输入期望头、分类、action、固定截止和`manifest_sha256`，不带正文或存储句柄。确认摘要使用长度前缀token编码：每个UTF-8字符串写为`十进制字节数:原字节`，连接后SHA-256；首token `knowledge-original-preview/0.5`，投影见OpenAPI。null固定为空token，数字为无前导零十进制；不做BOM、换行、Unicode规范化或隐式trim。绑定主体、域、登记、上传、策略版本、精确路径、完整分类、预期头、实际媒体类型／大小／SHA、action及截止。

确认先查原键回放，再检查上传未过期且ready、摘要完全匹配、明确确认、当前域／组／登记／策略权限和预期来源头。文件已ready后本机文件变化不改变这次已上传字节；重新上传须新会话。SQL短事务只把**已经验证和持久保存的对象**关联到原生Document／File、FileCommit／Item、来源修订、绑定及不可变回执，并推进来源头；不在事务里接收或解析大文件。

对象成功而SQL失败可留下未引用对象；不能创建成功回执或推进来源／原生文件夹头，不能自动清对象。确认提交后丢响应用原确认键查询；查询与确认串行化，等待≤10秒，无法确定返回503 `result_unknown`。404只表示当前不可见／无已确认结果，不证明未提交；不自动POST、不换键。由人明确选择“重发原确认请求”才重发同键同上传同摘要；已提交回放不依赖过期暂存和当前来源头。

首版确认为一个短SQL事务的同步结果，无新的解析队列或批量worker：`status=succeeded`且`save_status=created/updated/unchanged`，否则明确错误、无成功指针。原件对象校验／存储在ready之前完成；确认不再copy整件对象。失败回执查询与未知要分清，不能捕获网络异常直接显示“保存失败”。

回执中嵌入的来源元信息是确认时快照，`is_current`只反映当时状态；回执不可因新版本重写。固定修订详情的`is_current`反映本次读取时当前头，不能据该字段把回执改链到current。历史首版通过已保留的固定回执／修订链接进入，不新增原件历史列表。

## 7. 权限与固定读取

每次创建／接收／ready／确认／恢复／列表／HEAD／GET都复查当前登录、有效租户成员、域所有者、有效知识域、登记归属和本版禁外发策略；组与路径仅组织信息。知道任意ID、SHA、ETag或请求键不能授权；不存在／越域／越用户统一404，不通过Range错误泄露长度。数据存储故障503，授权查询故障失败关闭。会话内续传和状态GET也不能续用旧授权。

保存后只按当前知识域授权读取，固定修订指向不可变原件，不依赖本地上传文件或暂存TTL；旧版本和回执均能恢复。原生file下载、folder history、通用link／parse／delete／移动及子树操作继续覆盖新原件来源保护，不能绕过私有域／禁解析规则。

GET／HEAD先完成授权与对象身份检查，再产生内容头；响应包含服务端最终`Content-Type`、以固定`original_filename`安全编码的`Content-Disposition: attachment`、`X-Content-Type-Options: nosniff`、`Accept-Ranges: bytes`、整件强ETag `"sha256-{sha}"`，无预签名链接。整件与区间都`no-store`。HEAD返回整件头和Content-Length，无正文；忽略Range／If-Range。GET单范围支持`bytes=a-b/a-/-n`，末端超过EOF截到EOF，越起点／空区间／语法错误／多区间统一416并带`Content-Range: bytes */N`；授权失败不返回此头。若带`If-Range`且不等于强ETag，则忽略整个Range返回200整件（弱值、日期或其他值均不匹配）；匹配后才检查范围。没带If-Range按正常范围处理。本版不支持304／If-None-Match。Range只对固定修订字节计数，不转换UTF-8，客户端不得把部分范围SHA当整件SHA。

原件对象发布后禁止本应用覆盖同一内容地址；完整性元信息随ready验证固定，范围响应不声称重新计算了整件SHA。读取前stat长度／固定摘要元信息不符409 `original_integrity`，中途读取失败中断连接，客户端不得把截断200当完整下载。实施时须验证应用层保护和MinIO写入路径；对象服务管理员越权改写不在本POC威胁承诺内。浏览器下载携带同一原生Authorization，不在URL放令牌；首版明确采用既有认证请求的≤64MiB Blob下载，浏览器会缓冲整件，不宣称客户端恒定内存或零拷贝；服务端流式／Range单独验收。下载前校验HTTP状态、媒体类型、修订ETag与长度，整件Blob还核对整件SHA；错误JSON限32KiB解析，不能将登录页或错误Blob保存为原件，释放objectURL。浏览器流式直接落盘作为后续独立优化。

原件列表按修订ID升序，每页20，游标绑定主体／域／分组／original_only类型，600秒有效，不接受客户端页大小；逐页重新授权。它是当前头的游标视图，不是原子清单，导入期间发生头变化应刷新第一页，不冒充0.4固定项目版本。认证中间件可保留原生错误包络，客户端先按HTTP401处理；进入0.5处理器后使用本版结构化错误。

HEAD的成功及错误均无正文；0.5处理器错误通过`X-Knowledge-Error-Code`表达机器码，不能尝试JSON解码空正文。原生认证层401可无该头，按HTTP状态处理。GET二进制失败才使用有界JSON包络。若Range请求因If-Range不匹配收到200整件，不得当局部块拼接；只有整件分支可接收并校验完整长度／SHA。

## 8. 错误、客户端恢复及实现顺序

机器码见OpenAPI，不匹配message判断：401停止自动请求并重新登录；404隐藏缓存并要求检查当前主体／范围；409按`upload_busy/upload_already_ready/revision_changed/manifest_changed/policy_changed/idempotency_conflict`区分；410为到期；413大小／总配额；415格式；416范围；429接收槽位已占满；503为存储或结果未知。元信息显示实际状态，不把未解析／未索引当失败。

本地恢复分开记录：创建槽在POST前保存主体／域／登记／0.5协议、完整有界`CreateUpload`原请求及创建摘要、原键和已知上传ID；它只含**一个文件的原名／逻辑路径／分类等元信息**。确认槽在POST前保存原`{upload_id,manifest_sha256,idempotency_key,confirmed:true}`和已知回执ID。较旧版本仅存指针的槽，本版为恢复创建原请求明确增加上述单项元信息，不含原件／正文／令牌／本机绝对路径／整个登记路径列表。读本地记录先做schema和主体范围校验，不把本地存储当权限证明。上传by-key回包须匹配创建键及`request_sha256`；回执须匹配域／登记／上传ID／确认键／manifest，不只认任务ID。旧结果未明确前不覆盖槽或生成新键。

登记POST前仅存配置键及配置摘要；丢响应先GET按registration_key恢复。查询仍404时不得自动重建，允许人重新输入原配置、核对与原配置摘要一致后显式重发同配置POST；不以“404”推断原请求从未提交。登记原请求／返回配置的规范摘要采用同一长度前缀编码，首token `knowledge-original-registration/0.5`，绑定主体／域／配置键／名称／来源类型／输入模式／排序路径数与逐项路径／默认分类，具体投影在OpenAPI。所有按键查询与对应创建／确认使用同一域锁序，等待≤10秒返回503 result_unknown，不假报处理中无结果。

换用户清可见缓存，重登按原键重新授权。`receiving`前台每2秒GET，连续3次网络／503停止自动轮询并显示“结果未确认，手动核查”；关页停止GET。手动“核查原结果”只GET，成功后恢复正常状态；若仍receiving可恢复前台轮询，重传／确认仍须各自明确操作。`interrupted`要求用户重新选择文件并显式完整重传，服务端核对固定原名／长度／可空客户端SHA；未提供客户端SHA时，同名同长不证明字节相同，以本次ready实际SHA为准。原件“取消”只影响本次未提交暂存，不能删除历史。

| 顺序 | 单写入责任 | 通过条件 |
| --- | --- | --- |
| R4-C 候选 | A写契约与计划；消费会话只读核对具体客户端可消费性 | 字段／行为／摘要向量一致，17受保护资源不变，未知和范围明确；须真人确认再冻结 |
| R4-A1 存储与状态 | A写Go流式原语、暂存／lease／配额及OOXML有限接纳；沿用现有MinIO | 不使用整件`ReadAll/Get/Put []byte`接大文件，合成64MiB原件内存曲线与槽位／超时／崩溃可测；不接解析 |
| R4-A2 原生发布与读取 | A写统一快照元数据核心、original_only投影、原键／CAS和授权Range | 两版SHA一致，失败不推进头；旧C1不泄漏／不接原件，原生旁路拒绝，旧冻结链路回归 |
| R4-B 客户端／页面 | A明确交写权后由消费会话实施；当前只读 | 上传／元信息／明确确认／回执／固定下载和异常恢复；不把二进制送C1，不自动确认／重发 |
| R4-I 合成真实联调 | 回交清洁版本后A集成同版Go和Web | 下节全部工程验收执行并保存；仅通过后按既有main推送授权发布，不以Mock代真实链路 |

## 9. 工程验收与后续边界

候选确认后才执行以下工程验收，本文件不声称已经通过：

1. 合成有效PDF、PPTX、DOCX各两版和3MiB／64MiB文本原件；客户端输入、ready、固定下载SHA完全一致，BOM／CRLF／emoji原字节保持；旧版不随新头改变。
2. 截断／超长／错误声明SHA、伪扩展、恶意ZIP路径／宏／加密／压缩比、凭据标记、Content-Range／压缩请求均明确拒绝，无Document或头推进。
3. 并发PUT只接受一个epoch；上传进程中断、30秒自然租约恢复、完整重传、同键竞争及关页后原键恢复；旧epoch迟到不能ready。确认前后取消、到期、策略／预期头变化覆盖。
4. 对象写失败和写成后SQL回滚不推进原生文件夹／来源头；提交丢响应回放同一回执，创建／确认同键异请求409；同源不变不增修订。
5. GET／HEAD／单范围／suffix／If-Range／416及中途截断，未登录／跨域／跨用户／撤权均先授权；通用File／历史／parse/link等旁路纳入定向回归。发布对象不可覆盖路径有证据。
6. 既有0.2／0.3／0.4、C1文本／项目版本链路回归；目录页面继续同一分组和标签，原件明确无C1根／无全文／无向量／无解析。原0.4清单拒绝原件引用。
7. 记录3MiB、32MiB、64MiB各3次的上传／ready／确认／下载耗时、吞吐与基线／峰值RSS／临时磁盘；验收应用显式缓冲≤1MiB、无按文件大小扩张的Go正文分配路径。目标本机64MiB合成吞吐≥5MiB/s且确认SQL≤2秒；未达标记录并修复，不伪装生产p95。原代理缓冲设置、超时及实际MinIO须一起核验。
8. 旧POC6份／42块、原模型配置／用量和ingestion计数不变；模型实际请求0，服务仍使用既有Compose与卷，凭据不进Git或构建上下文。

R5网页／RSS、R6生命周期／保留与R7阶段2退出分别推进；阶段3才实现派生正文、页／章节节点、AST／关系与阅读保真。新原件不会自动加入旧0.4项目清单；混合清单和受控目录原件采集另提小候选。当前真实资料和模型仍禁止，不以连续推进指令代替后续放行。
