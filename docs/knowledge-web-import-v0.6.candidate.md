# C2 0.6.0 网页／RSS 标准来源候选

日期：2026-10-26。状态：**ReviewCandidate；未冻结、未接入、未部署**。运行基线为 main `7c564a738597e924e411f3b191aaa02d14ae42d8`。本文、[OpenAPI](knowledge-web-import-v0.6.candidate.openapi.json)、[合成规则向量](knowledge-web-import-v0.6.candidate.examples.json)共同定义候选；[验证记录](stage2r5-web-contract-verification.json)只说明候选检查，不代表应用、网络、RSSHub或性能验收。C1/C3、C2 0.2—0.5及历史候选23项保护文件保持原字节。

## 1. 目标、依据和选择

用户已要求网站通过原生 Feed／RSSHub 纳入同一知识结构，并持续推进来源进入系统。R5的完整交付是：**已有知识域／分组→登记固定网页或Feed→明确取得一次快照，或上传已有快照→固定观察／精确选择→安全阅读及逐项认可→原子确认→固定回执／修订**。本地快照导入只是其中一种输入，不代替在线取得。保存不触发分块、索引、Embedding、模型、后台订阅或删除。

已核实：[原生RSS解析](../internal/syncer/connector/rss.go)可解析RSS／Atom，但其标题fallback、MD5身份、纯文本渲染和prune不满足本项目；[离线准备](../internal/syncer/connector/knowledge_source.go)保留HTML／Feed与代码表格，但未接业务、并非安全渲染器，作者／类别尚未纳入输入指纹。[安全取得原语](../internal/utility/upload.go)已有每跳URL检查／IP钉选，但现有逐跳超时与有界ReadAll不等于本版整体预算。R4已交付不可变对象与原生快照元数据核心，能复用同一存储和版本体系。

选择同时提供 `local_snapshot` 和 `explicit_fetch`，共同进入唯一标准化／保存路径。保持现状无法收录保真的网页条目；只做本地导入缺少输入网址后的取得能力；直接接通原生RSS同步器会带入自动解析／删除和不稳定身份。首版不用JS浏览器、站点登录、递归爬虫、定时同步或另建网站知识库。

