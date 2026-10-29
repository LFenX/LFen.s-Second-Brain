> 历史归档（2026-10-28）：仅保留旧阶段事实，不作为正式开发指引，不恢复旧批准、排程或写权。当前资料见[参考基线](../../knowledge-core-reference-authority.md)。

# AR-8 独立完整审查

结论：完整审查发现 **AR-26、AR-27 两项 P1 问题**，需要返修。本轮不构成干净轮次；是否及如何重置计数由主执行者按实际记录计算。没有把 AR-7 的无发现结论作为前提，没有修改产品、契约、公开文档、配置或运行数据。

| 绑定项 | 本轮实际值 |
| --- | --- |
| reviewer | /root/review_round8，新只读审查子 Agent；未再委派 |
| 独立读取开始 | 2026-10-23T22:17:57Z |
| 完成 | 2026-10-23T22:30:56.367Z |
| delivery_commit | 720b88df692af0d77e352c658eaede308af7f89d |
| subject_sha256 | 8415e6faa2f995a74bf16b264c98598999246c14e549b395e13ad337914bb809 |
| requirements_sha256 | a62c5bf2cb663eec13451eff141c3bf3fef57230e5eb380bdcc5a542c29db403 |
| coverage | Complete |
| findings | AR-26、AR-27，均为确认且未在本轮修复 |
| unreviewed_items | [] |
| model_api_calls / real_data_mutations | 0 / 0 |

使用技能：run-governed-product-workflow 的 SKILL.md、commands/verify.md；go-naming 的 SKILL.md、.agents/rules/named.md。先读根 AGENTS.md，按原始需求逐条复核。授权来自 before.json 中用户对本次 A 阶段1交付明确回复“启动审查”，不扩展先前拒绝的旧文档审查。

## 原文与覆盖

直接重读 review/before.json 的 request_snapshot，按其完整字符串 UTF-8 原字节独立计算 SHA-256。不是对本报告或派生清单取摘要。完整原话如下：

> 阶段0方案通过。开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。暂不调用模型，完成验证后推送远端。

引用 S1 为原句“阶段0方案通过。”；S2 为完整原句“开始推进本机A的阶段1，先冻结双方共享的领域模型、节点身份、权限与外发规则及接口契约，再完成最小底座适配，给B整理可直接接续的交接说明。”；S3 为完整原句“暂不调用模型，完成验证后推送远端。”。以下多条引用 S2、S3 时保留这些完整原句及其中先后条件，不用短词裁剪原意。

| 项 | 原句 / 逐条要求 | 状态与证据 |
| --- | --- | --- |
| R01 | S1：保留已通过的阶段0方案 | 已实现；技术方案35章与36.1区分先前 Approved 和当前 Pending，未用本轮审查替代批准。 |
| R02 | S2：推进本机A的阶段1 | 已实现；35.8的1A.1/1A.2与36章、B交接第1/9节对应；没有实现或宣称B页面、MCP、阶段2—6已完成。 |
| R03 | S2：冻结共享领域模型 | 已实现；entity/knowledge.go全文件、Knowledgebase四扩展字段、File内部所有者字段、三目录投影表及加法迁移；原生KB作为唯一domain身份。 |
| R04 | S2：冻结节点身份 | 已实现；Document稳定source、FileCommitItem修订、JSON数组SHA逻辑/实例身份、source-v1/manual-v1；fixture的25字节、SHA及两个身份独立复算一致。 |
| R05 | S2：冻结权限与外发规则 | 部分实现；固定owner_only/catalogue/no_model/no_auto及当前/历史来源交集有实现与实际拒绝；AR-26遗漏祖先版本暴露，AR-27能破坏目录来源事实。不能称全部入口闭环。 |
| R06 | S2：冻结接口契约 | 已实现；C1/C3 1.0.0，14操作与router/handler/DTO逐项对应，126个schema引用全部可解析，C4/C5职责正确；没有代替B确认。 |
| R07 | S2：先冻结，再完成最小原生底座适配 | 部分实现；35章先定义语义，当前机器基线、生产者/消费者说明一致；原生版本、上传、运行装配已验证，但两个已确认旁路须修复。 |
| R08 | S2：给B可直接接续的说明 | 已实现；完整读取B交接第1—9节，含消费顺序、opaque ID、UTF-8范围、错误/409、策略、构建/运行/回退及待页面状态。 |
| R09 | S3：暂不调用模型 | 已实现于本轮操作；本轮仅隔离本地替身，所有模型用途拒绝与正常未注册库对照；既有安全摘要保持。没有独立网络抓包声明。 |
| R10 | S3：完成验证 | 部分实现；同输入完整工程测试/构建/实际运行证据复核，另执行本轮6组不同反例；两项安全断言失败，须返修复验。 |
| R11 | S3：验证后推送远端 | 截至本轮未实现，保留 PendingReviewCompletion；这是已授权的后续动作，不是未检查项。因当前发现问题须先返修复验，审查者未推送。 |

