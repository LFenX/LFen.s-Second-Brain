# 阶段2 A → B：C2来源导入候选契约

版本：0.2.0，状态：ReviewCandidate，基础契约：C1／C3 1.0.0。阶段1交付8aa324e已获用户明确验收。B已核对657ebd7的候选，反馈见第5节；本轮保持候选原字节，不冻结、不合并main，不表示阶段2整体完成。

## 1. 本轮范围与固定规则

只支持手动提交的UTF-8文本／代码，`source_kind=text_file`、`input_mode=inline_text`。来源登记中列出精确相对路径，最多16个；一次预检及导入只处理一个文件。单文件≤2MiB、无NUL；登记／提交JSON≤16KiB，含正文的预检JSON≤16MiB，字段长度和数量还受请求总容量限制。预检10分钟内有效。使用原生登录和同源`/api/v1`；每个操作都按当前私有域所有者授权，不接受客户端指定owner／tenant／服务器绝对路径。

原文先进入私有预检暂存，预检不创建Document、目录修订或索引；提交后才发布原生Document／File／FileCommit与C1目录。每个`registration_id + relative_path`持久映射一个Document，内容SHA不充当来源ID。登记由`domain_id + registration_key`幂等创建；相同键不同配置返回409。不同登记代表不同来源范围，不跨登记按内容去重。

处理方式固定为`catalogue_only`：结构deferred、全文／向量disabled、模型用途空、外发false、自动处理false；客户端不能开启这些能力。组件侧和语言在每项预检中显式提交，登记默认值只用于B预填，不覆盖逐项分类。标签不授予权限。

排除已知凭据／VCS／依赖／构建路径和两种私钥标记，不声称能够识别全部秘密。本轮仅合成资料；真实资料仍须后续明确允许清单。UTF-8 JSON不合法时拒绝，不静默替换原始非法字节。

## 2. HTTP操作

全部C2操作要求`X-Knowledge-Source-Contract-Version: 0.2.0`；基础头`X-Knowledge-Contract-Version`保持1.0.0。响应保留原基础envelope并增加`source_contract_version`，不修改C1响应或能力列表。完整请求／响应见[OpenAPI](knowledge-sources-v1.openapi.json)。

| 操作 | 路径（前缀/api/v1） | 返回 |
| --- | --- | --- |
| 能力与限制 | GET /knowledge/import-contract | 200，仅声明实际实现的单文件子集 |
| 登记 | POST /datasets/{domain_id}/knowledge/registrations | 新建201，相同配置回放200，配置冲突409 |
| 读取登记 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id} | 200，不返回服务器路径或凭据 |
| 预检 | POST /datasets/{domain_id}/knowledge/registrations/{registration_id}/previews | 201，固定单项、清单SHA、预期修订、有效期；不发布来源 |
| 读取预检 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id}/previews/{preview_id} | 200，不回传暂存正文；含已消费任务ID |
| 确认导入 | POST /datasets/{domain_id}/knowledge/registrations/{registration_id}/imports | 新回执201，原幂等键回放200；必须绑定preview_id、manifest_sha256及idempotency_key |
| 读取回执 | GET /datasets/{domain_id}/knowledge/registrations/{registration_id}/imports/{import_task_id} | 200，持久回执；身份重新授权 |

提交顺序：重新授权 → 核对登记／清单／有效期／策略版本／当前来源修订 → 原生保存及C1发布 → 持久回执。一个预检只消费一次；相同幂等键、同预检和清单回放原回执，换预检或清单使用同键返回409。原回放重新授权，但不重新导入、不因原预检过期而失去回执。幂等记录随回执保留，本轮没有自动删除或回收。

这是同步单项操作，没有后台批量调度。任务完成状态仅`succeeded`／`failed`；单项为`saved`／`unchanged`／`failed`，目录为`ready`／`failed`，其他产物保持禁用。201表示回执创建，不保证导入成功，B必须检查状态。可持久化的发布失败保存失败回执并保留旧来源；相同键回放失败回执，不隐式重试，重新预检后用新键重试。无法持久化回执时返回5xx，不声称生成了任务。

