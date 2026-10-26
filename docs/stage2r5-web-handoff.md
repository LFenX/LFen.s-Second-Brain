# S2-R5 网页／RSS 候选接续

日期：2026-10-26；v0.48；状态 **C2 0.6.0 已获真人批准并正式冻结，后端实施中**。真人对具体d798893回答“确认冻结并继续实施”，见[冻结记录](knowledge-web-import-v1.freeze.json)。当前运行和远端main仍为`7c564a738597e924e411f3b191aaa02d14ae42d8`；R4工程结果见[统一报告](stage2r4-original-verification.json)，R4人的验收仍Pending，不能与0.6冻结混淆。

## 当前实施增量

- v0.49已完成34操作的Go接线及[后端定向报告](stage2r5-backend-verification.json)：7包89顶层／327含子例，旧0.2—0.4等定向回归45／102，二进制构建与四mode help通过。真实MySQL／S3／API、前端和容量／故障联调仍待执行；运行仍R4，不能把能力端点代码的ready=true当已部署或验收。
- 观察产物采用≤12MiB的内部有界pack，一次对象保存并批量保存条目元信息；每条正文／阅读仍有独立SHA、长度及读取范围，确认前再准备所选原生解码快照。未改公开字段或预算。准备阶段未接入的Markdown／prep身份原型已收敛到正式0.6实现，原文件路径保留，原生通用RSS入口保持。
- 正式[OpenAPI](knowledge-web-import-v1.openapi.json)与[向量](knowledge-web-import-v1.examples.json)保留获批候选全部路径、schema、摘要、限制和规则；候选三件套及先前23份保护文件未改。冻结不表示工程验收。
- `e194924`已保存包内安全阅读组件及[24项单元／实际浏览器验证](stage2r5-reading-preparation-verification.json)，[截图](stage2r5-reading-component.png)明确是未接入应用的组件。它不证明RSS标准化、网络防护、SQL发布或完整R5已通过。
- A在`codex/stage2r5-web-import`持有唯一源码／Git写权，实施正式后端。Web会话只读准备接线清单，取得清洁固定提交和明确交权后才写客户端／页面。候选分支d798893保持，不能继续按未获批准等待。
- 当前后端优先顺序：契约类型／确定性摘要→有界HTML／RSS／Atom标准化及安全网络取得→Capture／观察／精确选择／原子确认／固定读取→定向验证与Web交替→真实合成联调及容量／恢复验证→推main。

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

1. **候选阶段（已完成）**：A保存并推送`codex/stage2r5-web-feed-candidate`的d798893，真人随后明确批准。官方文档研究不是真实来源导入，候选原报告保留交付时状态。
2. **正式后端（当前）**：A按批准候选制作正式冻结文件，保留候选原字节；先实现确定性标准化／网络预算与单路径存储，再完成0.6后端及合成验证。
3. **清洁固定后端提交后**：A明确交唯一Web写权，消费会话只写0.6 service/hook/page及合成测试。所有原键槽、身份、页预算、错误／读取格式以冻结OpenAPI为准；不改全局request／共享UI／旧DTO。A只读源码，准备隔离运行证据。
4. **清洁回交后**：A收回唯一写权，同版真实MySQL／对象存储＋合成HTTP源和浏览器联调；发现问题只定向修复，验证后按授权推main。不得把另一个会话结束当整个项目停工或完成。

验收逐项见候选第8节。当前只有契约检查，不报HTTP服务安全、解析效果、页面效果或吞吐已通过。实现需包含实体／XML／预算反例、跨登记身份、同时间正文变化、元信息增版、Feed缺项保留、原子SQL失败、丢响应／重启／撤权、旧0.2—0.5回归及零模型证明。5次容量门槛属于未来工程，不把静态向量当性能数据。

## 持续推进与待确认

0.6已明确批准，不再等待重复冻结确认。R4验收问答保持独立，R6仅有[设计输入](stage2r6-lifecycle-design-input.md)，未生成获批接口或接入代码。后续仍按R5→R6生命周期→R7阶段2退出→阶段3结构／代码关系及文档阅读→阶段4可选检索／引用→阶段5 MCP→阶段6人格／记忆推进，持续目标active。

真实AIHOT、真实RSSHub路由、AICS／本机文档、模型推理、后台订阅、存储GC、Windows／共享盘开放均未因本候选放开。阶段2真实样本退出依然需要后续明确范围；不能以合成成功替代整体目标。