完整重读技术方案35—37章、README、会话接续当前入口/第16—17节、B交接、C1/C3全schema与fixture、原a1报告及当前v0.22报告。35章的宏观模型、组织树/来源树/关系边、A/B职责及阶段1退出判据均核对；七项体验问题、60题、AST/依赖边、B页面、完整生命周期、阶段2—6、临时MCP Key及人格记忆不当作本轮已实现承诺。

## 条件、正反分支与例外

以下“既有”指本轮读取实现、对应测试及已保存输出，未重放运行服务的写入。新执行明确标注“本轮”。

| 条件/例外 | 成立输入、预期与实际 | 不成立/例外输入、预期与实际 | 证据 |
| --- | --- | --- | --- |
| 仅空私有库可注册 | 空me、无连接器/解析历史→注册成功/同category重放幂等 | team、有文档/计数、Connector、历史→409；其他用户→隐藏404 | knowledge.go:47/78，TestKnowledgeDomainRegistrationAndIsolation，28组HTTP摘要 |
| 已注册域固定私有 | owner可读当前域；原生permission改team仍不扩权 | 非owner、跨域父项/来源、普通副本owner不满足源交集→拒绝 | policy_ce.go:358，knowledge_test.go；本轮MultipleOrigins…验证a/b/c均不能绕过双来源交集 |
| 标签不是授权 | frontend/backend/shared/cli/non_code和language独立保存/返回 | 改分类产生修订而不产生Grant；组织parent不能引用Node | entity/knowledge.go，knowledgeSource/CreateGroup全实现、fixture、既有范围测试 |
| source≤2MiB、UTF-8无NUL且扩展名支持 | .tsx/.jsx保名保字节、CRLF，空文本根合法 | 超限、NUL、无效UTF-8、PDF/PPT→上传拒绝或415 | utility/knowledge.go，document/knowledge_test.go，source限额测试；不修改旧POC |
| 路径512/末级256 | 合法POSIX相对路径支持目录 | 超长末级、绝对/../反斜线/非规范路径→400 | knowledge_source.go:33、source限额回归/真实HTTP摘要 |
| JSON≤16KiB且精确单值 | 本轮16384字节含尾空白单对象→200 | 16385→413；name和转义同名\\u006eame→400；未知/大小写/重复/缺必填/null/多值→400 | round-8-probes.log，EncodedKeysAndExactBodyLimit；handler/knowledge.go:55/110 |
| 查询严格且分页限额 | 缺省50、1—200合法，正常游标续页 | 非法编码、显式空整数、未知/重复；本轮limit/lim%69t重名→400 | handler:145/167，原HTTP3组，本轮编码参数反例 |
| 游标绑定且每页授权 | 本轮同旧修订续页在新修订发布后仍读旧节点 | 非owner重放→404；跨新修订重放→400；Document失效后旧游标→404 | 本轮CursorReplayReauthorizesAndBindsRevision |
| 同域同修订、范围含根≤16层 | 合法子节点、UTF-8绝对字节左闭右开、≤65536回读 | 跨域/跨修订→404；历史追加/改不可变身份→409；切UTF-8/越父级→400；超读→413；第17层拒绝 | knowledge_source.go:279/387/422；节点范围/深度既有测试与HTTP |
| 变化需CAS，相同当前输入幂等 | 相同输入返回同修订；合法expected发布一个当前版 | 陈旧expected→409；历史节点保留旧原文，不跳新字节 | source事务、MySQL CAS摘要与commit-fixed.json无分叉 |
| 对象/DB失败不发布 | 本轮对象Put失败时四张版本/目录表均仍1条、旧current不变；恢复后相同CAS重试成功，旧字节不变 | DB节点失败回滚，不产生版本/目录；对象先写可残余，不要求跨存储原子性 | 本轮StorageFailure…；既有SnapshotRollback…；source_snapshot.go |
| 原生版本整批授权且共用Folder头 | 合法嵌套File提交成功，单调时间与最新头保持树 | 同租户异子树/跨租户File整批拒绝、0对象/0版本；受管树及祖先delete拒绝 | file/knowledge_test.go与真实MySQLcommit反例；读取面另见AR-26 |
| 旧任务/副本不是模型放行凭据 | 注册/旧副本六用途、恢复/Load/Execute/Embedding/WikiDirty/consumer在依赖前拒绝 | 普通未注册库与普通任务继续原行为；数据库失败关闭 | permission、task/knowledge_policy_test.go、consumer及service/knowledge_test.go |
| 已注册删除为逻辑归档 | raw/uploaded/snapshotted三模式保留KB/Folder/Doc/mapping/revision/对象，旧副本/队列继续受限 | 普通未注册库物理删除；归档域新目录接口隐藏，重注册不复活 | crud.go:398，knowledge_archive_test.go，archive-live-fixed.json |
| 名称不是来源归属 | 历史多Folder确定性绑定；迁移重放不改原文和旧地址；孤立目录保存私有owner | 仅真实租户根下公共.knowledgebase例外；同名下级、普通同名不认领孤立；明确来源raw子孙继承 | dao/knowledge_folder.go全实现、迁移/目录测试；skills漏口见AR-27 |
| 普通历史不应因当前File缺失被一律拒绝 | 本轮独立普通Folder先真实提交、删除当前File行，历史content仍200且原文正确 | 含已知受限来源的祖先不能用Folder授权扩大，当前实际泄露 | round-8-probes.log:50，AR-26；不要求混合受限祖先必须部分可见 |
| 验证完成后推送 | 修复/复验及审查结束后由主执行者推送 | 当前两项缺陷未修，发布保持待处理 | 当前报告publication与本轮结果；未把未发布记成未查 |

