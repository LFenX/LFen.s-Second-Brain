# 第33基线候选：独立集成测试启动初始化（2026-11-16）

拟议基线 `KCRL-20261116-33`。Root继续按最新人类要求单人实施、审阅与执行，其他会话保持暂停。本次唯一Go源码差异是共享授权MySQL集成测试在分词器后初始化查询构造器；产品代码、权限断言、模型、真实资料与根布局均不变。

开发里程碑4118675已正常提交并推送远端，145件、11项检查通过；旧GitPending保留其原时点。现有已登录内置浏览器的共享页面实测显示正确空状态。真实Native验收已到base/prepare，但barrier出现空指针；新初始化候选的MySQL/ES两ELF已离线编译、LLD20及4个目标枚举通过，实际修复后重跑仍Pending。Go21已推进到原始Go程序；最近一次启动状态停滞后Root按精确进程身份取消并闭合外层IO，原Unknown不提升。

当前只追加既有current_context内的standalone_integration_bootstrap_followup并保存Ref32原始manifest；97入口/102用途、47保护/26合同/11业务archive/28历史及所有选择器用途不改。旧执行记录字节与下方authority全文原样保留。需先用未修改校验器验证五后像RAM叠加，Root冻结精确候选后机械应用，再实测新锁。S1整体验收、S2/S3门及阶段提交要求保持；本候选不声称阶段完成、当前ELF已部署或最终正式可用。

---

# 第32基线候选：单会话执行与测试格式收尾（2026-11-16）

拟议基线 `KCRL-20261116-32`。最新人类明确要求暂停其他会话，后续由 Root 一人完成；不再派发 A/B/C/D。当前窗口是 Root 对两个 Go 测试文件做标准 gofmt 格式修正并记录实际验证进展，不改变业务接口、许可、模型、真实资料或根布局范围。

已实测：10 个 Web 文件175/175；67个 Go 定向单元及原生尾测通过；服务端与CLI构建、LLD20和五种help通过。仅随后两个测试文件的格式候选尚未落地；规范词法 token 完全一致，产品源码与既有构建产物不变。S1真实SQL/ES/HTTP/浏览器验收仍Pending，S2/S3门保持。登录已由用户确认；原管理容器自动重启改为unless-stopped并核实运行。

Git仍Pending：首个Windows钩子因多行shell执行问题误改两件冻结内容，Root精确恢复后Ref31通过；全十一项只读重跑只剩两件gofmt失败。本候选保留冻结schema及旧authority全部字节，不通过格式化历史或改写旧失败求通过。后续须所有检查通过才正常commit/push，不把开发提交当成正式验收。

只在既有current_context选择器追加 `sole_root_validation_and_format_followup`；旧执行记录全文以一次插入保留，下面Ref31全文为原字节后缀。Ref31 raw基线原样存入既有archive目录，旧previous链不丢失；97入口/102用途、47保护/26合同/11archive/28历史及所有选择器、角色与用途不变。候选须先经过未修改校验器的RAM叠加审阅，Root冻结精确六后像并机械应用后，才实测新锁。

---

# 第31基线候选：Web测试夹具与验证边界（2026-11-16）

