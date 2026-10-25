# 阶段2 B：C2 单文件导入 Web 交付

日期：2026-10-25。工作分支：`codex/stage2b-import-ui`，直接从 **123482016576fec368104e531c257b6e0e523dd5** 建立。唯一契约为 **C2 0.2.0 Frozen**，Git对象LF字节SHA256为 `98b2ad05730cd392b0ce88cb591ba5191cbf071b22df8b39bbdc33bd76c45091`；C1／C3继续1.0.0。[冻结决定](knowledge-sources-v1.freeze.json)及[交接第5／9节](stage2a-c2-handoff.md)保持不变。

**接续更新：本文件第1—5节和原机器报告是B独立交付时的历史范围；随后A/B已在`codex/stage2ab-integration`完成合成真实后端联调。当前分支、修复及未验项见第6节和[集成交接](stage2ab-integration-handoff.md)。**

**正式客户端、Hooks、四步界面、结果恢复与固定修订阅读入口已实现；本轮工程与Mock验证完成，真实后端联调尚未执行，用户验收Pending。** 不将本次交付记作阶段2整体完成。没有修改Go／DAO／后端路由、共享OpenAPI、依赖锁文件或运行凭据，没有合并main。

## 1. 页面入口与操作

从已有私有知识域的知识目录进入“导入合成文本”，路由为 `/dataset/knowledge-import/{domain_id}`。页面可切换已有知识域；域和分组均复用C1读取。只有原生授权的私有域及Frozen能力通过后才显示表单。

1. **来源登记**：输入登记键、名称、1—16条精确相对路径和默认分类，保存登记。相同键同配置可回放；配置409由用户处理，不自动换键。登记响应丢失时只提供原配置核查，登记成功不表示原文已保存。
2. **私有预览**：从登记范围选一个路径，读取一份合成UTF-8文件，或载入页面提供的合成例子。逐项分组、前端／后端／共享／CLI／非代码标签、语言均明确提交。保留BOM、CRLF、emoji和空文件；非法UTF-8、NUL及超容量拒绝，不转码或截断。格式／路径仍由服务端最终判断，前端不复制格式白名单。
3. **明确确认**：展示原文字节数、SHA、操作、分类、基线修订和到期时间；用户单独勾选确认再提交。修改文件、路径、正文或分类会废弃预览及确认。确认前固定三字段请求，并先保存恢复标识；提交中防重复操作。
4. **回执**：分别展示HTTP结果、业务成功／失败、原文保存、目录、结构、全文／向量索引及模型调用次数。201可以是失败回执；失败没有成功阅读入口。unchanged说明沿用修订。结构deferred、全文和向量disabled、模型0保持独立显示。
5. **阅读**：成功回执进入既有C1阅读器；URL携带服务器返回的来源、固定修订、根节点和摘要。读取仍只用C1服务与1.0.0头，校对身份／摘要并使用既有≤64KiB字节窗口。目录刷新保持URL中的修订；返回当前目录是单独的用户操作。

容量分别为登记／确认JSON16KiB、预览JSON16MiB、原文UTF-8字节2MiB。前端直接消费冻结schema进行请求／响应校验，包括时间格式；不接受异步running、索引成功、非零模型次数或失败回执中的发布ID。C2所有请求同时发送基础1.0.0和来源0.2.0版本头；ReviewCandidate或其他版本停止功能，不回退draft。

## 2. 原请求恢复与重试

| 情况 | 页面处理 |
| --- | --- |
| 超时、断开、5xx或不可靠的确认响应 | 显示“结果待确认”，保留原preview_id／manifest_sha256／idempotency_key；禁止直接开始另一轮，不自动换键或循环重试 |
| 用户点击核查 | 重新授权读取登记和固定预览；有consumed_task_id则GET原回执，无任务则重放同一确认请求。回放成功仍检查业务状态 |
| 通用knowledge_conflict | 不匹配英文message。先核查原请求／已消费任务；原请求409后再次确认预览尚未消费，才提供用户主动“重新选择文件并预览”。仍无法确定结果时继续保留原请求 |
| 已知failed回执 | 用户主动重试，重新选择文件、创建新预览，使用新键并再次明确确认；旧键只查询旧结果 |
| 401／404或版本失败 | 隐藏私有名称、正文和相关查询／变更缓存；重新检查权限及版本后才恢复。不会展示旧缓存冒充本次结果 |

恢复记录存于浏览器sessionStorage，只含登记ID与原确认三字段。按现有原生用户和知识域隔离，同一账号重新登录后可继续核查；直接令牌会话缺少用户信息时仅在该会话范围恢复，不自动合并到另一账号。记录不保存原文、登录令牌、模型Key或运行配置，本地范围键不作为后端身份。存储失败时不发送首次确认POST。没有新增来源ID、映射表、任务后端或读取接口。

## 3. 实现位置

| 位置 | 作用 |
| --- | --- |
| `web/src/services/knowledge-sources-service.ts` | 7个冻结C2操作，沿用registerNextServer／next-request；校验版本、schema、HTTP状态和响应作用域 |
| `web/src/interfaces/{request,database}/knowledge-sources.ts` | 请求／响应DTO；成功与失败回执用可区分联合类型 |
| `web/src/hooks/use-knowledge-sources.ts` | 当前会话查询键、无自动重试的能力查询／动作；不把原始令牌放进queryKey |
| `web/src/pages/dataset/knowledge-import/` | 四步向导、分组与分类、结果恢复；动作集中在use-import-flow中 |
| `web/src/utils/knowledge-import.ts` | 严格UTF-8读取、字节／摘要、预览与回执关联校验、最小恢复记录 |
| `web/src/utils/knowledge-source-link.ts` | 回执到C1固定修订的URL与参数校验，不推导身份 |
| `knowledge-catalogue/index.tsx`和C1 Hooks | 新增按固定修订读取和导入入口，保留原目录浏览；C1 HTTP服务不变 |