普通原生版本delete引用不存在File.status为已确认基线限制，不是本轮新增问题；不把它当完整生命周期已验。CreateDataset→verifyEmbeddingAvailability→ResolveInfo/driver是本地配置解析，不是远程推理。无历史/注册/marker且已有关联普通文档的目录不按名称反推旧私有归属；任意复制粘贴不在DLP范围。没有强加请求已重叠时的额外隔离级别。

## 14操作与实现对照

基础前缀/api/v1。每项同时核对原生认证、可选版本头、真实成功HTTP/code0、隐藏404/其他稳定错误、schema和DTO；错误500不包含内部SQL。完整参数/schema引用核对数据保存在 round-8-hashes-before.json 与 round-8-verification.json。

| 方法/接口 | 输入/输出与拥有者 |
| --- | --- |
| GET /knowledge/contract | 无查询；Capabilities；handler/knowledge.go:181，只声明C1/C3 |
| GET /knowledge/domains | cursor/limit；DomainPage；service knowledge.go:193 |
| PUT /datasets/{id}/knowledge/domain | RegisterDomain→Domain；handler:215、service:78 |
| GET …/knowledge/domain | Domain；handler:235、service:144 |
| POST …/knowledge/groups | CreateGroup→Group、201；handler:251，校验同域parent/kind/side |
| GET …/knowledge/groups | parent_id/cursor/limit→GroupPage；根parent不传 |
| POST …/knowledge/sources | SnapshotSource→Snapshot、200幂等/201新建；handler:291、service:47 |
| GET …/knowledge/sources | group_id/cursor/limit→SourcePage；缺省全域当前来源 |
| GET …/knowledge/sources/{revision} | 固定Source；history不切到current，Document失效隐藏 |
| POST …/knowledge/nodes | CreateNode→Node、200幂等/201新建；handler:351、service:279 |
| GET …/knowledge/nodes | 必填source_revision_id；parent/cursor/limit→NodePage |
| GET …/knowledge/nodes/{node} | Node；domain/source存在与状态复核 |
| GET …/knowledge/nodes/{node}/content | 可缩小绝对start/end→Content；64KiB、UTF-8、SHA完整性 |
| POST …/knowledge/policy/check | CheckPolicy→Decision；六种purpose仅预检，不授予执行权 |

router/knowledge_test.go对全路径认证与注册，entity/knowledge_test.go对机器fixture身份，handler/knowledge_test.go对拒绝协议；本轮重新手工对照完整14操作及其schema，不以这些派生测试代替原文。

## AR-26：祖先目录版本及变更接口绕过已知来源所有者交集（P1）

