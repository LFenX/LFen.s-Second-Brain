# S2-R3 批次候选消费核对与回交

日期：2026-10-25；分支：`codex/stage2r3-batch-candidate`。
固定候选：`d9d2ce1968485e002a42fb80867a44eecf244f94`；接手时工作树干净。
范围：候选17操作、35 schema、全部给定结构/语义/摘要/原字节例，以及取消、租约、分页、项目清单和恢复的静态消费可用性。本轮仅合成资料、业务API调用0、模型调用0。

## 实际校验

独立脚本使用 Node v22.22.3、已有 web/node_modules AJV 2020/ajv-formats，读取固定 Git 对象，不导入生产校验器或修改候选。

- 全部35 schema编译通过，另补35个逐schema正例；17操作的版本头、路径参数、请求/响应引用及no-store结构核对通过。
- 给定41结构正反例、25跨字段语义例全部符合预期。
- 2摘要向量均从input逐字段重建投影，再核tokens、LP原字节hex、SHA及input摘要，全部通过；不是仅对tokens自验。
- 2原字节向量通过：55字节BOM/CRLF/中文/emoji及空文本，往返字节及预览SHA/大小一致。
- 8附加断言通过：UTF-8排序区别于UTF-16、512字节路径边界、分类null/空语言、字节数/预期头绑定及消费状态不入原摘要。
- 相对main `6577af32a1755e3a7889bbf1b7b387544a920ac5`，旧11份受保护文件哈希一致；2项前置协议声明哈希一致。

静态给定预期通过不代表候选一致或实现完成；`consumer_ready_for_freeze=false`。以下4项交A集中修订。

## 集中问题

### R3-CONS-01 · P2 · 项目例遗漏关联任务的成功成员

正文第84行要求固定基版加全部成功任务条目。examples第2441行批次向量固定`web/卡片.tsx`和`web/空.ts`两项；第2533行项目向量引用该两项摘要，却在`expected_manifest_id=null`时仅有`added=1/total_entries=1`，遗漏空文件。空文本也是合格项，不能静默丢失。两个向量自身SHA正确，缺口是跨对象成员关系。
证据：JSON的`projectionLinks`和`manifest-vector-omits-successful-selection`。建议补齐完整成功任务、两项清单及相关回执并重建向量，或提供真正独立且完整的一项批次；增加成员遗漏的语义反例。A只读补充亦确认此项。

### R3-CONS-02 · P2 · HTTP/code/reason映射不一致

OpenAPI第4304行声明`knowledge_batch_version_mismatch`，第4774行起409映射却只列`knowledge_batch_conflict`及非枚举文字`version_mismatch_or_named_conflict`。此外Error schema接受HTTP404搭配conflict/source_changed、expired/result_unknown；给定25语义例未覆盖错误三元组。
证据：`machineMapping`及`error-404-conflict-reason`、`error-expired-result-unknown`、`error-version-mismatch`。建议明确各HTTP状态允许的code/reason组合，分开版本错误与业务冲突，补正反例；客户端不能靠message分流。

### R3-CONS-03 · P2 · 重试父任务有效性须明确

A只读补充，已对照正文第65行及AcceptRequest：当前仅称retry_of_task_id是“受校验的来历”。应写死父任务同主体/域/登记、允许的父终态，以及新选择为未完成子集或原完整选择的规则；同键回放绑定同一来历。旧任务不变，失败重试经新观察/预览/逐项认可生成关联新任务。此为候选定义缺口，不是已执行的越权测试结论。

### R3-CONS-04 · P2 · 清单发布恢复元数据须单列

A只读补充，已对照正文第73、118行及manifest_recovery例：第5节只列批次恢复，第7节要求核对原项目及expected_manifest_id。应明确首次发布前允许持久化domain/协议、project_id、preview_id、摘要、原键、expected_manifest_id和task_id；首次null基版必须显式保留，不能用缺字段或后来读取的新Head代替。404/503保留原上下文，不自动换键/POST。

## 实施边界与未验项

候选已明确全批固定预览后逐项主动认可、一次202受理、持久任务与终态、DB租约epoch/锁序、取消保留已成功项、结果revision分页、关联新任务重试及独立项目CAS发布。修订上述项后，可据此设计独立0.4客户端/状态流，复用分类、原文窗口与C1固定链接；不能循环旧0.3单文件POST冒充任务。

非阻断注意：ContentWindow字节/区间/next_offset、任务计数与分页关联需独立跨字段校验，AJV结构通过不足；任务列表依赖已知registration_id，页面须用稳定URL及允许的持久标识支持关页恢复，不把仅React状态/sessionStorage当持久任务发现。

worker/SQL事务、双进程旧epoch拒绝、重启、取消竞态、实际权限与资源性能均待实施验收；本轮未证明它们。未重跑旧238项，未构建、修改生产Go/Web/冻结文件/依赖/运行配置，未操作服务或进行多轮审查。

## 静态证据与交付状态

原始脚本和日志留在忽略目录`.local-data/stage2r3-20261025/consumer/`：`check-contract.cjs`、`validation-result.json`、`validation.log`、`validation-initial.log`。运行命令：`node .local-data/stage2r3-20261025/consumer/check-contract.cjs`。

候选仍ReviewCandidate，冻结Pending、实现未开始、用户决定Pending。本轮仅提交本交接文档；A收回写权后集中处理4项，消费者恢复只读，不自行修候选或增加核对轮次。