拟议基线 `KCRL-20261116-31`，总控仍为 Codex 会话 `01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。本轮仅在既有 `coordination-20261106-round3/current_context` 选择器内追加独立 `test_harness_validation_followup`；Ref30 及更早全文保持原字节后缀、原事实时点。准备阶段 Main 仍为 Ref30，本段与新锁均为候选；独审、冻结和 Root 另授 exact6 窗后，须实测实际树及对应新锁，不能刷新旧锁强行通过。

唯一预测源码差异是 `web/src/services/__tests__/knowledge-access-service.test.ts` 与 `web/src/pages/dataset/__tests__/narrow-navigation.test.tsx` 两件测试夹具。Source 仍为 5141 件，按未修改算法的 RAM overlay 预测 SHA-256 为 `06859d88b23cfff5fe74efe7d80e8253aba14508912c214d2f16de0c8c834503`。其余四件是 execution record、本文、reference lock 与 Ref30 原字节快照；引用仍为97入口/102用途，全部 IDs、selectors、roles、用途及47保护/26合同/11archive/28history不改。

本轮 Web 实测为169/175、六败；production build exit0，公开 index descriptor 为3552B、SHA前缀 `665ee`（本次派发未给全摘要，不补造）。Root b7c090 的 corrected Ref30 check 为 Passed_ReferenceLockOnly。两件最小候选使用局部真实 Node WebCrypto/精确恢复，并保留七导航及 owner/文件跳转断言；两件实际 format/lint 通过，后续 Jest 为 NotRun，原六败和首次 NUL 解析/01格式失败均保留，不改称通过。

真实登录用户的“已进入系统”由 Root 首次记录于2026-11-15T17:32:35Z（JST日历日期为2026-11-16）；时间来自Root clock回执，只表示Root首次记录时点，不是用户点击发生时间。A04 preflight exit124、unit执行0；Go21 actual24 首次 fresh_native_select 在work8000ms/host8500ms到期且物理关闭，形式结果仍 Unknown。新记录不补绿，也不扩展这些证据的原用途。

KCA02 具体人类 subject、三个阶段范围、合成材料与无新模型边界不变。S1正式验收仍Pending，S2/S3门保留；中间开发commit仍须当前定向unit/build/diff合格并由Root另授。没有新增接口、模型、生产、凭据或根布局授权。Ref30 raw176623B/`560ec710b6b33c65bee3a0d659b407805013923a27d9d9f92f8114da4b9d2c5a` 仅逐字节保存在既有归档目录的 `docs/archive/knowledge-core-20261028/reference-baseline-KCRL-20261115-30.json`，供previous链追溯，不把旧正文升级为当前指导。

---

# 当前任务参考资料基线

**2026-11-15 第30基线候选：exact138源码集成开发里程碑，S1真实验收仍Pending**

拟议基线ID：`KCRL-20261115-30`；总控仍为Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。事实入口仍为`coordination-20261106-round3/current_context`，只在原选择器内追加`source_integration_development_milestone_followup`。旧35根键、30子段及其原值/原始片段保留；以下Ref29及更早全文保留原字节、原时点。

本次是Root在既有access02具体人类实施决定内的调度改排，不是新的人类批准：先准备并独审Ref30/exact138，冻结并另授具名有限单次写窗后实际集成，再取得该集成版定向单元/构建与差异/秘密检查证据，由另派Git执行者处理有证据的中间开发commit/push。该里程碑不得称S1完成；S1真实合成验收仍Pending，S1→S2→S3门及阶段完成且验收后的checkpoint保留。catalog/preflight旧先验收再落地安排不回填。

唯一源码集合为已冻结delivery-catalog.safe.json的138件、2493188字节，99新/39改：67后端、35Web、9有效Go测试、21 Go21测试、6合成夹具；集合SHA `ded7ef763a3dc3637b14cbb5a06a6c0e05a229dcf3701f08ca39afc7a021ccbb`。主树/HEAD前像集合SHA `8397b020640fed785afc586b448f33ef3bcd864cdf5009b60450b54e6bf17563`。只涉及internal/web既有布局，不新增共享接口或扩大具体决定。D预检与B gate仅按原时点、原用途引用；B gate是Root调度参考，不是人类批准。

准备时实际主树仍Ref29通过，HEAD `f804b3737a8f69af14b1ff1c2d042e8ea7c2a9f1`，源码5048/`2c786735cda277f5d02238a2c6086f4c1972c5ce12369ed51545c5afe2efa152`；138实际落地0，本任务新commit/push0。5141/`a50ffc5160792bc0eebc5293c3294faf61cd96a9905cfddffdcdbee9a99ba94d`只是在内存覆盖exact138所得预测，不是主树实测或正式新锁通过。未修改校验器的叠加结果只能称CandidateOnly；实际应用后须重新测量全树并执行exact新锁校验；不同即停止受影响工作并重审，不刷新旧锁求通过。

既有源码接受及分层单元/构建仍限原版本/范围，不能替代新集成版验证。Native2 actual02仍Unknown（PS exit2/callerClosed不证明深层）；Go21 actual20失败后关闭、业务NotRun；WB03真人到登录页，但本工具browsers=[]，真实浏览器验收NotRun。S2/S3后续，合成材料范围不变，不续授模型、真实AICS、生产发布、凭据、根布局或模型资源变更。

最小候选只涉及execution-record、authority、manifest及Ref29原始manifest基线快照四路径；仅两项既有内容摘要更新，97条/102用途、26正式契约、47保护件、11业务归档、28历史原路径及全部选择器/用途不变。旧Ref29原176178字节/SHA `ab12875dfc1e0385eb1df1c6b474b4439d8d0e811f57ccc1259497055ff1001e`只进入既有baseline链，不增加归档指导用途。

本候选不授予正式写入。须C独审、Root冻结exact后像并另授有限单次机械权限；本准备仅写指定ignored候选目录，未写主树/源码/Git，未构建、调用服务或读取私有运行资料。新锁raw SHA在独立交接给出，不嵌入被锁内容。

---
# 当前任务参考资料基线

**2026-11-08 第29基线候选：阶段2完成与已授权品牌差异闭环**

当前基线标识：`KCRL-20261108-29`；总控仍为 Codex 会话 `01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。事实入口仍为 `coordination-20261106-round3/current_context`，只在原 `/coordination_20261106_round3` 内追加 `branding_and_stage02_completion_followup`。35 个根键、此前全部29个子段（包含原28子段和Ref28追加段）的原值与原始片段保留；下面第28及更早导语均保持完整原字节和原时点。

阶段2已经正常 commit 并 push：`bae0c516b1966b904657cb69f0a23121f3bf0991`，parent `78f0337dcda1cd2240797564f0176debba22c843`，共34路径（13源码、21文档）。提交后 index SHA-256 为 `df2745f04039f8e5d271d509c1802c4af63e10cb40654c4e855bcab65a45e8ed`。总控已独立核实远端、clean 和当时 Ref28 通过；此后的人类品牌改动是新的工作区事实，不能把阶段2再写成待 Git，也不追改 Ref28 写成时的 Pending 字段。

既有本地运行闭环据固定回执保留：API `d19f5c…` 的实际进程/挂载 ELF SHA 为 `992907675220c2e47c56547c85c0d2f1cbb4e1c6904c3d6084574d7f4612b045`，仅 identity+health；Web `9d8dd5…` 实际 served index 为 `eeea33cb00901137ecc9cb7c94b1607ffc2bbc57a6487c8ae329df957b6451e5`。24项后验为17 Pass、0 Fail、1 Partial、6 NotRun，原 Browser03 的15 Pass、4 Fail、5 NotRun不改、不当作全量重跑或唯一计数相加。F01失败分支未自然复现，F02具体CSV原因/修正仍Partial；没有产品最终验收或全栈同版结论。

之后真人在既有会话“重新设计第二大脑图标”明确要求重新设计 Web 图标。总控直接读取实际 userMessage，绑定源会话 `01a1b7f8-6180-74c1-a50b-fa48574f625d`、turn `01a1b7f8-7d6e-7370-afa2-01b69d02faf9`；开始/结束1794087878和1794088263仅是turn元数据，不造点击或批准时间。最小品牌两文件权限已明确，不再重复请示。

唯一已核实工作区变化是 `web/public/logo-mascot.png` 与 `web/src/conf.json`：PNG为814909字节、1254×1254、8位RGBA且有Alpha通道，SHA `dbcc3d91d13299d16af74f49683e7bf13204d39ba2239db03c18f18cef9b8cb2`；conf为140字节、SHA `55144749eaf93d04d23433941d434a72c0639b0bd3086a4455da475a8a8337ee`，仅 appLogo 从 `/logo-mascot.png?v=4` 到 `?v=5`，appName/projectUrl不变。产品品牌仍由该conf统一，原路径不重排。