复用的目录重试／分页按钮明确设为`type=button`，用于导入表单时不会意外触发提交。共享`components/ui/`、产品品牌配置和Logo未修改。

## 4. 实际验证范围

**5套68项通过：正式C2服务20项、UTF-8／关联／恢复工具6项、导入页面13项、原C1服务12项和阅读器17项。** 公共可复现命令（先按web锁文件安装依赖）：

```powershell
cd web
npx jest --config jest.config.ts --runInBand --no-coverage --runTestsByPath src/services/knowledge-sources-service.test.ts src/utils/knowledge-import.test.ts src/pages/dataset/knowledge-import/__tests__/import.test.tsx src/services/knowledge-foundation-service.test.ts src/pages/dataset/knowledge-catalogue/__tests__/catalogue.test.tsx
npm run build
```

覆盖Frozen／版本拒绝、7操作与双版本头、5类标签逐项传递、201失败、失败新预览／新键／新确认、超时同键、页面重挂与同账号续登恢复、已消费回执GET、过期／竞争更新通用409、401／404隐藏、输入与容量／格式、空文件、BOM／CRLF／emoji、固定旧修订与目录刷新。Mock transport全部在测试或临时浏览器入口，不是产品后端或生产开关。

- 完整默认production构建通过，保留原项目的大chunk提示；没有部署或替换A运行产物。
- 定向oxfmt和项目配置oxlint通过。冻结基线与当前完整tsc均为207项，归一后新增／移除错误均0；**全量类型检查仍未通过**，不把增量无回归写作全仓绿色。
- 实际测试／构建使用公开源码镜像和现有精确锁文件依赖缓存；相关源文件逐字节核对，不复制`.env.local`或私有运行文件。首次完整构建因镜像遗漏公共API参考Markdown而失败，补齐该公开文件后通过；没有为此修改应用源码。
- 初次类型检查的4处新增类型问题、Jest转换器对导入类型／泛型箭头的解析问题已修正。浏览器发现HTML pattern的`v`标志兼容问题，已按相同允许字符集修复并加入验证；没有修改冻结契约。

浏览器使用实际页面组件和本地合成Mock：完整四步、固定修订阅读、201失败后再次明确确认、模拟丢失已发布响应后GET回执均通过。BOM样本为51字节，摘要`cec498db42f76b9035460537194b81fa4e096f3dc7242d9d5c4aabc7f47d9b48`与本地原字节一致；390px视口scrollWidth=390。模拟超时恢复只有1次导入POST。它们不证明真实MySQL／MinIO、网络故障、原生登录或完整应用外壳联调。

截图均显式标记MOCK：[窄屏预览](stage2b-import-ui-preview-mobile.png)、[成功回执](stage2b-import-ui-receipt.png)、[201失败回执](stage2b-import-ui-failed.png)。[机器记录](stage2b-import-ui-verification.json)保存原需求、逐项覆盖、摘要和证据边界。

## 5. 真实联调准备与缺口

**B真实后端联调：NotRun。** 本机未发现`lfen-ragflow-poc`项目运行容器；本轮未加载同版后端或创建／使用本机原生登录。没有使用A回环地址或交换A凭据。A已有后端验证继续归属于A，不计为B真实请求。

接收分支后：

1. 同一运行环境加载冻结Go版本及本分支Web产物，或在B准备同版本后端与自己的原生登录；沿用既有Compose／卷，不复制A私有配置。后端能力必须是Frozen 0.2.0。
2. 使用正确同源入口。现有Vite开发代理仍指向本机Go API9380／Admin9381；若本机运行映射为18080，应先对齐运行地址与代理，不能把A的127.0.0.1当作远程地址。B没有修改代理或新增后端变体。
3. 使用独立合成私有域，按第1节跑完整流程；读取回执固定修订，另发布一版后刷新目录确认旧版不变。确认保存／目录／禁索引及模型0。
4. 由运行环境提供可控的失败、丢失响应、过期／竞争修订和权限撤销场景，验证真实201失败、同键核查与重新预览；保留脱敏证据。不要在真实资料或共享服务上做破坏性注入。
5. 真实联调及用户验收通过后再安排main合并。当前没有目录扫描、PDF／PPT、网站／RSS、批量异步、取消、移动删除、解析索引或模型能力。

回退仅需撤销本次Web增量或恢复前版Web产物，不改后端、凭据或卷。回退页面不会撤销已经确认的服务器导入；未知结果继续用原请求核查。本轮仅推B工作分支，最终提交号以该分支头和交付消息为准。

## 6. v0.33后续真实联调

本机A已将B `8d62cc7`与A `5fe4d51`汇合至`codex/stage2ab-integration`，加载本分支Web并连接原Frozen Go／MySQL／MinIO。原生登录、四步、固定旧修订、真实201失败重试、丢失响应重载／GET恢复、过期与竞争409、404隐藏及身份隔离通过；原Mock结果继续保持独立证据，不改写为B另一机器实测。

真实联调发现确认字段被通用snakeCase改为`manifest_sha_256`。C2客户端现于schema验证后显式JSON序列化并设置Content-Type；Go、全局请求层和冻结契约未改。新增真实interceptor／Axios序列化回归，合计6套69项通过，21文件格式／lint及生产构建通过；本机全量类型检查仍207项，本轮改动4文件0项。

完整情况以[真实报告](stage2ab-runtime-verification.json)与[集成交接](stage2ab-integration-handoff.md)为准。B从集成头接续并保留该修复，不再从旧头重复实现；A的新来源仍未接入。继续只合成／模型0／不审查／不合main，本轮用户验收和阶段2整体退出保持待定。
