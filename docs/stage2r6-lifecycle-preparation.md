# S2-R6 生命周期前置能力盘点

日期：2026-10-26；生产源码基线main `38ce003`，当前分支只增加R4候选／准备文档。**本批仅本地只读事实与语义建议，没有共享接口、Go实体、迁移或生命周期实现。** 消费会话集中回传，A核实关键拥有路径；无业务／网络／模型请求，没有真实移动、归档恢复、删除或物理GC。

## 1. 已存在的能力及边界

| 范围 | 核实事实 | 不能据此宣称的能力 |
| --- | --- | --- |
| 域归档 | [crud.go](../internal/service/dataset/crud.go)的`deleteDatasetAuthorized`锁域并重新授权；知识域只设KB状态无效，保留文件／来源／快照／对象，普通库才走清理 | 这是整域隐藏；没有因此获得来源级回收站、域恢复或归档后C1仍可读能力 |
| 分组 | [knowledge.go](../internal/service/dataset/knowledge.go)已有project/topic/module创建／列表、同域父节点与深度约束；分组不是File.ParentID或权限 | 不等于已经支持分组移动／归档／恢复，标签和分组不授予访问 |
| 来源绑定 | [knowledge_import.go](../internal/entity/knowledge_import.go)唯一绑定登记＋路径，Document另唯一，source_id即DocumentID | 不能按SHA猜移动、不能给同一Document简单插第二条普通绑定作旧位置别名；不能静默扩大登记范围 |
| 固定版本 | [knowledge_source.go](../internal/service/dataset/knowledge_source.go)以原始SHA＋路径＋分类生成输入摘要；域锁内核对预期头并保留旧修订；相同历史输入会复用原revision ID | revision ID不是单调操作序号；如果动作要求识别“离开再回来”，只比较revision ID不足，需要单独生命周期版本 |
| 原生快照 | [source_snapshot.go](../internal/service/file/source_snapshot.go)已有文件夹行锁、FileCommit／Item、前驱和对象地址 | 当前仍接`[]byte`；R4所需“引用已验证对象的统一元数据核心”在本基线未实现，不提前假设存在 |
| 固定阅读 | C1按固定revision并JOIN有效Document读取；原生提交读取固定对象，C1校验SHA／大小 | 直接删Document／设其status=0会阻断旧C1阅读，不能充当“归档后保留历史”；清空全部is_current也不是归档协议 |
| 原生文件操作 | [file_folder.go](../internal/service/file/file_folder.go)跨folder移动受管树受保护；普通移动会copy/delete对象和修改parent/location；就地rename可改File／Document显示名 | rename不更新知识来源定位／固定修订，不等于领域移动；普通物理move没有源／目标CAS或固定业务回执 |
| 删除与来源保护 | [file_delete.go](../internal/service/file/file_delete.go)先逐项鉴权并拒绝受管树；[knowledge_folder.go](../internal/dao/knowledge_folder.go)保留当前／历史来源与祖先后代约束，归档域不从来源事实中剔除 | 不能通过清映射／knowledge_enabled／历史记录让已归档知识变成普通可删文件；普通删除不是逻辑归档 |
| 项目版本 | [knowledge_project_manifest.go](../internal/service/dataset/knowledge_project_manifest.go)新发布校验固定修订／路径／分类／root、当前绑定和分组；已发布GET读取存档Payload，不按新绑定重算条目 | 移动不自动重写旧清单；旧基版沿用项改绑后可能不能再通过新发布校验，不能声称“项目版本自动同步移动” |
| 现有清理 | [knowledge_directory.go](../internal/service/dataset/knowledge_directory.go)到期清临时正文／观察，24小时前未消费元信息可清，已消费关联保留；[source_plan](../internal/service/dataset/knowledge_source_plan.go)只产出missing_proposal／unobserved | 临时TTL不等于历史／原件／回执GC；不能说系统完全没有清理，也不能把扫描缺失当删除或归档许可 |

## 2. 应在后续候选中明确的最小语义