实际全树代码快照仍5048份，新 canonical SHA-256 为 `2c786735cda277f5d02238a2c6086f4c1972c5ce12369ed51545c5afe2efa152`。PNG不进入既有代码扩展名算法，conf进入；仅在内存的canonical摘要表以精确HEAD的conf摘要做反事实替换，即重建旧5048/4226快照，主树文件从未临时改回或恢复。这来自实际测量及已核实真人范围，不是预测、也不是刷新旧锁求通过。旧 Ref28 原177920字节/SHA `57ceba610d02b05e4340fcda302607abd2bda9d12e5214c4e8c6b338203276c2` 精确保存在新的基线快照路径；本准备时原CLI仍如实因 source drift REFUSED。

总控已对源PNG做只读基础目视审阅：蓝/青脑形、白色书页线、金色连接点，无文字/秘密；这不是GUI上线验收。运行中的 Web 仍是阶段2已有v4静态产物，新Logo尚未构建、部署或GUI验收，不能称已上线。现有KCA-20261108-02具体人类A决定、subject `a6d98e012a735c3a2dd75d72f6492fa9928c98b5ee14ea8b98c800e79fdad0e3`、17入口及原14件保持；S1/S2/S3仍按原具体范围推进，当前A/B只是ignored候选，70组原产品验收仍全部NotRun。品牌请求不扩大旧owner-only0.2接口或模型、真实AICS、OAuth、生产发布、人格记忆、根目录/资源权限。

本候选只更新两项确实受影响的选择内容摘要及准确事实基线；97条/102用途、26正式契约、47保护件、11归档文档和28历史原路径清单不变。旧manifest快照另属基线链，不增加这些历史业务文档的指导用途。本准备仅可声明未修改校验器的四后像叠加 candidate-only 结果；须独审、总控冻结exact4并另授有限单次机械租约，实际应用后才执行主树Ref29精确校验。新锁raw SHA在独立交接中给出，不嵌入被锁内容。本准备0主树/Git/服务/模型写入，不外推全局零调用。

---
# 当前任务参考资料基线

**2026-11-08 第28基线候选：实际源码闭环与共享访问具体批准**

当前基线ID：`KCRL-20261108-28`；总控为Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。当前事实入口仍为`coordination-20261106-round3/current_context`，仅在原`/coordination_20261106_round3`内追加`stage02_and_access_authorization_followup`。旧35根字段、round3原28子段及其失败、时点、运行版本和验收范围原样保留；以下Ref27和更早导语均为各自原时点历史。

A已实际应用冻结exact13源码：10修改、3新增，Go2/Web11；实际全源码5048份，canonical SHA-256 `4226d996c441a79fd1a05475c26fc52a856b693a3bbfc0aaca1cdd81b00510ba`。实际回执SHA `65189ce25f2a52499cbdda2217a3e132ca5d5ca9ec70c903712f1230acaebec9`，Root另核真实13差异、范围外5035源码、47保护/26合同/49已提交路径、HEAD和index；旧5045/834afa38是批准前快照。此前5048预测只保留为预测，当前事实来自实际应用回执与独立实测。旧Ref27原字节SHA `4013cba9546dd693a32f62a07c801a5522fafe2fd66339a499f77b7fdc0f9ac7`原样归档；应用后的旧锁仅source drift拒绝，不谎报通过、不刷新旧锁。

分层验证按原回执登记：Go VALIDATE02真实red与4新/1既有top、6states sub绿，native父在m.Run前跳过，crop另计1top/3sub；BUILD01原build.sh --go实际构建CLI/server并完成四模式和CLI共5help。Web既有201与C独立37属于不同范围，不累加unique；BUILD02实际一次Vite/npm exit0，13817模块、1073产物/56165520字节，index `eeea33cb00901137ecc9cb7c94b1607ffc2bbc57a6487c8ae329df957b6451e5`。Go首轮timeout执行0、Web BUILD01 preVite失败、BUILD02预检失败及Browser03的15P/4F/5NotRun均保留；新构建不回填旧失败。新API/Web切换、定向GUI和阶段2 commit/push在Root本准备任务派发时仍Pending，旧Go57/Webfca运行证据保留原范围。各任务0调用不外推全局0；Q04的operator版本适用性边界不由此重写。

真人已在Root的一次具体问题中选择“A：实施三个阶段（推荐）”，精确批准KCA-20261108-02 subject SHA `a6d98e012a735c3a2dd75d72f6492fa9928c98b5ee14ea8b98c800e79fdad0e3`。新入口`access02-implementation-decision/implementation_authority`保存原问题、两选项、原答案及Root首次记录UTC；`access02-subject/implementation_design`和`access02-copy-map/reference_resolution`绑定原14件331418字节及独立正式路径映射。C 已完成决定/映射登记独审：17 件精确副本与用途、14 原件/subject 和 156 个相对引用一致，P0/P1/P2 均为 0，无实质修订要求；回执 SHA `9949fbb739f0b979f38043a505506e7fe08534cdded2fa3767382a42ed1429d8`。这只接受登记候选，完整 Ref28/21 件独审与正式生效仍是独立关口。

原Draft/Pending字段是写成时状态，任何字节均不回填；当前批准来自独立具体决定。按S1个人只读模块共享→S2部门授权→S3临时MCP凭据实施与合成验收，接受固定版本、结构化成员摘要、许可行稳定顺序、粗世代续签、默认1小时/最长8小时且可撤销的凭据及具名可配置Bearer客户端先行。70组产品验收仍全部NotRun，方案尚未实现。