存储写入与数据库不是分布式事务：数据库发布使用事务及保存点，失败不改变原Document指向／当前目录修订；内容寻址的未引用对象保留待后续受控清理，本轮不删除共享版本对象。客户端断开不代表已完成提交被撤销；先按幂等键重放确认结果。

## 3. B逐项核对清单

- [x] 登记键、登记ID、相对路径、Document/source_id、来源修订、节点ID各自语义；禁止在前端再生成来源ID或用SHA合并文件。
- [x] 登记默认分类预填，逐项group_id／component_side／language明确提交；前端、后端、共享、CLI、非代码分别展示。
- [x] 预检只展示固定一项、字节数／摘要／预期修订／到期时间；确认操作与预检分开，不能自动提交。
- [x] 检查HTTP与业务状态：201失败回执、401登录、404隐藏无权／不存在、409配置／幂等／修订／过期／已消费冲突、413容量、415格式、5xx失败；失败不沿用旧正文假装成功。
- [x] 提交后使用回执的source_id／source_revision_id／root_node_id接现有C1原文页，不另建读取接口；`saved`不表示可检索或已向量化。
- [x] 网络超时保持同一个幂等键回放；失败回执明确提示重新预检后新键重试，不创建自动重试风暴。
- [x] 核对单文件同步子集是否足够开发首个表单；批量、列表／分页、异步进度、取消、目录与二进制能力需要下一增量，不从draft字段推断已经支持。

上述勾选对应B的c6321b5核对记录，核对对象为657ebd7的C2 0.2.0，不代表新页面或真实联调完成。A本轮未重跑B的前端测试，未合并B分支；本轮候选与B核对版本的OpenAPI／示例摘要一致。双方冻结仍待后续明确记录。

## 4. 拥有边界与下一步

| 拥有者 | 本轮负责 | 边界 |
| --- | --- | --- |
| 本机A | C2机器契约、Go实体／迁移／路由／服务、原生保存与C1发布适配、授权／禁模型、合成验证和运行加载 | 不修改B的Web／客户端，不接入真实资料 |
| B | 核对本清单，准备登记／预检／确认／回执界面和消费者合成测试，沿用原登录／查询层和C1页面 | 不修改共享实体／DAO／路由，不另建来源或任务后端，不擅自扩大C2能力 |
| 双方 | 以同一候选版本核对字段、错误和显示；语义调整先改唯一契约再同步生产／消费 | 未做真实B页面联调不得标记2B完成或阶段2整体退出 |

C1／C3原OpenAPI、B阶段1源码和历史测试报告继续保留。目录扫描／受控根、PDF／PPT原件、网页／RSS、批量任务、移动／删除／保留规则仍属后续工作；模型、解析、索引与检索不进入本次导入。A只生成并导入本轮合成资料，Git不传登录、模型Key、数据库、原件或运行配置。

## 5. B反馈接收与A处理（v0.30）