1. 单个已知source_id、同域、当前主体／成员／策略授权；不跨域转移、不递归移动／归档整棵树、不搬真实磁盘／MinIO原件。域恢复、跨登记与分组reparent另列范围。
2. 组织归类变化和生产者定位变化是不同动作。改group／标签可保留原采集定位并产生新元信息修订；改登记内路径须核对目标配置／范围／占用和旧定位后续规则。建议先限制同登记已允许目标，不把展示名当来源路径。
3. 来源ID／FileID／原件SHA稳定；旧revision、节点、回执和清单不原地修改。新事实单独发布。目标active或archived占位均冲突，同字节也不能覆盖、合并或自动加后缀。
4. 新动作同时核对预期来源修订与独立单调生命周期版本，约束位置／active-archived状态世代；明确no-op。两源抢目标、A→B→A、归档→恢复及旧任务迟到都需合成反例。数据库唯一性与Unicode／大小写比较规则不能只由浏览器猜测。
5. 来源归档建议另存active→archived状态，保留绑定、当前头、Document／File、所有历史及保护事实，默认列表在分页前过滤，目标位置保留。合法域授权下固定历史继续读；撤权仍拒绝。若要求归档即禁止历史，应另确认，不能混同撤权。
6. 恢复只改archived→active并递增生命周期版本，核对目标、登记、组和策略；不覆盖冲突、不恢复域／root权限／订阅／ingestion。恢复不是内容回滚，当前修订不自动换历史版。
7. 原键按主体／域／动作隔离；同元组回放当时固定结果，异元组冲突。超时先查原结果，GET不写；回放旧移动／归档结果不能再次施加动作，当前生命周期另查。旧C2确认路径也须在域锁内拒绝过时的归档／移动写，使用其既有冲突语义；旧成功结果保留。
8. 旧项目清单／回执原字节及head不随生命周期动作重写；新项目版仍独立预览／确认／CAS。归档源能否进入新发布的历史基版沿用需明确选择，不能假定旧0.4理解新状态或改冻结DTO。
9. 首版不物理GC。保留闭包包括全部修订／节点、原生Commit／Item／TreeState、绑定／退役定位、原请求／原键／回执、项目head及非head清单、活跃／未知任务、租约，以及后续原件／网页／阅读表示和备份。对象引用按后端＋bucket＋key识别，不只看当前head或SHA。

上述均为建议，未冻结、未接入。当前0.5原件候选也仍待确认，不把持续目标当所有生命周期取舍已批准。

## 3. 后续工程验收输入

- 移动后来源身份与字节保持，新位置／分类为新事实；旧C1／原件／清单／回执保持原固定指向。
- 目标active／archived占用、no-op、跨域／越界、无效组／登记、大小写／Unicode冲突：拒绝且不改任何头／绑定。
- 两源抢目标，移动与旧导入／任务／原件确认竞争，源头与生命周期世代分别变化，A→B→A及归档恢复：仅完整预期者提交，无半迁移和死锁。
- 旧路径再观察、旧预览确认、崩溃恢复和root撤权不静默复活或复制来源；缺失仍是proposal，不修改外部来源。
- 归档默认隐藏但不删除历史／原件／保护；合法域授权下历史按最终选择读取，跨域／跨用户／撤权所有入口拒绝且分页不泄漏。
- 恢复不回滚内容或重授权限；源／目标／组失效不自动找替代名。域归档与来源归档明确区分。
- 丢响应、双进程同键／异元组、重复动作和未知结果：原键查／回放不重复操作，旧结果与当前状态分离。
- SQL／对象故障不产生可见半迁移；项目CAS独立；原生move/delete/link、历史及祖先维护仍保护受管来源，普通来源不被扩大拦截。
- 临时清理与持久保留分开；当前不执行物理GC。尚未落地的原件／网页相应用例标NotRun，不以本次读代码当通过。

本项只完成前置调查，不替代R4实施，也不宣称阶段2或全项目已完成。继续合成、模型0、无多轮审查。