上述批准只覆盖该exact subject，不把旧owner-only 0.2契约改成共享接口，不批准OAuth/任意Agent自动接入、新模型调用、真实AICS、生产部署/发布、人格记忆、根目录重排或破坏性删卷。旧26正式契约、47保护件、11归档、28历史原路径和80旧选择器/用途保留；17个新条目按具体决定、已批准设计、实施/验收计划、静态证据及合成输入分别读取，不一概升为已运行正式产品契约。后续代码任务须先取得实际生效且校验通过的新锁、总控给定raw SHA和具名有限源码窗，不能直接据ignored候选开业务写窗。最新真人要求每完成并验收一个阶段即commit并push，由Root另派Git执行者；旧历史禁令不覆盖这一最新明确授权，发布仍独立。

本导语和候选后像本身不授予正式写入。须C登记复核、完整21件独审、Root冻结逐件/集合摘要并另签具名有限单次exact21租约，实际机械应用后再执行Ref28精确校验；本准备包只可声明候选叠加校验，不能声明main已通过Ref28。新锁raw SHA只在独立交接/闭环回执提供，不嵌入被锁内容。

# 当前任务参考资料基线

**2026-11-07 第三轮 shared ordinal 受控后续事实入口**

当前基线ID：`KCRL-20261107-27`；总控仍为Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。当前入口仍为`coordination-20261106-round3/current_context`，仅在原`/coordination_20261106_round3`内追加`shared_ordinal_followup`，观察截点为`2026-11-06T17:02:41.000Z`。旧28源码、原main/runtime/失败/验收时点和既有范围原样保留。

后续只记录已冻结命名源码集合的内部ordinal修复候选及C源审、physical02真实Close、同MD新revision保存而R2 index_failed、旧R1 ready/mixed及分层unit/build/实际应用回执；R2失败根因仍为Unknown，不由候选或单元测试反推。真实应用后的源码为5045份/SHA `834afa38cfd22b83fcd3d599fe4ec7e1a6e57e22428ab3e2e517a1b460260359`，仅来自已核对的实际命名源码应用回执和全源码实测；旧5043/07153观察保留。unit01缓存初始化permission denied和新4 tests执行0按原失败保留；unit02因公开副本缺少既有fixture而失败，新4 tests、ES与native尾均NotRun；unit03 focused首项通过、第二项因SQLite fixture缺表失败，尚未进入阅读序断言；unit04及后续尝试逐次记录，Root最终接受的unit回执单独绑定。构建阶段另据实际回执记录，不将后续通过回填前次失败。

本导语和候选后像不授权正式写入。须独审、总控冻结精确四后像与独立摘要、具名机械执行者及有限单次exact4租约后方能生效；正式生效还须实际Ref27校验。保留26合同、47保护件、11归档、28历史原路径及所有既有选择器用途。Go全量构建、运行切换、18Q、SDK replay和评分未完成门不标Pass；不续授新接口、模型、真实资料、部署、发布或产品最终验收。新锁raw SHA在独立交接/实际闭环回执给出，不嵌入本锁所锁内容。

### 历史基线26导语（原时点原字节）

**2026-11-06 第三轮协调、源码与分层验证入口**