B交付位于`codex/stage2b-c2-review`，提交[c6321b5](https://github.com/LFenX/LFen.s-Second-Brain/commit/c6321b56bb026c685e19f3c4de49a190328e9def)，[完整反馈](https://github.com/LFenX/LFen.s-Second-Brain/blob/c6321b56bb026c685e19f3c4de49a190328e9def/docs/stage2b-c2-review.md)。B确认当前单文件子集没有消费者冻结阻断项，正式客户端／Hooks、四步页面、同键恢复和回执指定修订的C1入口尚未实现。本轮只接收意见，不合并该分支，不代替用户验收或双方冻结。

| B建议 | A本轮处理 | 契约边界 |
| --- | --- | --- |
| NB-01：细分冲突原因 | 保持通用`knowledge_conflict`。先核查原请求／已消费任务，再按现有规则重新预检；B不匹配英文message判断具体原因。新增reason/code留待双方共同决策 | 不新增字段或错误码 |
| NB-02：分端点容量 | 下表补充已有范围，能力中的16MiB指含正文预检JSON | OpenAPI／能力返回保持原字节和语义 |
| NB-03：允许格式与路径 | 补充已有实现说明，文件选择提示不能宣称任意UTF-8均可；后端仍是最终判定 | B不复制一套后端白名单；新增格式以后单独讨论 |

| 输入或输出 | 现有限制 |
| --- | --- |
| 登记／确认JSON | ≤16KiB；另外遵守字段长度与数量限制 |
| 含正文预检JSON | ≤16MiB；不是原文件容量 |
| 原文件UTF-8字节 | ≤2MiB，允许空文件，无NUL；不静默转码或截断 |
| 登记允许清单 | 1—16个精确规范相对路径；单次预检／确认一项 |
| C1原文读取 | 单次≤64KiB，起止按UTF-8字节边界；大文件分段，不自动改读当前修订 |

格式唯一实现为[IsKnowledgeTextSource](../internal/utility/knowledge.go)：当前扩展名包括`.txt/.md/.markdown/.ts/.tsx/.js/.jsx/.go/.css/.scss/.json/.yaml/.yml/.html/.htm/.sql/.sh/.c/.cpp/.h/.hpp/.java/.rs/.py`，以实际源码和服务端400／413／415判定为准；CSV、PDF、PPT不属于本版。这里只识别源码／文本，不执行代码或解析文档。

路径采用规范POSIX相对路径，拒绝绝对路径、盘符、反斜线、越级／空段／非规范路径；凭据、VCS、依赖、构建等排除规则见[isKnowledgeImportPath](../internal/service/dataset/knowledge_import.go)。现有私钥标记检查不是完整秘密识别，本轮仍只用合成资料。

## 6. A六项验证与必要修复

最新机器证据见[验证报告的current_followup v0.30](stage2a-c2-verification.json)，性能明细见[小样本基线](stage2a-c2-performance.json)。原v0.29顶层报告和历史决定保留原时点；新工程结果的用户验收仍Pending。

| 顺序 | 本轮结果 |
| --- | --- |
| 1 合成与重复入口 | 8份生成样本、动态ID，37次HTTP；原字节、前后端／共享／CLI／非代码标签、未变与更新／旧版回读通过 |
| 2 并发与幂等 | 真实MySQL：5轮、每场景8路，登记／同键回执／预检消费／竞争更新及原生计数一致；部署API并发另有19次请求 |
| 3 故障与恢复 | 仅测试请求的MinIO代理拒绝原文／快照PUT；MySQL发布和回执持久化失败、同键回放、重新预检重试；提交后客户端deadline；失败不更新旧指向 |
| 4 权限与边界 | 52次HTTP，精确2MiB及非法输入；修复唯一owner成员被停用后返回500的问题，知识域入口按原合同先拒绝无活动成员并返回404；真实DB错误仍500，不改变C2 |
| 5 性能 | 1KiB／64KiB／512KiB／2MiB×并发1／3×四操作，32组各20次；含前置1010次HTTP，完整数据保留，不宣称p95或百万级达标 |
| 6 运行与恢复 | `build.sh`构建及四服务模式help；两次真实MySQL迁移；SQL和原文／历史快照对象恢复到独立库／桶，新进程回读两版及回执；原空间不覆盖、不删除 |

唯一业务改动位于[knowledgeBase](../internal/service/dataset/knowledge.go)，只是活动成员检查与既有404规则的修复。共享OpenAPI／示例、C1／C3、实体／路由、B前端源码保持；五个依赖容器和卷不重建，只替换应用的只读Go二进制。未启动模型／解析任务或多轮审查。

本轮修复后相关三个Go包的`TestKnowledge`定向回归为45个顶层／88个含子用例通过，另有真实MySQL／MinIO并发、故障、迁移和恢复验证；不把旧报告的六包数字或B的前端测试计作本轮重跑。

## 7. 可重复执行入口

公开工具为[knowledge_import_poc.mjs](../tools/scripts/knowledge_import_poc.mjs)，使用项目现有`web`锁文件中的Ajv／ajv-formats，不新增依赖。只允许回环地址和登记／目录／原文接口，禁止解析／模型端点，拒绝重定向；最多1800次请求、单请求30秒，单次执行预算20分钟。正式结果需检查退出码和`result.json.status`，失败不改记为通过。

每台机器自行准备原生登录，将`authorization`字段写入被忽略、限制访问的`.local-data/c2-verify/auth.private.json`；不要复制A的登录或把凭据写入命令行／Git。`LFEN_POC_BASE_URL`默认`http://127.0.0.1:18080`，当前原生最小环境必须先就绪。

```powershell
$env:LFEN_POC_SYNTHETIC_ONLY = '1'
$env:LFEN_POC_AUTH_FILE = (Resolve-Path -LiteralPath '.local-data/c2-verify/auth.private.json').Path
$env:LFEN_POC_OUTPUT = Join-Path (Get-Location) '.local-data/c2-verify/smoke-1'
node tools/scripts/knowledge_import_poc.mjs --mode smoke
```

其余模式为`concurrency`、`boundaries`、`performance`，每次选择新的输出目录，防止覆盖证据。运行入口自动创建独立合成域和登记，不接收真实文件路径，不删除既有域；样本保留供复核，不代表已有自动清理策略。

Go验证在原生库齐备的Linux构建环境通过`build.sh`执行。仅在本机私有JSON中配置`mysql_dsn`、`minio_host`、`minio_user`、`minio_password`、`evidence_dir`，通过`LFEN_C2_TEST_CONFIG`指定文件；不加载模型Key。MySQL账号需能创建临时库，测试忽略DSN原数据库名，生成全新`lfen_c2_test_*`库，不修改应用库；MinIO仅使用测试新建命名空间。不要把集成用例放入默认unit层。

```bash
export LFEN_POC_SYNTHETIC_ONLY=1
export LFEN_C2_TEST_CONFIG=/tool-downloads/c2-test.private.json
bash build.sh --test -run TestKnowledge ./internal/service/dataset ./internal/handler ./internal/dao
bash build.sh --test-integration-go -run 'TestKnowledgeImportMySQL/(Concurrency|Failure|Permission)' ./internal/service/dataset
bash build.sh --test-integration-go -run TestKnowledgeMigrationMySQLRepeatable ./internal/dao
bash build.sh --test-integration-go -run TestKnowledgeImportMySQL/Persistence ./internal/service/dataset
```

恢复验证须先备份上述Persistence生成的**合成数据库**以及`objects.private.json`；对象同时包含以数据集ID定位的原文桶和以受管文件夹ID定位的历史快照桶，保留bucket／key／checksum。SQL恢复到另一个`lfen_c2_test_restore_*`库，把它填入同一私有JSON的`restore_db`，`evidence_dir`保持Persistence输出目录，再单独运行：

```bash
bash build.sh --test-integration-go -run TestKnowledgeImportRestoredMySQL ./internal/service/dataset
```

该用例读取对象备份并写入另一个随机`lfen-c2-restore-*`桶，按原逻辑桶ID保留前缀，由新进程读取恢复库／桶；不删除或覆盖原库／桶。原文／对象／SQL备份和详细日志全部留在忽略的受限目录。当前证明合成恢复流程，不代表真实知识库全量灾备已验收。

## 8. 构建加载、回退与下一步

本机A的构建工具为Go 1.27.3、LLD 22.1.8；继续使用既有`build.sh --go`和原生库，不使用裸`go build`替代。构建后验证api／admin／ingestor／syncer四模式`--help`能启动；上游help退出码1可接受，但崩溃或超时不接受。

当前应用只读挂载二进制SHA-256为`759aaeacac158720ce7c0c1c9c217894367718d1bf617a43d20953d6f1c11577`。容器healthy、实际运行文件摘要一致；C2仍ReviewCandidate，C1／C3仍1.0.0。构建源码镜像已与主仓库Go输入逐文件核对；源码修改不会自动进入运行容器。

本轮私有备份位于`.local-data/stage2a-hardening-20261025/compose.before.private.json`；需要回退时仅恢复原只读二进制挂载，再用当前私有Compose对`ragflow-cpu`执行`up -d --no-deps`。不回滚全库，不删除卷，不重新生成凭据。本轮没有新增产品表或迁移DDL；新增测试库／桶与应用数据隔离。回退说明已核对，但没有额外切回旧版再切回新版制造运行波动。

下一步是基于当前候选及B的非阻断建议明确记录双方冻结决定，再由B落地正式客户端／Hooks、登记／预览／确认／回执四步界面、同键恢复和固定修订C1入口，随后合成真实联调。本轮不执行冻结、页面开发、B分支合入或main合并；真实资料、模型和后续来源类型继续保留原授权边界。
