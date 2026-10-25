# S2-R5 网页／RSS 候选接续

日期：2026-10-26；v0.46；状态 **候选待真人冻结，业务未接入**。当前运行和远端main为`7c564a738597e924e411f3b191aaa02d14ae42d8`；R4工程结果见[统一报告](stage2r4-original-verification.json)，R4人的验收仍Pending。本批不把R4批准移作0.6批准。

## 当前入口

- [C2 0.6具体候选](knowledge-web-import-v0.6.candidate.md)、[34操作／54schema](knowledge-web-import-v0.6.candidate.openapi.json)、[合成向量](knowledge-web-import-v0.6.candidate.examples.json)、[本轮验证](stage2r5-web-contract-verification.json)。均为ReviewCandidate，没有新增路由、表、迁移、客户端或运行部署。
- 可重放文档核验：先准备现有`web/node_modules`，在仓库根执行`node tools/scripts/verify_knowledge_web_contract.cjs`。只读候选／Git和23保护文件，结果写忽略目录`.local-data/stage2r5-20261026`；无业务、站点或模型请求。
- A为唯一源码／Git写入者。“R4 原件导入客户端与界面”已交回7e70c0d，当前只读，完成本轮一次集中字段核对；旧“阶段2知识消费与前端”继续真人只读限制。不得自行重复审查、改服务或争抢分支。

## 已收敛的选择

| 问题 | 本候选结论 |
| --- | --- |
| R5范围 | 本地快照＋明确单次匿名取得共用标准化／保存；不能仅完成本地导入就称R5完成 |
| RSSHub | 接其RSS输出，producer是来源声明；不宣称已部署RSSHub或真实路由已运行 |
| 网络 | 固定起始URL；起始origin包含在最多4个明确origin中；HTTP80／HTTPS443、最多3跳、15秒整体预算、2MiB、拒压缩／304；每跳DNS／公网IP钉选，无站点Cookie或授权头 |
| 取得原键 | 创建Capture不联网；明确acquire才取得；在线每Capture一次，重复返回状态；失败先查原状态，再明确新建，不原键暗中重抓 |
| 观察寿命 | Capture创建30分钟；Capture＋钉选normalizer唯一观察，同截止；预览另10分钟且不越Capture截止，解决“观察10分钟已过但唯一身份不能重建”冲突 |
| 版本钉选 | Capture创建即固定normalizer/converter/sanitizer/resource_policy；恢复不改用最新实现重解释 |
| 丢响应恢复 | Capture创建按原键，观察按Capture发现，预览按其原键，确认按原确认键；恢复槽保存原选择／分类／期望头，不保存原文／整个Feed／令牌 |
| 原子确认 | 1—32项、总派生物≤8MiB；事务外验对象，一次短SQL全原子发布；每原生文件夹版本树只处理一次；不做部分成功 |
| 固定与实时头 | 观察是固定事实；target端点只明确刷新本登记条目的实时头，允许同Capture分次预览，不为刷新头重新抓站点 |
| 身份／版本 | GUID/Atom ID原样不透明，缺ID才弱URL，标题不造来源；作者／类别入修订，取得时间／别的条目不入；canonical不合并；缺项不删 |
| 原件／片段／阅读 | 原Capture是网页HTML／Feed XML；decoded是解码片段，native快照固定它；受限HTML阅读另存SHA，不伪造XML坐标或C1根 |
| 阅读安全 | 白名单HTML＋空sandbox／禁外连CSP；移除meta refresh/base/link和触发资源／导航属性；父页独立引用列表，不依赖子页脚本或postMessage |
| 认证分层 | 源站401/403/404→本系统502并保留有界failure，不触发登录退出；本系统401/404只表示本机认证／权限 |
| 总预算 | 各条／各Capture／选择总额均分别定义；JSON≤32KiB、列表最多20项按预算缩页、UTF-8窗口≤4096字节、整件表示≤2MiB，不渲染半截HTML |
| 旧版与生命周期 | 23保护文件不动；0.6独立DTO，C1／0.4／0.5不加字段；无prune／自动索引／物理GC，移动归档由R6另定 |

## 批次交替

1. **候选阶段（当前）**：A保存并推送`codex/stage2r5-web-feed-candidate`，只问本版具体冻结；消费会话保持只读。官方文档研究不是真实来源导入。
2. **获真人批准后**：A按批准候选制作正式冻结文件，保留候选原字节；先实现私有确定性标准化／网络预算与单路径存储，再完成0.6后端及合成验证。未答复不得将问卷默认项当批准。
3. **清洁固定后端提交后**：A明确交唯一Web写权，消费会话只写0.6 service/hook/page及合成测试。所有原键槽、身份、页预算、错误／读取格式以冻结OpenAPI为准；不改全局request／共享UI／旧DTO。A只读源码，准备隔离运行证据。
4. **清洁回交后**：A收回唯一写权，同版真实MySQL／对象存储＋合成HTTP源和浏览器联调；发现问题只定向修复，验证后按授权推main。不得把另一个会话结束当整个项目停工或完成。

验收逐项见候选第8节。当前只有契约检查，不报HTTP服务安全、解析效果、页面效果或吞吐已通过。实现需包含实体／XML／预算反例、跨登记身份、同时间正文变化、元信息增版、Feed缺项保留、原子SQL失败、丢响应／重启／撤权、旧0.2—0.5回归及零模型证明。5次容量门槛属于未来工程，不把静态向量当性能数据。

## 持续推进与待确认

0.6冻结Pending期间，A可继续合成HTTP源／故障夹具和独立内部准备，不能把新共享DTO或接口接入业务。R4验收问答保持独立，不影响本项设计准备。后续仍按R5→R6生命周期→R7阶段2退出→阶段3结构／代码关系及文档阅读→阶段4可选检索／引用→阶段5 MCP→阶段6人格／记忆推进，持续目标active。

真实AIHOT、真实RSSHub路由、AICS／本机文档、模型推理、后台订阅、存储GC、Windows／共享盘开放均未因本候选放开。阶段2真实样本退出依然需要后续明确范围；不能以合成成功替代整体目标。