当前基线ID：`KCRL-20261106-26`；总控仍为Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`，只做统筹、协调和复核决定。当前事实入口只读`coordination-20261106-round3/current_context`，选择器为执行记录的`/coordination_20261106_round3`。本轮事实准备截点为`2026-11-06T05:11:24.000Z`；各观察保留执行者、原时点与运行版本。旧25、24和23导语及旧记录原值保留，只作对应时点追溯，不恢复旧写窗。新锁raw SHA只在独立安全交接/实际闭环回执提供，不嵌入本锁所锁内容。

正式机械写入26前，writer必须独立核实28个已审后像、B的实际`AppliedExactPostimages_PendingReferenceClosure_NotBuildOrGUIAcceptance`回执及实际源码5043份/SHA `07153ec9b040d6b85e086b45394aedbe2aeb73625464ad3a1c35a76286af17cd`，算法为`sha256-canonical-path-kind-content-sha256-v1`；准备时主树5034/7850仍只是前时点，预测和旧15c07/5042值均不冒充应用结果。写入还须D独审通过、总控冻结精确事实/四后像并签发独立SHA和有限时效的单次四文件租约；本段与候选文件本身不授权写入。HEAD和index绑定在独立机械包。

Web第五次实际完整生产构建已成功，13816模块、1072文件、56164043字节，index `fca79096b0e408c2d89d7c1e596b99312a0d7e563886dcb0a370548599f69c9c`，已真实只读切换29395；四次旧失败及worker0首轮884文件partial原样保留。新Web/c17 GUI为21项中13Pass、8NotRun、0FailureObserved：从零建domain/group、同registration r1/r2/nativeDOCX及390/1280/1600和旧固定读取有本轮证据，真实并发、失权、自然C02 by-key404和新Go等缺项不升格。旧873的390失败和005阻塞历史不改；知识group不增加文件系统3根/6文件806字节。

SDK04在c17上已完成TS1.30.0/Go1.8.0各一次兼容/默认关闭门，合计20/40wire，后台GET404为保留的nonfatal事实；随后04:26的两官方SDK默认关闭/实际GET405门已通过：TS1.30.0和Go1.8.0各一次、各6wire/2business，总21wire=9metadata+12SDK，source/derived body0、retry0。自发GET405/no-store真实观察；AllowPOST仅来自另计同origin头探针，不回填未记录Allow的background rows。Go recorder405 warning1/publicSDKlogger0，TS advisory0。该范围关闭server GET405 P2，不代表旧published-index stale或固定证据/read业务全过。Go405四文件最终候选、格式和绑定定向单元已闭环，5个top/4个outer JSON subpass；16个router子表项是源码/控制流计数，native父35个body被TestMain绕过不算通过，crop仅1top/3内联。不是全部Go、integration/e2e/native验收。

两次完整server构建均未取得完整wrapper验收：monitor02查询失败与residentguard03资源停止分开保留，不推成Go源码失败，原build.sh完成码Unknown、原03 API help执行0不追改。后续独立既有ELF验证NoNewBuild已核实server/CLI结构、Go1.27.3、明确输入和各一次help；完整native/toolchain仍PartialBeyondExplicitChecks。后续另授的真实c75切换已完成：新API696cc133…，实际/proc1原路径/dev2096/inode271451/334083768字节/单link0555/SHA c75fd9ae6ee515fd01bc84221eb6ecbff0139a9f28fec7b4480f29fe59359fc6先完整验证，随后52次预算内health轮询于66496/180000ms得到200/status ok。原切换本身只通过identity+health；其后独立SDK405门通过。B短GUI又在1280/390保留6固定refs、两oldnative head和empty[1453,1453)实际点击；仅一个当前r2 DOCX新task成功，新producer33347528…/enginec75，原source/revision不变。CSV390首次刷新回读确认缺失，后续正常回读成功且原因Unknown；server6453ms但首次终态39150ms>30s，不算限时观测通过。没有重跑21matrix/race或证明全局0，剩余parsed/tokenizer/旧published-index stale兼容仍须独立范围。旧a491/c17停止保留，controller stop exit137/noOOM分开记录；bf550和partial cache保留。Web840192/29395/indexfca790未变，私有env/cmd/config/网络/端口/5models/tokenizer仅保留内存比较，不导出值或秘密摘要。原03失败、Unknown build完成码和PartialBeyondExplicitChecks不追改，也不宣称全栈同版。D的c17 Linux CSV/DOCX各一次新任务成功，CSV计数NotExposed/null，DOCX仅任务级0；旧超时及未明因果保留。

质量amendment02只有总控治理批准，没有真实转运或操作员许可。18Q、答案/评分、MD/TSX导入、Q根、两次发布、operators及unblind均NotRun；D只能在未来另授的精确helper窗转运且不作操作员，A只管运行/metadata不碰新7raw，B/C原角色不变。Go辅助helper02现已补到真实Linux CGO0/offline/own-tmpfs proof：test-c、build及fullrun各一次，15top/39sub PASS，expiry12是39子集，fail/skip0；default/help各一次exit1及零rawReadAPI。C-QGF-001由source关闭补到上述实际拥有层测试；没有真实Q转运/卷证明。proof01错误Go list模板exit1、proof02宿主envelope误停均原样保留，后者按实际source更正且未重复default/compile/build。D ownNTFS完整handoff实际在04:04、早于旧04:11截点；旧候选只是C未读完整新件。metadata-only旧失败保留，own0x81 narrow6/full11通过、host body/direnum API0，9file+32dir readcap和70metadata-only为opens非unique/syscall；真实源readcap/C独审/Docker bind/Q卷/transfer仍未授。Go锁升级无需重编译，但host25→26须新caller/模板/grant精确重绑；Node127仍只证明原单入口不可用。旧host caller硬绑25/7850，26必须新精确caller/模板重绑。

B已实际完成精确28源码应用，当前canonical源码5043/07153，实际final result绑定见新记录；这只进入PendingReferenceClosure，不是build/GUI/质量验收。旧25 source声明有意stale，不刷新旧锁。

本锁只确定资料用途与上述分层事实。保留34旧根字段、4605原值片段、13旧选择器、21行runtime_rebuild、26合同、47保护件、11归档和28历史原路径；23/24及archive index原字节不改。全量typecheck/integration/e2e、质量/盲测、真人最终产品验收和发布未建立。无模型新调用、真实AICS、新业务接口、部门Grant/临时MCPKey、人格记忆、根目录重排、删资源/卷、重建凭据或commit/push授权；执行者报告内0调用不外推全局0。以下25导语及旧段为原时点历史。

### 历史基线25导语（2026-11-05 12:43:11.508Z原时点）

**2026-11-05 第二轮协调与分层事实入口**

当前基线ID：`KCRL-20261105-25`；总控仍为Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`，只做统筹、协调与复核决定。最新本轮事实只读`coordination-20261105-round2/current_context`，选择器为执行记录的`/coordination_20261105_round2`。原`coordination-20261105-current`及`/coordination_20261105`保留24锁04:37Z原快照，只作该时点追溯；不覆盖本轮后续事实，不恢复旧写窗。`foundation-current`继续保留原需求、续作顺序和既有批准对象范围，其旧主控、Paused及旧未完成项不作本轮进度。新锁文件raw SHA只在独立安全交接中提供，不嵌入本锁所锁内容。

实际源码为5034份，SHA `7850af3d42edd8a3b1426691c15a5bf20b9f9fb8e3a250c3966f0f7e525f9666`；HEAD `87d61549e4b8ecdf2827170c7851b2ed7bb0c251`。本轮10修改、2新增共12个Web路径已按冻结V3/工具栏候选字节应用，主树4套定向测试103/103，lint为0 error、3 warning，format通过；不与staging103或更早70/16/17相加。Go273/c17既有构建、定向单元与五个help、原生四类链路及官方TS/Go SDK结果均按各自运行时点记录，最终Web应用后的Go输入只核实3318项、39,868,185字节，`conf/private.pem`一项为`Unknown/Excluded`且actual kind/SHA为null，状态为`NotEstablished_Partial3318Plus1Excluded`，`full3319_binding_claim=false`；原manifest未记录kind，不虚构历史kind相等。该例外未作内容读取或单独摘要输出，公开默认来源尚未确证，详见新入口，不把历史结果改成新重跑。完整Web生产包1071文件、56,110,637字节，index为`873a96be6822dc8eb54e22462026fca326a374765fd9108b1481294838fb6ce3`，已只读挂载29395，c17 API未重启；保留首次Less超时未定因及独立attempt02命令显示更正证据。