标准依据只用于解析语义：[Atom RFC 4287](https://www.rfc-editor.org/rfc/rfc4287.html)支持text／html／xhtml与外部content、规定ID及xml:base语义；[XML Base](https://www.w3.org/TR/xmlbase/)规定嵌套基地址继承。本项目另选更小的接纳范围，不能宣称完整标准实现。[RSSHub](https://github.com/DIYgod/RSSHub)是Feed生产端；本项目接其RSS输出，不复制其路由引擎。官方在线指南本轮读取403，未据此推断具体路由可用性。

## 2. 范围、容量和权限

以下为**待确认默认值／未来验收门槛**，不是实测结果。

| 维度 | 明确规则 |
| --- | --- |
| 格式 | 非空UTF-8 HTML、RSS 2.0、Atom Feed；可带原样保存的UTF-8 BOM；不接Atom单条Entry、RSS 1.0、非UTF-8、NUL、DTD、实体声明或外部实体；XML必须完整闭合 |
| 原快照 | 1—2,097,152字节（2MiB）；不解压。媒体与内容根格式不符拒绝，不因HTTP200推断格式正确 |
| 预算 | 每Capture最多1000个原条目、HTML/XML深度64、总解析节点50,000；超限整体观察失败，不静默截前1000条 |
| 条目及总额 | 每条解码正文≤2MiB、安全阅读≤2MiB、元信息投影≤8KiB、引用≤256项且≤512KiB，单条引用JSON≤16KiB；整个Capture解码正文≤4MiB、阅读≤8MiB、元信息≤1MiB、引用≤4096项且≤2MiB；超限观察失败 |
| 选择／确认 | 1—32项，条目键唯一并按键排序；所选正文＋阅读＋元信息＋引用总字节≤8MiB；先生成对象，再一次短SQL全部提交；不存在部分成功 |
| JSON／分页 | 请求和响应均≤32KiB（包含UTF-8序列化／转义及包络），未知字段／重复键／尾随JSON拒绝；列表最多20项，按实际序列化预算缩页，不以截断字段凑数；有剩余时游标必须前进 |
| 内容读取 | 完整表示是独立二进制响应，≤2MiB；UTF-8窗口独立JSON端点，最多4096原字节、边界必须对齐；原快照只下载，不直接执行；不套用0.5 Range协议 |
| 寿命 | Capture创建起固定30分钟；唯一观察同期限；选择预览有效期=min(创建起10分钟,Capture截止)，GET不续期；回执、原键和固定修订永久逻辑保留，本版不物理GC |
| 并发 | 每主体／域最多2个未结束Capture、预留原字节总额≤4MiB，正在取得／上传／解析最多1个；全部署此类活动最多2个，以SQL租约协调。与0.5上传槽位分别计量，不宣称跨类型总槽位 |
| 时间 | 本地上传30秒、在线取得15秒、解析10秒；均从本次尝试开始计整体时间；30秒租约／5秒续约，数据库时间，丢epoch后不得公布结果；HTTP等待不超过25秒，超时先查原结果 |
| 自动化 | 模型调用0、解析只做确定性格式标准化；结构节点／图关系、分块、全文／向量、订阅调度均未启动 |

所有接口仍为原生登录的**当前私有域所有者**，同时核对登记／Capture／观察／预览／修订属于同域同主体、当前域有效、策略版本和分组。标签不是授权。401／404、策略或契约冲突立即停止轮询并清敏感界面；每页／窗口／二进制读取重新授权，`Cache-Control: no-store`，不返回对象键、桶、临时路径、预签名URL、DNS结果或内部地址。

当前C1策略DTO不新增字段。`explicit_fetch`另需本版登记的明确网络范围和每次执行的`confirmed=true`，不以`external_models=false`推断允许或禁止网络取得，也不修改它。知识正文、问题、Cookie、会话令牌、模型Key都不能成为取得请求头／正文。未来MCP的条目读取授权不自动包含整个Feed原快照；本版原快照接口只给域／Capture所有者，不提前开放MCP。

## 3. 登记、网络取得和来源证明

登记：`source_kind=web_page/web_feed`、`origin_url`、`producer=direct/rsshub/other_feed/browser_export`、默认分类、不可变`network_scope`。域内登记键逐字节区分大小写／Unicode、不trim／NFC化；同键同规范配置回放，异配置409。只允许取得登记的**固定起始URL**，Capture不能传任意新URL。不同登记是不同来源范围，即使URL／GUID／字节相同也不合并。

`origin_url`是绝对HTTP(S)地址，无userinfo／fragment／控制字符、ASCII主机（国际化域名先使用punycode）、只用HTTP的80／HTTPS的443默认端口；URL规范规则为scheme／host小写、删除默认端口、空path变`/`、解析相对路径的点段、保留path与query的转义及顺序，不解码后再归一化。拒绝明确凭据查询名（忽略大小写的key/token/api_key/access_token/password/secret/signature），这只是有限拒绝，不承诺完整秘密识别；带鉴权参数的RSSHub路由不在首版范围。

`network_scope.enabled=false`时只接本地快照，`allowed_origins=[]`。开启后明确列出1—4个规范origin，包含起始origin，严格按scheme＋host＋有效端口匹配，无通配符；重定向可在这些origin内改path／query，不能跳出。每次`POST .../acquire`必须携带登记的`scope_sha256`和明确确认；恢复界面重置此确认，不自动重新取得。

在线首版只发匿名GET，`Accept-Encoding: identity`；Cookie／Authorization／Referer／环境代理／用户自定义头一律不发送。最多3次重定向（最多4个HTTP响应），每跳及每次建连重新检查当前授权、URL、全部DNS候选均为允许的公网单播IP，并把已验IP钉选到拨号、保留正确Host／TLS主机校验。拒绝loopback、RFC1918、link-local、metadata、非公网／特殊用途地址与混合安全／危险DNS结果；不提供生产绕过开关。整体15秒包含DNS、跳转、TLS、响应和正文，不因跳转重置；响应头≤16KiB，原体≤2MiB，无自动重试。压缩、非200（含204/304）、错误类型／编码分别返回确定错误；304不代表“未变”，本版不用ETag／Last-Modified跳过正文比较。

创建Capture不会发网络请求；只有随后明确`POST acquire`从`awaiting_fetch`取得一次。一个在线Capture最多一次取得尝试（包含跳转），已开始过的重复acquire只回原状态、不重新联网；从未开始且已取消／到期的新写返回410。已知失败为rejected，进程中断／结果未知待租约归一后为interrupted，在线二者均不可重试；先GET取得确定状态，再由人重新认可、新建Capture。不得以“恢复”重放第二次网络请求。acquire首次CAS带`expected_capture_revision`；原状态回放不因旧revision重施动作，仍检查scope与当前权限。

在线Capture的`provenance=server_http`保留实际requested/final URL、成功时间、200状态、最多3条跳转和有界Content-Type／ETag／Last-Modified声明（不作真实性认证）。本地Capture为`user_supplied`：用户声明requested/final URL及claimed_captured_at，`server_received_at`另存；HTTP状态／响应头／服务器取得时间为空，不能补造。Feed条目的HTTP完整性不能从Feed这次200推断。

本轮冻结后工程仅访问**合成传输／合成HTTP服务**。真实AIHOT、RSSHub公共实例、私人站或AICS均不因本候选自动获准访问；不在测试配置关闭SSRF。测试用私有注入transport／resolver验证公网语义与本地HTTP夹具，生产构造器不能注入任意拨号。

## 4. 身份、表示和修订

- `source_id=原生Document ID`；`source_revision_id=FileCommitItem ID`；沿用同一知识域／分组／绑定／SourceRevision。Web私有表示`web_capture`不建立C1根节点（`root_node_id=null`），C1继续只读text并对其他表示返回既有415；0.4清单和0.5DTO不加字段。
- 网页只有一项，逻辑身份为本登记的`page`（identity_basis与identity_value均为page）。Feed优先非空RSS GUID／Atom ID解码后的原字符串，视为不透明值，不trim、大小写折叠、URL化或Unicode规范化；API `entry_key`使用第7节entry_key投影的typed-LP摘要（指定前缀＋含identity_basis/identity_value的对象）。缺ID时以有效解析链接的规范定位（去fragment）作`weak_url`；只有标题的项列为不可选，不能用标题造身份。同键同版本投影折叠并记录原位置，异投影使整个观察409冲突，不按第一条覆盖。
- 原生逻辑路径为登记私有目录下`web/<entry_key>.html`（HTML/XHTML）或`.txt`（text／仅引用），标题只是展示名；身份不是路径猜测、上传ID或SHA。正式存储绑定登记＋条目键，碰撞／旧绑定类型不符拒绝，不靠SHA合并来源。
- 原网页／Feed XML为`raw_capture`，SHA覆盖原字节。`decoded_content`是XML一次解码后的所选HTML/text（Atom XHTML为确定序列化的内容片段），不是原始条目XML；定位保存Capture＋原条目序号集合，不伪造XML字节范围、页码、行号。原快照中的换行/BOM与实体写法可不同于解码片段，均各有SHA与长度。
- `safe_reading`为本版受限HTML和转换版本，SHA／长度独立；原生File/CommitItem记录已验解码片段对象（仅引用项可为0字节），Web修订另固定原Capture指向、结构元信息与安全阅读对象。取得时间、别的Feed项变化或条目顺序不强迫本项增版；unchanged回执仍指向本次Capture以保留新观察证据，旧修订的首次来源证明不改写。
- 来源输入投影含条目身份、解码SHA／类型／长度、安全阅读SHA／版本、规定元信息／引用、有效base、分类；元信息包括标题、作者（有序，≤8项）、类别（≤16个按UTF-8字节排序去重的原字符串集合）、published/updated原声明、canonical声明、完整性。作者／类别变化增版；相同／倒退／缺更新时间仍逐次比正文，不能按时间跳过。canonical缺失为null，与“明确声明同一URL”不同，canonical不改身份或抓取范围。
- 当前头输入相同为`unchanged`，不同为`updated`，未绑定为`created`。回到历史相同输入复用既有修订ID；修订ID不是操作世代。本版不做移动／归档，R6须另有生命周期版本。转换器升级不回写历史；只有明确的新观察／预览／确认才能公布新版结果。

完整性只有`summary/unknown/external_reference`。RSS description为summary，content:encoded和普通网页为unknown，Atom外部content src为external_reference；不提供自动full。外部content只引用，不追抓；若同时有summary，保存其解码正文并仍标external_reference。纯引用项解码为空、阅读显示明确占位，必须逐项认可“只有引用”，不能显示全文。空的普通正文没有可用引用时不可选。

观察条目中的`source_input_sha256`按登记默认分类计算，来源头也是观察时的固定事实；创建预览时按用户完整分类重新投影并验证显式期望头。分类更改不能复用旧输入摘要；观察GET不随当前头更新。独立`GET web-registrations/{id}/target?entry_key=...`只读取本登记绑定的实时头／输入摘要／分类，不改变观察或网络；同一Capture分次选择或头冲突后，可明确刷新目标头再创建新预览，不必为取得头重抓站点。引用按发现顺序稳定去重，`reference_id`为规定typed-LP投影SHA；missing身份项仅用`identity_basis=missing/identity_value=position:<原序号>`生成本观察内展示键，永不建立来源绑定。

## 5. base、引用和安全阅读

网页以实际final URL（本地为声明final，否则requested）为回退，采用文档顺序中第一个可解析HTTP(S) `base[href]`；多个base记警告，canonical不作为base。Atom按各元素继承`xml:base`，先以上层有效base解析当前声明，再解析该元素的URI。RSS扩展的xml:base采用同样规则；无声明回退Feed final URL。Feed内嵌HTML继承内容元素的有效base，其首个有效HTML base可覆盖该片段，不套用网页canonical或任意条目link。无效／危险base不采用并警告，原声明留原快照。

每条引用保存`raw_href`、有效`base_url`、带fragment的`resolved_url`（不能安全解析时null）、`fragment`、关系／标签／媒体声明。身份使用的定位去fragment，阅读引用不丢fragment。HTTP(S)引用可由父页明确打开，`mailto`只作文本，其他危险scheme只作文本警告；不下载附件、不转模型。发现范围为a[href]、img[src]、RSS enclosure、Atom link／外部content；不宣称穷尽CSS、srcset、脚本资源或所有附件。

安全阅读采用**严格白名单HTML**：p/h1—h6/ul/ol/li/blockquote/pre/code/table/thead/tbody/tfoot/tr/th/td/br/hr/strong/em/del/sub/sup/span；仅允许受限rowspan/colspan（1—100）、受限code语言和生成的引用标记。保留pre/code文本换行缩进、表格顺序与合并单元格。删除script/style/form/iframe/object/svg/math等执行／资源节点及全部事件、style、href、src、srcset、外部资源属性；图片变带alt／引用标记的文字。原HTML始终只下载或按转义文字显示。

后端完整阅读响应和前端iframe均使用`default-src 'none'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'`，只准生成器固定样式，所有输入样式删除；后端另加`sandbox`、`frame-ancestors 'self'`和nosniff，iframe `sandbox=""`且不允许脚本／同源／弹窗／导航。前端先经已认证请求取得≤2MiB、校验SHA，再以固定CSP的srcDoc显示，不把原HTML注入主DOM；不使用会自行加载合法远程图片的SafeImg。引用由父页独立明确操作，带noopener/noreferrer，不在预览／恢复时自动取得。

二进制表示端点不接受Range，收到Range为416；Content-Disposition原快照固定attachment，safe_reading只能按上述CSP取得；JSON窗口基于完整表示字节、UTF-8对齐，空正文仅允许[0,0)。完整表示带encoding、整件SHA／长度、converter/sanitizer/resource_policy版本；窗口绑定固定entry／revision和完整表示SHA，取齐验证后才能渲染，半截HTML不得直接渲染。空sandbox不依赖子页面脚本／postMessage或同源DOM桥接，首版固定高度内部滚动，引用编号对应父页独立列表。meta refresh/base/link及所有导航／提交触发属性均不能进入安全表示。动态页/窗口预算包含包络；任何单项无法合法装入也不能静默裁剪。

## 6. 固定资源、恢复和确认

Capture创建原键→上传／取得→唯一观察→选择预览原键→确认原键，**四个资源身份各自可恢复**。所有键逐字节比较；创建同键同请求回200原资源，异请求409；回放先于TTL／容量检查但仍重新授权，不重施工作。恢复槽只在本机保存当前用户／域／登记与有限请求元信息、Capture/观察/预览/回执ID、≤32项选择／分类／期望头及原键，≤32KiB；不存原文、整个Feed、Key或登录令牌，不因恢复自动发写请求。Capture创建即钉选normalizer、converter、sanitizer与resource_policy版本；恢复GET不能用部署后的最新解析器重解释旧字节，不支持的生产版本拒绝新工作但旧固定表示仍可读。

| 资源 | 状态／恢复规则 |
| --- | --- |
| Capture | `awaiting_bytes/awaiting_fetch/receiving/fetching/ready/interrupted/rejected/cancelled/expired`。本地仅awaiting_bytes或interrupted可显式从0完整重传；在线仅awaiting_fetch能执行一次，interrupted也不可重取。ready不可再写。创建丢响应按原键GET；模式不匹配拒绝；取消、格式拒绝、到期终态不复活 |
| 观察 | 一个Capture＋固定`normalizer_version`唯一，`parsing/ready/interrupted/rejected/expired`。POST只创建／推进这个唯一资源，GET按Capture发现它，不会因丢observation_id重建；无观察返回404。崩溃租约后显式POST可从同一原字节重试，输出须确定 |
| 预览 | 独立`idempotency_key`和按键GET，`ready/committed/cancelled/expired`。请求完整列明条目键、分类、expected_revision_id；一份固定观察可做多份独立预览，一份预览只能消费一次。不能将“首32项完成”显示为整个Feed已导入 |
| 回执 | 确认携带preview_id＋manifest_sha256＋原确认键＋confirmed=true＋恰好全部所选entry_key的认可集合。回执与其items固定保留；GET或回放不会再次取得、确认或换键 |

`revision`是每个可变资源单调正整数；同资源旧revision响应不得覆盖新状态，同revision不一致视为无效响应。网络／503连续三次状态读取失败后转手动核查；401／404、版本／策略不符立即清敏感状态，不能后台继续。失败含stage、机器码、可空upstream_http_status与结果是否已确定，源站401/403/404只能表现为本系统502并可GET核查，不能触发登录拦截器退出。已committed预览可在Capture到期／取消后恢复原回执；未消费预览则受原Capture当前状态和期限约束。Capture取消只废弃未消费准备，任何已确认历史仍在，固定修订读取不依赖暂存TTL。

每份预览绑定主体、域、登记、配置与网络范围摘要、Capture原SHA、观察ID与normalizer版本／摘要、当时策略版本、完整排序选择、逐项输入摘要／分类／预期头／action、总字节及截止。确认前先在事务外核验所有不可变产物；再按域→登记→Capture→观察→预览→原键→原生文件夹的固定锁序，重新授权／核对TTL、Capture状态、全部来源头、策略和摘要。任一失败，**32项均不发布**，没有半份Document/File/Commit/SourceRevision/绑定或回执。

同一原生文件夹每批只读取／序列化一次版本树，创建一次FileCommit及逐项FileCommitItem，复用已有版本核心，不对32项重复复制整树。对象写入在SQL外且不可变，事务失败留下的对象不是已导入，也不在本版物理回收。确认成功原子保存全量固定回执／预览committed；同预览换新确认键409，不重复导入。取消先取得锁则确认失败，确认先完成则取消返回committed；丢响应先按原确认键查，不自动换键。

窗口缺项、A+B→B、空Feed、过滤缩小、404／超时／解析失败、Feed变址均**不删除、不归档、不移动旧来源**，没有隐式水位／锚点推进和prune。新的当前列表是按当前头的实时分页，不称固定全库快照；固定观察／回执／修订则不随当前头改变。

## 7. 接口、错误与摘要

所有路径位于`/api/v1`；请求同时要求`X-Knowledge-Contract-Version: 1.0.0`和`X-Knowledge-Source-Contract-Version: 0.6.0`；响应回显及request_id。ID32小写hex、entry_key/SHA64小写hex，键1—128安全字符。以下操作均独立于旧版，确切schema／字段必填性见OpenAPI。

| 分组 | 操作 |
| --- | --- |
| 能力／登记 | GET web-import-contract；POST web-registrations；GET登记／by-key／target实时头 |
| Capture | POST captures；GET ID／by-key；PUT content（仅local）；POST acquire（仅fetch且明确确认）；DELETE；GET原体content |
| 固定观察 | POST／GET Capture的observation；GET其entries、固定entry及references；GET entry表示content／window |
| 精确预览 | POST previews；GET ID／by-key；DELETE；GET items |
| 确认／回执 | POST imports；GET ID／by-key及固定items |
| 固定消费 | GET web-revisions；GET固定revision、references、表示content／window与原Capture下载 |

错误包络≤32KiB，无内部路径／URL密钥／DNS结果。400请求非法／长度摘要不符；401未登录；404本系统未授权／资源不在当前域；409键冲突、状态忙、同键异内容、策略／头／manifest变化；410对过期／取消准备的新写入；413原体／配额超限；415格式／编码不支持；416不支持Range或窗口非法；422危险地址／拒绝压缩／解析或结构预算；429并发槽忙；502源站错误（附有界upstream_http_status，**不能冒充本系统404**）；503存储／SQL或结果未知；504在线／上传／解析超时。授权有效时GET终态资源200，过期不删除键；确定失败与结果未知分开，后者只要求先GET。

摘要采用新版本**typed-LP**，不改变旧版摘要：`LP(s)=ASCII(UTF8字节数)+":"+UTF8(s)`；null编码`LP("n")`，字符串`LP("s")+LP(value)`，布尔`LP("b")+LP("1"或"0")`，安全整数`LP("i")+LP(十进制)`，数组`LP("a")+LP(项数)+各项编码`，对象`LP("o")+LP(键数)+按键UTF-8字节排序的LP(key)+值编码`。最终SHA覆盖`LP(指定prefix)+encode(指定投影)`；不接受浮点／未定义，不trim／NFC，不混淆null与空字符串。OpenAPI `x-digest-projection`列出固定投影，向量同时给输入和编码hex。Capture原SHA、观察SHA、选择manifest、来源输入各有用途，不互作身份。

网络origin、类别集合、精确选择／认可按UTF-8字节排序；作者／引用按声明顺序，引用只折叠全投影相同项；观察的entries按entry_key排序，positions按数值排序，包含不可选项（其source_input_sha256为null）。请求键不入内容投影，其隔离作用由主体／域／登记／资源类型的唯一约束承担。`capture_request.local_declaration`在本地模式包含CreateLocalCapture除登记摘要、模式和原键外的全部字段，在线为null；登记／选择请求等投影也只纳入OpenAPI明列字段。元信息／引用的字节预算是其完整UTF-8 JSON字节，不是摘要编码长度。

## 8. 开发顺序与验收

| 批次 | A／消费方边界 | 必须通过 |
| --- | --- | --- |
| R5.0 候选 | A唯一文档／Git写入；消费会话一次集中核对客户端字段，不启动多轮审查 | schema／正反例、typed-LP摘要、身份／增量／恢复／权限规则、23保护文件和文档范围；真人具体批准后才冻结 |
| R5.1 后端 | A实现同一解析／网络适配、Capture／观察／预览／原子保存；不改旧DTO | 合成HTML/RSS/Atom两版；不透明ID／弱键／跨登记；作者与同时间正文变化、空窗不删；格式／实体／预算拒绝；SSRF每跳／DNS／超时／gzip／304；对象／SQL失败无半发布；原键／租约／权限／并发CAS |
| R5.2 客户端 | A固定清洁后端提交后交新会话唯一Web写权；service→hook→page，独立0.6键／恢复槽；A源码只读 | 模式／网络确认、分页≤20／选择≤32、只引用认可、三表示、SHA／受限iframe零资源自加载、三处丢响应恢复、撤权清理；不改共享UI或全局请求栈 |
| R5.3 同版联调 | 清洁交回A后，由A唯一写入做针对性修复 | 真实MySQL／既有对象存储、合成HTTP源、登录浏览器、更新两版／旧版回读、杀进程／SQL失败／同键竞争；旧0.2—0.5回归，旧POC／用量指纹不变、模型0 |

容量采用当前机器同一版本：2MiB原体、1000条Feed、32项原子确认、20条分页及2MiB阅读各独立跑5次，记录中位数／最大值与实际RSS／暂存峰值；本地暖启动观察最大≤2秒、32项确认HTTP最大≤2秒、单次解析进程增量RSS≤128MiB为首期门槛。网络15秒是失败预算，不是源站延迟保证；不把5次最大值称生产p95，不声称百万库性能。失败保留原记录，修复后有针对性复验，不降低原目标凑通过。

RSSHub合成RSS只能证明兼容输入；**真正RSSHub路由生成、所选真实网站、JS渲染、登录鉴权、非UTF-8、304增量、持续订阅与生产负载仍单列未验／后续范围**。本版候选批准不替代真实来源外发／访问许可、R4用户验收或阶段2整体验收。
