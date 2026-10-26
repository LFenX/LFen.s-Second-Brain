# S2-R5 客户端单写入交接

日期：2026-10-26。本页是明确写窗的范围说明；实际交权以主会话发送的固定HEAD与消息为准，不能仅凭此页自行写入。

## 固定后端与当前事实

- Go源码`1ac4e4f3cd0ca74e79ba10e71593287c18bc98ac`；后续纯文档提交不改变该运行源码。分支`codex/stage2r5-web-import`，main仍R4，不在本窗合main或部署。
- 二进制SHA：`2a7de569de80c9f27ed549ab10dc9de60519ecce0a52ad519c615919e8a37502`。本机API已加载，六服务沿用原Compose／卷／凭据。
- [正式冻结](knowledge-web-import-v1.freeze.json)、[34操作／54schema](knowledge-web-import-v1.openapi.json)、[正式向量](knowledge-web-import-v1.examples.json)、[29保护项及验证](stage2r5-backend-verification.json)。0.2—0.6、C1/C3及历史候选均不可改。
- capabilities已实际返回runtime_ready=true，所有固定限制／四版本与冻结一致。本地上传、观察、两项预览确认、回执／固定读、128字符大小写键、loopback在拨号前拒绝与原次acquire回放、越权404、Range416通过；38响应。不是完整R5、全部网络、浏览器或容量验收。
- 合成运行元信息在忽略目录`.local-data/stage2r5-20261026/runtime-state.private.json`；既有合成账号与认证仍由A操作，不把凭据复制进源码／聊天／构建输出。该Capture已明确取消，固定回执／修订仍有效，后续演示另建合成Capture。

## 唯一写窗

主会话交权后，接方可写以下20个生产文件及对应`__tests__/`合成测试、新建本轮客户端交付文档。其余源码、根文档、契约、后端、依赖／锁文件、共享UI和全局request不改；不得操作运行服务或模型。确需越界先回报A，不自行扩展。

| 范围 | 文件 |
| --- | --- |
| 请求／响应类型 | `web/src/interfaces/request/knowledge-web.ts`、`web/src/interfaces/database/knowledge-web.ts` |
| 客户端／Hooks | `web/src/services/knowledge-web-service.ts`、`web/src/hooks/use-knowledge-web.ts` |
| 局部校验／摘要／阅读 | `web/src/utils/knowledge-web-schema.ts`、`web/src/utils/knowledge-web.ts`、`web/src/utils/knowledge-web-reading.ts` |
| 流程 | `web/src/pages/dataset/knowledge-import/web.tsx`、`web-fields.tsx`、`web-capture.tsx`、`web-observation.tsx`、`web-preview.tsx`、`web-receipt.tsx`、`web-reading.tsx`、`web-revisions.tsx`、`use-web-flow.ts`（后七项同一目录） |
| 局部接线 | `web/src/pages/dataset/knowledge-import/index.tsx`、`web/src/pages/dataset/knowledge-catalogue/index.tsx`、`web/src/locales/zh.ts`、`web/src/locales/en.ts` |
| 交付说明 | 可新增`docs/stage2r5-web-client-delivery.md`及本批客户端验证JSON；不改A的统一后端报告 |

A在写窗内对源码／Git只读，只在忽略目录执行现有二进制的合成运行／容量／故障验证。接方完成后跑局部验证、正常提交并交回清洁HEAD／文件清单／结果／缺口。A收回写权后统一真实联调及main推送。不得两边同时改同一检出。

## 客户端约束与落地顺序

1. **service与类型**：只走Go；局部1.0.0＋0.6.0头，34操作及54schema；按冻结typed-LP精确投影，不复用0.5摘要。数据样本的runtime_ready是动态boolean，候选样本false不改变当前后端true。沿用next-request；PUT须保留原Blob，二进制只接受200／octet-stream／长度及X-Content-SHA256，不搬0.5 Range／ETag。
2. **Hooks与原键槽**：统一factory，按session／owner／domain／registration等分隔。主体取C1已授权Domain.owner_id，不能读取本地profile.id。严格RecoverySlot字段，三组原请求和选择完整落槽，先GET恢复，未知不换键；本地可显式重传原件，在线每Capture一次，失败不能重新acquire。
3. **四段流程**：登记／创建与明确取得→唯一观察／精确选择≤32→固定预览及逐项初始未认可→明确确认／固定回执。独立Acquire默认未确认；preview选择变更须清空旧认可。来源／完整性／未索引状态清楚展示，不能把Feed首32项称为全Feed导入。
4. **阅读与目录**：mode=web接既有导入与目录入口；实际固定WebRevision、完整表示／UTF-8窗口／引用分开。raw／decoded仅转义或下载；safe_reading整件SHA核验后一次挂空sandbox和固定禁外连CSP；不使用SafeImg、不依赖iframe脚本或postMessage。父页独立引用明确打开，noopener／noreferrer，不自动获取图片／附件。
5. **恢复与异常**：源站401/403/404是API502＋Failure，不能触发本机登出；本机401／404、策略／契约冲突清敏感状态。revision倒序丢弃，同revision矛盾拒绝；三次状态失败转人工。分页不足20仍跟next_cursor，守32KiB与UTF-8字节预算。窗口不把半截HTML直接显示为阅读。
6. **unchanged语义**：本次Receipt.capture_id可与旧WebRevision.capture_id不同；按entry_key／输入及表示摘要／分类核对，不能改旧原Capture指向。分类覆盖必须重新投影source_input，不能强制等于观察默认分类的旧摘要。

## 回交标准

- 正式向量、HTTP wire／34操作、三组原键／单次acquire、Hooks隔离／单调修订、四段页面／逐项认可、空正文／安全阅读／资源不自加载、源站502与本机认证分离均有合成测试。
- 运行改动文件格式／lint、相关测试、生产构建与类型增量；保留既有207项类型基线，新增必须为0，不修无关全仓类型问题。遵守`web/AGENTS.md`及正常hooks，不绕过。
- 仅合成、模型0；不发真实网页／RSSHub请求，不部署、不改后端／冻结契约、不合main、不启动多轮审查。交回实际提交、完整文件清单、命令与通过数、未完成项；测试成功不当作人的验收。
