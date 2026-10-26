# S2-R6 来源生命周期接续

2026-10-26；v0.53；**真人已批准19db38d，C2 0.7.0正式冻结，开始实施；尚未迁移或部署。** [冻结回执](knowledge-source-lifecycle-v1.freeze.json)绑定具体回答与原候选字节。R5/main 81f3a8c已验收，决定记录已推main `8cbc36b`；本机仍运行R5后端1ac4e4f＋前端40ed01f。实施分支`codex/stage2r6-lifecycle`，A唯一源码／Git写入。

**当前后端增量**：Go实现、真实MySQL迁移、四mode启动与实际API／S3最小闭环已通过，新后端已加载，Web仍R5。统一记录见[实施报告](stage2r6-lifecycle-verification.json)，正式客户端按[独立写窗说明](stage2r6-lifecycle-client-handoff.md)接续。上段“尚未部署”描述冻结时点；当前不得再启用无世代旧后端写入。35份保护文件不变，完整浏览器／故障／容量仍待验证，R6没有声称整体完成。

## 当前入口

- [具体候选](knowledge-source-lifecycle-v0.7.candidate.md)、[10操作／20schema](knowledge-source-lifecycle-v0.7.candidate.openapi.json)、[合成规则向量](knowledge-source-lifecycle-v0.7.candidate.examples.json)、[文档核验](stage2r6-lifecycle-contract-verification.json)。只读重放：仓库根执行`node tools/scripts/verify_knowledge_source_lifecycle_contract.cjs`，依赖已安装的web/node_modules，输出只进忽略目录。
- 仅文档／参考决策核验：20schema、110结构例、14摘要、33动作、24私有栅栏、7项目规则、9路径、7筛选、15错误映射、10接口＋数量检查、29冻结文件＋数量检查。不是应用测试或性能结果。
- 候选保留C1/C3与C2 0.2—0.6的29保护文件原字节；R5[统一报告](stage2r5-web-verification.json)与[发布回执](stage2r5-main-integration.json)不被本候选重新验收。

## 已选候选语义

1. Document／File／FileCommitItem原生身份；新增来源世代和active／archived，当前头仍既有is_current。
2. 四动作完整实现；目录／Web／无登记C1首版禁移动，.2／.5仅既有精确目标。重分类／移动元信息修订引用原对象，不读写原件正文。
3. 实际头／内容／路径／分类／状态变化全部推进世代，含历史ID复用。双CAS／策略先于no-op，成功原键回放只展示固定旧结果，不再施加动作。
4. 退役位置独立占位，同SHA不并源；归档不删除／撤权，固定历史和保护保持。旧默认当前列表在SQL过滤archive后分页，新0.7提供归档列表。
5. 所有旧生产者加私有准备栅栏；.6从明确创建Preview钉当前世代，可复用仍有效Capture／Observation，不重新Acquire。C1只保证本次调用期间栅栏，不能伪称客户端完整ABA保护。
6. 上线前旧无栅栏的未消费准备拒绝，成功项／回执／manifest保留；停止无栅栏旧写入进程再迁移。该影响随具体候选一并交真人确认，当前未执行。

## 集中协作与写窗

“R4 原件导入客户端与界面”会话在R5清洁回交7ed1789后，按明确只读任务一次回传8项客户端／恢复设计输入；A已处理并记录在验证报告，没有发起R5或R6多轮审查。另一旧会话的真人只读限制保持。

本次具体确认已取得，按批准范围由A先冻结正式三件套并实现Go；不复用上一批Web写权。只有A交付清洁固定提交、文件白名单与验证命令后，消费会话才持唯一Web写权；A对源码／Git只读并准备忽略运行证据。清洁回交后A收回、同版联调与必要修复，通过后按已获授权推main。候选原报告保留准备时点状态，正式实施事实记入[统一报告](stage2r6-lifecycle-verification.json)。

## 确认后的顺序与验收

| 顺序 | A拥有路径／工作 | 消费会话后续工作 | 验收 |
| --- | --- | --- | --- |
| 1 | 既有entity／dao迁移、来源私有状态、三个发布核心、旧准备／任务／清单栅栏 | 只读准备字段接线 | 旧DTO原字节；初始化／MAX／ABA／缺栅栏；.6新预览边界；C1无登记与已登记旁路；旧成功回执保持 |
| 2 | dataset生命周期业务、原生对象元数据记录、handler／router、四动作和当前列表 | 只读核对已冻结消费点 | 同事务、精确占位、no-op双CAS、固定结果、SQL分页、撤权和原件正文IO=0 |
| 3 | 清洁后端同版最小联调后交写权 | 独立0.7 service／Hooks／恢复、详情与四动作／归档列表 | 不改共享UI／全局request；认可默认false、冲突不追头、未知先GET、不换键、主体隔离；定向测试／生产构建 |
| 4 | 清洁回交后A收回；两个API、浏览器、迁移过渡／故障／容量与旧链路回归 | 只读提供具体联调反馈 | 四动作和固定阅读、回执、抢位／ABA／重启／撤权；五次2秒元信息门槛；64MiB元信息动作对象GET／PUT=0；合成模型0；发布回执 |

原件／网页不创建C1根，源恢复不恢复域／权限／索引，目录缺失不自动归档。旧0.4明确历史正文可沿用，但当前组织必须匹配且准备世代未变；不自动修复旧清单。

R6完成后继续R7阶段2退出，再按阶段3—6推进。真实AICS／文档／网站和模型仍需后续明确范围；此候选不开放。持续目标active，不把同伴任务结束或等待工程验收当全项目结束。

具体冻结待回复期间，A已独立[准备R7离线合成语料](stage2r7-synthetic-preparation.json)，30份文件及五版契约定位已核对，没有导入／迁移／模型／网络。协作会话继续一次只读编排输入；不据此接未批准0.7接口，也不将夹具存在当阶段退出。