位置：internal/handler/file_commit.go:143 的 folderAccessible 仅检查URL Folder；其调用者183/247/365/440/489/542/579/628在返回树、条目、diff、统计、内容或继承TreeState时没有检查引用的受限File/Folder。internal/service/file/file_commit.go:62、816、821、921、1009、1112按Folder/Commit直接返回数据；router.go:456—493三组原生路由共用这些handler。

复现条件与步骤：

1. 在隔离SQLite/MemoryStorage创建普通用户copy-owner的workspace及nested文件。用真实FileCommitService.CreateCommit在祖先workspace执行add/modify，两次成功并写入真实合成版本对象，未手工伪造FileCommit或blob。
2. 保存旧版可留存的显式File2Document→注册私有域关系，source-owner是该来源所有者。该关系对应旧原生File链接在AR-06修复前允许而当前明确承诺继续约束的历史状态；当前复现不是运行新链接，也不是任意粘贴内容。
3. 当前checker对copy-owner精确File及nested Folder读取均返回ErrPermissionDenied；对更上层workspace的原生Folder读取仍允许。
4. 经真实FileCommitHandler和真实版本服务请求下表，均HTTP200/code0并包含受限内容。加入无历史但含已标记私有子树的独立workspace，其changes也泄露raw名称。
5. 对普通根文件作普通修改，CreateCommit仍返回继承树，含受限文件ID/名称/哈希；没有修改受限文件本身。另一个完全普通Folder的已删File历史读取成功，作为修复保留正例。

| 暴露面 | 本轮实际差异 |
| --- | --- |
| GET commits/{id}/tree | 返回private-source.txt、FileID、hash、对象位置、大小和子目录 |
| GET commits/{id}/files/{file}/content | 返回“synthetic private revision”原文字节 |
| GET commits/{id}/files | 返回受限FileID、hash、对象地址及版本信息 |
| GET commits/{id} | files及file_count包含受限项 |
| GET commits/diff | 返回受限FileID/文件名/old/new hash和对象地址 |
| GET changes | 返回受限raw子孙新文件名；另无任何commit的workspace也泄露名称 |
| GET commits | total及FileCount包含受限来源；直接在未过滤集合计数/分页 |
| POST commits | 仅修改普通文件也返回含受限来源的继承TreeState |

预期：URL祖先Folder原生授权不能扩大已知来源所有者交集；上述接口不得披露不满足来源约束的名称、计数、修订、原文或树。可整体隐藏含受限来源的祖先版本面，不强求部分返回。应同时考虑当前子树、该Folder全部历史TreeState的File/parent引用及FileCommitItem.FileID；已删普通File而无受限来源事实时，保持原生Folder授权后的普通历史可读。

影响边界：确认的是原生版本与变更接口实际披露合成数据，没有尝试真实内容或模型请求。当前单File版本目录GetFileVersionHistory:677先按精确File授权；直接File下载(file.go:438→GetFileContent)及Document下载(document.go:1179)已有精确来源/数据集检查，不能以它们的成功拒绝推断上述祖先版本面安全。Dataset版本路由额外有数据集权限/Folder解析，仍须保留；所有共享handler均需统一处理。未检出commit restore/revert接口，不杜撰已存在的恢复入口。

证据：round8-handler-counterexamples.reference.txt；round-8-probes.log:50、83—133（其中最终安全断言失败，完整确切输出以日志为准）；round-8-verification.json中的失败事件2026-10-23T22:25:32.993766556Z。首次人工插入版本的初始探针原件保存在round8-initial-history-probe.reference.txt/round-8-probes-initial.log，最终判断使用上述真实CreateCommit强化版。

## AR-27：skills初始化去重删除遗留受管目录并解除raw来源限制（P1）

位置：internal/service/file/file_folder.go:176—199，initSkillsFolder第一次按name/parent/tenant查询未排除KnowledgeFolderIDs，后续去重循环直接重挂子孙、删除重复目录；listFiles:87在自有租户根首次浏览调用。Dataset UpdateDataset:390按持久KnowledgeFolderID同步改名，名称校验只针对KB同名，不针对原生root已有skills。

复现前置为**遗留可识别受管目录位于租户根，且与普通skills同名**。初始a1版本MoveFiles没有当前受管树门禁，允许已有目录迁到根；迁移会保留/绑定可识别来源目录，当前没有将所有旧目录强行搬回公共容器。单次旧Move若目标已同名会被原生命名检查拒绝，不能据此声称一次move必然产生同名：受管目录先以不冲突名称位于根，之后当前Dataset改名按绑定ID直接改成skills，或已有旧并发重复状态，均是该初始化必须安全处理的边界。独立运行反例只构造该持久前置并调用真实拥有者函数，不宣称本轮在运行库重放了整条旧迁移路径。