仅将12项中的桌面和核心链路改进记为已应用事实，不宣称整个候选、移动端或产品最终验收通过。新873包实际验证了TXT/CSV自动刷新、无任务手动刷新、计数更新及固定引用/索引保持，并复用既有index完成四类固定读取78/89/131/93字节。004只在1280/1600桌面通过；真实390下新增cells strip为0宽，固定阅读不可用，状态为`Failure_Observed`。父侧栏/高度/padding机制未改与新增局部滚动条失败同时保留；缺少before实测，不能把全部失败归旧布局或断言全为新退化。005为`NotRun_BlockedByExistingC2OriginalWorkflow`：同owner/domain旧回执缺少新登记入口，本轮没有新DOCX上传或新R2任务。busy/late/race仅`UnitOnly`，新project/head/后续原件修订不匹配的浏览器情形未验。C2与窄屏后续候选仍需独立范围决定和写窗，本记录不扩大当前12路径。

全量typecheck、完整integration/e2e、质量/独立盲测、真实资料及模型作用评分仍为`NotRun_CurrentNotEstablished`；历史207诊断不算本轮失败。旧315版143字节TXT timeout根因仍未解决；shared315 Admin及原生依赖输入未全绑定，不宣称全栈同版或完全隔离。全局model counters未仪表化，各执行者获准动作范围内的计数不外推为全局零调用。真人最终产品验收、commit/push/发布未发生。

新25只锁定本轮已审定事实、原资料用途和读取范围，不产生接口冻结、模型、真实AICS、部门Grant/临时MCPKey、人格/记忆、根目录重排、删资源/卷或重建凭据授权。正式修改仍须总控另签精确单写窗；旧33根字段、21行runtime_rebuild、12旧选择器、既有契约/保护对象与23/24历史原字节保留。以下24导语及之后23导语均为原时点历史，不构成第二个当前事实入口。

### 历史基线24导语（2026-11-05 04:37Z原时点）

**2026-11-05 当前协调与阶段事实入口**

当前基线ID：`KCRL-20261105-24`；当前总控：Codex会话`01a1a9a5-c724-7f52-8cbb-16d19d982ba4`。总控只做统筹、协调与复核决定；开发、测试、运行及获准的机械记录由具名执行会话承担。最新真人原话及本轮截止事实只读`coordination-20261105-current/current_context`，选择器为执行记录的`/coordination_20261105`。新锁文件SHA由总控在独立交接中提供，不回填本锁所锁选择器，避免摘要循环。

先校验总控提供的24锁SHA，再读`coordination-20261105-current`作为当前协调/阶段事实。`foundation-current`继续保留原需求、续作顺序和已批准对象范围；其旧Paused、旧主控、旧source及旧未完成项仅属历史时点，不作为本轮进度。既有current命名选择器和批准/保护对象原值保留，不因名称获得新的验收或写权。

本轮当前源码：5032份，SHA `7dd39b05c2f69f68de15e3c48fe8f2f0a351d9ac66192e5a180287c7848e2dd9`；HEAD `87d61549e4b8ecdf2827170c7851b2ed7bb0c251`。仅两份已批准Web文件改变，定向70/70、lint/format通过；新原生bundle未建。临时315运行版的源码输入仍未绑定，三类HTTP/两SDK通过不等于包含R1的最终四类验收。143字节TXT timeout根因未决；同版完整Go/五help、质量/独立盲测、最终人的验收和发布均未完成。

总控复核具体事实、路径和摘要后，才可向具名执行者签发唯一、精确、互不重叠的写窗来机械更新。子任务不得自动刷新摘要使失败通过。仅合成资料；模型/OCR/转写/embedding/rerank/generation新调用、真实AICS、部门Grant/临时MCPKey、根目录重排、批删资源、删卷/重建凭据、commit/push/发布均不在本轮范围。

以下23锁导语明确保留为2026-10-30历史记录，其中Claude主协调及当时状态已被以上当前协调安排接替，不构成第二个当前主控。

### 历史基线23导语（2026-10-30原时点）

> **2026-10-30 最新状态：真人已要求恢复开发。** 新唯一主协调为本地 Claude Code 会话（接替`01a18185-7d13-7b43-af32-f6c03ab768f6`）。原检出目录已删除：暂停检查点所列 `.local-data` 私有证据、零版本修正候选、29394/29395 隔离服务及原 POC 六服务/卷均不在本机，只作历史引用，不补造。已按真人决定 KCF-021-20261030-01 修正默认关闭 `local_policy_version:0` 的策略视图 schema 三处副本；MCP（纯 Go、CGO 关闭）与 Web 定向测试红绿通过。完整 build.sh 构建、隔离合成运行环境重建、混合索引/发布/召回与质量评测均未执行。暂停时状态见[暂停检查点](knowledge-core-pause-checkpoint-20261030.md)，恢复事实见执行记录 `resume` 段。
> 
> 基线ID：`KCRL-20261030-23`；日期：2026-10-30；维护者：唯一主协调会话（本地 Claude Code，接替`01a18185-7d13-7b43-af32-f6c03ab768f6`）。本文件确定**资料用途和读取范围**，批准事实来自真人的独立决定，锁本身不产生批准。机器清单为[引用锁](knowledge-core-reference-lock.json)，校验／受控读取入口为`tools/scripts/knowledge-reference-lock.cjs`。前两版原字节快照见[第一版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-01.json)及[批准前第二版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-02.json)，及[实施启动第三版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-03.json)，以及[后端交回第四版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-04.json)，及[Web交回第五版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-05.json)，及[组件交回第六版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-06.json)，及[首片交付第七版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261028-07.json)，及[基础启动第八版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261029-08.json)，及[内部投影/HTTP第九版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261029-09.json)，及[单一归一化第十版](archive/knowledge-core-20261028/reference-baseline-KCRL-20261029-10.json)，均不能继续作本轮入口。

---

旧ID `coordination-current` 仍保留23锁所登记的2026-10-28历史前缀及原选择器，只供该原范围追溯；它不是本轮协调/阶段事实入口，也不恢复旧写窗。

## 1. 先读规则