隔离测试创建普通skills与绑定KnowledgeFolderID的受管skills，普通项create_time=1，受管项=2，raw子文件属于受管Folder。调用前CheckKnowledgeFileTransfer(raw)为ErrKnowledgeIngestionDenied；调用真实initSkillsFolder后：

- 受管Folder行数变为0。
- raw被循环移到普通skills目录；该初始化不更新KB绑定，持久ID指向已删行。
- 相同raw的CheckKnowledgeFileTransfer变为nil，明确来源目录的限制已丢失。

预期：普通系统目录初始化不能采用、重挂或删除受管/孤立知识目录；不能破坏绑定与raw来源约束。该入口第一次同名查询与全部去重写入均须保证候选为普通目录，不能只保护手工Move/Delete；不要求重写已有目录结构。

真实影响仅以上精确事实：未执行新链接、模型、正文下载、实际外发或运行数据修改，不能扩大为“每种正文均已公开”。与AR-26不同，这是内部初始化写入破坏来源事实，单纯给版本入口补授权不能修复。

同类名字入口已核对：DAO.InitDatasetDocs和newAFileFromDataset在候选查询排除KnowledgeFolderIDs；Document.newAFileFromKB及普通版本Folder解析也排除；原生名称去重/普通删除有对应回归。发现漏口为独立initSkillsFolder路径。首次探针未安装common.Logger导致本地替身panic，保留round-8-skills-fixture-failure.log；补zap.NewNop后确认业务差异，不将夹具panic当产品问题。

证据：round8-file-counterexamples.reference.txt；round-8-skills-probe.log:33—38，失败事件2026-10-23T22:26:57.119943724Z。

## 前25项处置逐项复核

下列“原目标修复成立”仅表示相应实现/断言在当前输入成立，不将概括性“所有入口”推为全闭环。本轮新增旁路单独登记26/27。

| 原项 | 当前复核依据/处置 |
| --- | --- |
| AR-01 | Agent adapter:227在模型构造/翻译/metadata前guard，构造/增强替身0次；原目标修复成立。 |
| AR-02 | knowledgeObjectFields解码精确key且去重；本轮转义等价键也400；成立。 |
| AR-03 | path总长512、basename256在写入前判断；非法400，合法长目录不被截短；成立。 |
| AR-04 | 新关联32/64小写hex校验在DAO前；非宽松写入；成立。 |
| AR-05 | 两种版本写共用Folder行锁+最新头锁定读+递增时间；MySQL无分叉证据成立。 |
| AR-06 | Link及后台convert拒绝注册目标/显式来源，普通正例保留；成立。 |
| AR-07 | 当前File浏览关键词、父路径、分页/聚合前裁剪；成立；祖先版本面另有AR-26。 |
| AR-08 | ParseQuery错误/空整数/重复未知拒绝；本轮编码重复参数400；成立。 |
| AR-09 | 四列中的FolderID唯一绑定，注册/rename/上传/版本/归档使用ID；成立；skills去重可破坏旧绑定见AR-27。 |
| AR-10 | UpdateDataset先鉴权再输出策略，事务内再核对；非owner/不存在同响应；成立。 |
| AR-11 | OpenAPI C4检索/证据/复用，C5 MCP工具，与35.6一致；成立。 |
| AR-12 | 孤立旧目录不按当前名称猜归属，creator/tenant保守回退；成立。 |
| AR-13 | 多历史Folder选择当前名否则ID最小，重复迁移保留全部历史地址；成立。 |
| AR-14 | 显式原生Move/Delete与祖先子树拒绝在写前；成立；内部skills重挂漏口为AR-27。 |
| AR-15 | Dataset/Document普通名称查找、版本解析、去重/删除排除受管；已改路径成立；遗漏skills见AR-27。 |
| AR-16 | 无snapshot孤立同名不认领；内部marker保存分类；成立。 |
| AR-17 | 当前/历史File来源与原生授权交集，目标Dataset列表/用途继承，删普通副本保留其他映射；精确路径成立；祖先版本绕过另见AR-26。 |
| AR-18 | 整批File租户/URL子树/操作权限先于对象写入，嵌套普通正例保留；成立。 |
| AR-19 | Created恢复、任务Load/Execute、生产Embedding、WikiDirty与consumer依赖前guard；普通任务继续；成立。 |
| AR-20 | 普通upload不领孤立目录，迁移marker固定owner，普通delete先解析目录；成立。 |
| AR-21 | .knowledgebase例外只在真实租户根直接子层；保留名子目录仍私有；成立。 |
| AR-22 | 显式目录约束沿ParentID至raw子孙，所有者交集和模型/Transfer共用；关系未破坏时成立；26/27不能被此局部正确性覆盖。 |
| AR-23 | 注册域锁内重读当前授权后status0归档，全事实与对象保留；三模式对照成立。 |
| AR-24 | 本地模型可用性说明纠正，原a1报告字节保留，当前交接无远程因果误述；成立。 |
| AR-25 | README当前入口统一35章，A底座已实现/B页面待接，不再旧S0/仅方案；成立。 |

## 本轮方法、工程证据与限制

清单全量核对：round-8-hashes-before.json于22:18:37.816Z、round-8-hashes-after.json于2026-10-23T22:30:56.210Z逐项重算86文件；两次manifest_mismatches=[]，79个Go输入与build-src逐项一致。两次独立从实际manifest构造JSON计算subject，均等于冻结摘要；HEAD全程720b88d。原报告与git show a1a171c原字节SHA相同。追加审查报告及docs/stage1a-review-round-7.md不计产品subject。未把追加式结果当作新产品版本。

完整工程日志复核为review-archive-final.log：19包、3478顶层/5660含子用例pass、10个既有skip、0fail，另native/croptest两个包ok及Go构建。79个同输入支持复用该工程结果；**本轮没有声称重跑完整工程测试**。四模式help日志均有Usage和正确flag，help原生退出1为既有约定；本轮读取旧输出而非重复构建。

本轮新执行命令均经bash build.sh --test，禁止直接go test；同一公开构建副本及本地SQLite/MemoryStorage，不连接运行数据库：

- `bash build.sh --test -count=1 -run TestReviewRound8 -json ./internal/handler ./internal/service/dataset ./internal/permission`：4组Passed，祖先历史安全断言Failed，属AR-26证据。
- `bash build.sh --test -count=1 -run TestReviewRound8SkillsInitializationPreservesManagedIdentity -json ./internal/service/file`：安全断言Failed，属AR-27证据。
- 4组Passed分别为精确16KiB及编码键/参数、多来源owner交集与普通正例、对象写失败保持current并重试、游标重授权与跨修订/失效拒绝。它们区别于AR-7的分类往返/损坏TreeState/归档后删副本/空文件深度反例。
- 强化no-history夹具最初使用自指租户root，命中原生递归自身而中止；保留round-8-probes-fixture-interrupted.log。改成普通workspace Folder后重跑得到最终泄露断言；不将该夹具问题当本轮新增产品缺陷。
- 临时Go测试4份全部已移出build-src为reference.txt；构建副本不残留该临时测试，记录all_temporary_tests_removed=true。

运行只读核对见round-8-verification.json（22:28:25.532Z）：应用running/healthy、二进制readonly，挂载及4个实际/proc/*/exe均为234dc7959f050cbcd733239bc26ab47a41d320cd50494f8159a32d4948e213d1，api/admin/ingestor/syncer各1；5个RAGFlow依赖ID/image/mount均与原参考一致。两个外部AICS容器变化沿用external-environment-observation.json真实时点事实，本轮未改动它们，也不归因变更主体。

旧POC/模型仅比对主Agent已提取的current-safe-fingerprint.json与runtime-before-archive、smoke.after四项安全摘要，全部相等，6docs/42chunks/0active。读取了摘要SQL逻辑，没有import native-api.mjs、直连DB或读取env/Key/access_token值；没有独立网络抓包声明。原真实HTTP28组及33组附加摘要、对应断言脚本、MySQL无分叉/作用域0行、publication秘密扫描12值/5431blob/0命中均核对，不重放任何运行HTTP写入或读取原始HTTP/error/dump。

A用户验收、B契约确认/页面、整阶段1及远端推送各保持实际Pending。未做真实模型、真实资料扩围、百万规模、B页面或破坏性恢复演练，这些是明确范围外/后续动作，不是本轮未检查项。AR-26与AR-27均有条件、预期/实际、代码位置和可运行反例，无待决疑似项；coverage=Complete、findings非空、unreviewed_items=[]。