1. 最新真人指令优先；本基线不能覆盖平台指令、真人限制和适用AGENTS。用户原话、Agent工作计划、代码事实、既有契约和候选草案分别解释，不能互相替代。
2. 每个后续任务先取得主控提供的基线ID、引用锁文件SHA-256、固定源码状态、允许的资料条目ID／用途和写窗。先校验，再读分配条目，并回报采用的基线；未取得写窗不写源码／Git／部署。
3. 校验失败、摘要漂移、源码事实基线变化、资料未登记或用途不符时，停止依赖该资料的动作并向主控回报。子任务不能自行更新摘要、扩大白名单或把草案升为冻结。
4. 文档里的链接不继承信任。搜索命中、文件名含v1/final/freeze、较新的时间、旧会话消息和“当前／下一步”字样均不产生执行权。
5. 旧方案可解释历史，不能重新启动复杂格式扩展、人工Wiki、人格／记忆或旧A/B排程。历史批次的批准不延续为本次发布、模型或真实资料授权。

## 2. 当前资料的用途

| 类别 | 条目／读取范围 | 可作依据 | 禁止推论 |
| --- | --- | --- | --- |
| 真人原始方向 | `product-request`：旧执行记录的`/request_snapshot` | 知识库核心、RAGFlow改造、统一模型、权限与协作愿景 | 不能从“现在开始”推导模型／部署／新接口批准 |
| 本轮锁定要求 | `reference-lock-request`：本轮记录的`/request_snapshot` | 固定可信资料，防历史跑偏 | 不扩大产品实现范围 |
| 工程规则 | `root-instructions`、Web任务另读`web-instructions` | 当前代码／构建／格式／安全与协作限制 | 上游迁移建议不能成为批量删除授权；目录重排仍须真人确认 |
| 当前工作计划 | `current-plan`：主线计划全文；`foundation-current`：最新真人续作顺序、当前基础任务与交接 | 工作包、依赖、先后、交接与验收计划 | Agent保存计划不等于人的验收或具体接口冻结 |
| 当前事实快照 | `code-inventory`、`retrieval-mcp`、`backend-current`、`web-current`、`mcp-current`及`live-current`；`model-research`仅固定对照版本 | 按已列源码及固定官方版本核实的事实／缺口 | 静态能力不等于真实联调通过；研究建议不是已批准设计 |
| 新设计草稿 | `unified-model-draft`、`content-schema-draft`、`local-access-draft` | 设计讨论、离线映射和影响分析 | 0.2已实现的内部投影/HTTP定向结果按foundation-current读取；其余设计仍为候选，不能宣称整体完成、人的验收或正式契约冻结 |
| 已批准首片设计 | `lexical-slice-draft`、`lexical-api-draft`、`lexical-slice-bundle`，用途`implementation_design` | 真人绑定KCLS-20261028-01/subject摘要批准的owner纯词法实施；三件原字节不改 | 文件内Pending是写成时状态；真实批准读`implementation-decision`。不批准整个模型/部门/Grant、模型调用或部署，不代表已实现 |
| 实施权与交接 | `implementation-decision`、`implementation-handoff` | 真人原问题/原答复/批准摘要；单写窗及三开发会话交接 | 执行者不能自行扩权限/换版本/互发消息；工程最终Acceptance仍独立Pending |
| 本轮执行记录 | `execution-request`、`execution-current`、`execution-evidence`，各按选择器读取 | 启动/子代理澄清原话、实际开发交回及分层检查 | 不把研究完成当业务联调通过或人的批准 |
| 合成输入 | `synthetic-fixtures` | 4份原件／3类格式、39人工事实和8问题 | 夹具内NotRun是输入写成时状态，不能改写；实际Go/SQL/ES及原生HTTP/MinIO/Web/MCP验证另见execution-evidence和live-current，原夹具状态不回填 |
| 既有正式契约 | 26份已登记formal条目，按功能读取 | 既有C1/C2/R1/R2接口及原批准边界 | 不是新模型／部门授权／本地检索接口的批准，也不是当前开发顺序 |
| 既有工程证据 | R1、R2后端、R2客户端报告的登记字段 | 该版本、输入和方法下的结果及未验项 | R2中间／Mock报告不代表最终浏览器／整体退出；Pending不改Approved |
| 混合历史容器 | README与接续只读获准前缀；技术方案只读第41章；旧协调说明只读2026-10-28覆盖段 | 当前入口及历史边界提示 | 不读完整容器后自行挑选旧“当前”段作为指令 |

新增统一0.2具体实施对象读取foundation-interface-decision（真人原决定）、foundation-interface-subject（逐件摘要）、foundation-interface-review、foundation-parsed-schema、foundation-lexical-protocol及foundation-lexical-schema（限定implementation_design）。这些条目只批准固定对象和同范围修复/合成验证，不把广义设计草案、最终验收或发布升格。

以上条目路径、SHA-256、选择器、用途和限制以机器清单为准。事实文档锁定的是源码快照；后续源码变化必须重新核实受影响事实，不能只重算摘要。

## 3. 历史资料处置

历史入口统一为[归档索引](archive/README.md)。已实际归档11份旧说明；28份因引用链保留原路径，已明确降为历史参考，均不作正式开发指引。移动只发生在docs内，归档清单记录原／新摘要和位置。

| 资料 | 当前判定 | 正确用法 |
| --- | --- | --- |
| README／接续中v0.62及更早段落 | 历史快照；如“正式Web／故障／容量待做”已不代表最新事实 | 按需由主控追溯，不能指导当前任务 |
| 技术方案第1—40章 | 历史目标／决定／交付；第35章含旧阶段6人格记忆等排程 | 仅在明确历史核对任务中读取；当前按第41章与主线计划 |
| stage1/stage2/stage3的handoff、client-notes、execution-plan | 旧写窗和旧后续动作已过时 | 不恢复旧主A权力、旧客户端写权或旧格式/Wiki关键路径 |
| 21份受保护旧candidate | 原字节必须保留，但已是历史候选 | 既有接口读formal文件；追溯批准对象另行指定 |
| `knowledge-sources-v1.draft.*`等未登记draft | 旧草稿，不能作为现行契约 | 不因v1文件名提升可信度 |
| 已归档的`stage3r2-document-runtime.md` | 运行历史说明，记录的Web index为旧值 | 经归档索引追溯，不据此替换当前运行版本；现状须按主控任务只读实核 |
| 上游指南、release notes、包README、旧会话副本 | 未纳入当前产品方向；有用技术片段也需按拥有路径复核 | 主控按具体问题解锁，不自动继承上游的部署／Agent／Python安排 |
| ignored运行证据和私有配置 | 不进子任务默认资料包 | 主控只提取获准安全字段；不输出凭据、token、nonce或完整私有配置 |

未列入可读条目的其他文档默认是“历史或未复核背景资料”，不默认判为内容错误，也不能作当前执行依据。源码可按任务定位真实拥有路径；源码中的注释／包文档同样不能覆盖真人范围。

## 4. 校验与读取

从仓库根执行（只读，无网络、无安装）：

```powershell
node tools/scripts/knowledge-reference-lock.cjs check
node tools/scripts/knowledge-reference-lock.cjs read --id current-plan --use execution_plan
node tools/scripts/knowledge-reference-lock.cjs read --id technical-current --use current_context
node tools/scripts/knowledge-reference-lock.cjs read --id unified-model-draft --use design_reference
```

正式派发还必须将主控给出的实际引用锁摘要传入`--expect-sha256`，不能把工具当次算出的摘要自动当成主控批准。命令`list`列出可读条目；`read`只能用登记ID，不能直接传任意路径；章节／字段由选择器限制。脚本没有刷新／批准命令。

脚本检查：资料与选择器摘要、47保护件原摘要、原话摘要、既有契约类型和当前源码指纹。源码指纹基于当前跟踪／非忽略代码文件的实际字节，允许纯文档提交而不依赖“本次提交必须引用自身HEAD”的循环。

这是工程工作流的读取约束，**不是操作系统文件权限隔离**。普通文件工具仍能打开文件，因此AGENTS和主控派发必须同时约束用途；不得声称技术上禁止了所有旁路读取。

## 5. 后续派发必须携带

```text
任务ID / 主控 / 执行者 / 固定分支与HEAD
资料基线ID + 引用锁文件实际SHA-256
允许读取的条目ID + 各条用途（只读登记选择器输出）
源码范围 / 唯一写窗或ignored白名单 / 47保护件
当前候选状态与未验项 / 输入输出 / 验证与回报点
启动回报：锁校验结果 + 实际采用条目ID与用途
遇资料冲突：列冲突原文和条目，不按时间或名称自行裁决
```

最新真人要求先补统一基础，再做复杂数据召回与MCP作用实测；首片通过不等于原始产品主线完成。当前继续范围与原话见foundation-current。

只有主控可在同范围授权内复核并决定基线变更；按2026-11-05最新真人分工，机械应用由获得精确单写窗的具名执行者承担。涉及需求、接口冻结、模型范围或发布的新决定仍由真人作出。每次更新保存旧基线原字节、摘要、变化原因、受影响条目和新校验结果。

## 6. 历史观察（2026-10-30及此前，不作本轮进度）

当前源码5032份，SHA `378f5cd7c4da198aa153a7beb49313034c151529b17a11fcf61dbce55ed4f8bb`。统一模型、解析归一化、parsed发布/上传、JSON数字精度及8路径解析保真修复已有分层工程证据；7阶段架构和16生产者+4结构条目已在29391真实中文页面核对滚动与固定来源读取。

真人已批准具体候选KCF-020-20261030-01，subject SHA `2495953aec14e4a62d0a703d05759a81e599716a0fd9d09e554b302f109d795b`。新接口的实施依据只读foundation-interface-decision及其绑定对象；广义unified-model-draft仍不是整体冻结契约。默认关闭、当前所有者、固定修订、纯词法与三个只读MCP不扩权。

同一核心的四类混合检索26拥有层路径已独核并合入，共享Go公共输入3264份与已验候选一致；13服务、5handler、2router为各自运行，38注册HTTP响应/190schema/15语义检查限SQLite、内存存储与ES替身。候选同版二进制及四模式/CLI help已验，尚不是最终组合运行版本。源窗之外逐行指纹、52保护文件及Git index保持一致。

旧29390运行版另已完成PDF6节点/91字节、PPTX8节点/94字节和隐藏幻灯片标记，75请求中两次首次确认401均保留并由原键恢复。离线已复现上传Close污染持久连接context；现场连接因果尚未打点，不能宣称401已修复。

Web候选手动流程的历史切换/新任务启动和MCP投影/响应预算/实际API接线10路径已合入，准备版14Go顶层/30子项通过，真实SDK待验。上传EOF与关闭watcher的7路径修复已合入；最终组合与新原生复验未完成。新MySQL/MinIO/NATS/ES/Web/MCP原生基础和复杂召回质量均未通过或未启动，必须继续。

下一步：串行核收上述修复与组合构建，在新增独立合成环境打通原生保存、快照、解析、四类混合索引/一次检索/准确回读、Web和两个标准MCP SDK；基础通过后再运行复杂已标及独立盲集。模型/真实AICS/部门Grant/临时Key/原POC替换/发布不在批准范围。
<!-- reviewed-source-current -->
Web手动原件/快照/解析/取消/固定回读和四类统一词法消费的17最终路径已独核并合入；历史切换与终态再解析回归关闭。102测试、38原body/status回放、lint/format通过，类型基线207且新增0；2025构建输入和1673产物131830790字节逐件摘要核实。原架构/滚动和旧schema字节保留。统一26、上传7、MCP10及Web17已组合源码，最终Go构建、真实MySQL/MinIO/NATS/ES/Web/两个标准SDK尚待；先基础后复杂召回及独立盲集，质量未启动。
