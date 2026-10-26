# S2-R5 Web 客户端交付

日期：2026-10-26。分支：`codex/stage2r5-web-import`；客户端起点：`be10950ab2b955d2462975c0af52c9d99d8a93db`。工程状态：**客户端实现及合成验证完成，人的验收 Pending**。完整摘要、命令、文件 SHA 与需求对照见 [客户端验证记录](stage2r5-web-client-verification.json)。

## 已实现行为

- 既有知识域内新增 `mode=web`，依次完成登记、创建与明确取得／上传 Capture、唯一观察与精确选择、固定预览与逐项认可、总确认及固定回执。最多明确选择 32 项，保存不代表导入整个 Feed，不启动订阅、模型或索引。
- 34 个操作沿用现有 Go／Axios 请求层；局部请求头固定 1.0.0＋0.6.0，全部等待 **25 秒**。原始 PUT 保留原 File／Blob，服务端上传单次尝试上界仍为 30 秒。源站 401／403／404 保持 API 502；本机 401 仍由现有认证拦截器处理。
- 使用正式 0.6 typed-LP 投影；分类覆盖重新计算来源输入摘要，预览核对全部所选项、元信息与引用预算，确认核对消费后预览与固定回执。`unchanged` 修订保留自己的旧 Capture；本次回执 Capture 不覆盖它。
- 授权主体取 C1 `owner_only` 域的 `owner_id`。完整原 Capture／Preview／Confirm 请求先落入严格有界恢复槽；登记另存原键与配置元信息，不存正文或凭据。刷新先 GET 原结果，unknown404 保留原键，只允许明确重发同一请求。在线 Acquire 不自动重试；超时后只核查状态。确定拒绝后仍需再查原键，才能明确重新准备。
- 查询按 session、owner、domain、registration 和筛选项隔离；旧 revision 不覆盖新状态，同 revision 矛盾拒绝；连续三次状态读取失败停止自动轮询。失权或策略／契约冲突清除可见敏感状态，保留主体隔离的恢复记录。
- 目录保留文本默认视图，网页修订按所选分组懒加载并支持登记筛选。固定链接同时携带修订、来源与输入摘要。
- 解码正文和 UTF-8 窗口只转义显示；窗口保持连续范围、表示描述符与已核验正文一致。完整安全表示核验长度和 SHA-256 后一次挂载空 `sandbox` iframe，父页加入固定禁外连 CSP 并剥除活动标签与属性。引用由父页独立展示，明确打开时使用 `noopener noreferrer`。原快照只下载；不套用 0.5 HEAD／Range／ETag 协议。

## 文件清单

20 个生产文件均在 [单写入交接范围](stage2r5-web-client-handoff.md) 内：

| 范围 | 文件 |
| --- | --- |
| 请求与响应 | `web/src/interfaces/request/knowledge-web.ts`、`web/src/interfaces/database/knowledge-web.ts` |
| 客户端与 Hooks | `web/src/services/knowledge-web-service.ts`、`web/src/hooks/use-knowledge-web.ts` |
| 校验、摘要和安全阅读 | `web/src/utils/knowledge-web-schema.ts`、`knowledge-web.ts`、`knowledge-web-reading.ts` |
| 四段流程 | `web/src/pages/dataset/knowledge-import/web.tsx`、`web-fields.tsx`、`web-capture.tsx`、`web-observation.tsx`、`web-preview.tsx`、`web-receipt.tsx`、`web-reading.tsx`、`web-revisions.tsx`、`use-web-flow.ts` |
| 局部接线 | `web/src/pages/dataset/knowledge-import/index.tsx`、`web/src/pages/dataset/knowledge-catalogue/index.tsx`、`web/src/locales/zh.ts`、`web/src/locales/en.ts` |

7 个测试／夹具文件：

- `web/src/utils/__tests__/knowledge-web.test.ts`
- `web/src/services/__tests__/knowledge-web-service.test.ts`
- `web/src/services/__tests__/knowledge-web-wire.test.ts`
- `web/src/hooks/__tests__/use-knowledge-web.test.tsx`
- `web/src/pages/dataset/knowledge-import/__tests__/web.test.tsx`
- `web/src/pages/dataset/knowledge-import/__tests__/web-fixture.ts`
- `web/src/pages/dataset/knowledge-catalogue/__tests__/web.test.tsx`

另新增本交付说明与客户端验证 JSON；未修改后端、全局请求、共享 UI、依赖／锁文件、根计划或 A 的报告。

## 实际验证

| 项目 | 结果 |
| --- | --- |
| 格式／lint | 改动的 27 个 Web 文件通过，0 警告 |
| 相关 Jest 回归 | **27 组／717 项全部通过**：本轮 6 组／211 项，既有 21 组／506 项 |
| 冻结向量 | 82 结构、12 typed-LP 编码、13 摘要、10 指纹；另验证 22 项客户端动作／origin／UTF-8 窗口语义 |
| 类型增量 | 原有 207 项诊断前后逐行一致，新增 0；全仓类型命令仍以 2 退出，不标记为全仓类型检查通过 |
| 生产构建 | 52.07 秒，1,058 个产物；保留已有大型 chunk 提示 |
| 保护摘要 | 后端报告中的 29 项全部保持原 SHA-256 |
| 真实网站／模型调用 | 本轮均为 0 |

本机忽略目录 `.local-data/stage2r5-frontend-20261026/` 保存执行前记录、失败与重试日志、最终 Jest JSON、类型前后输出、构建日志及产物摘要。先前失败未覆盖删除，最终判断只引用最后通过的验证。

## 交回边界

本轮只使用合成 HTTP 响应和 jsdom；未操作运行服务、真实网站、RSSHub 或模型，未部署、切换分支或合并 main。A 独立执行的 MySQL／S3／网络／故障／容量结果仍由其统一报告维护，本页不重复冒认。

主会话收回唯一写权后，继续部署并核验真实浏览器的文件选择、网络取得、确认／恢复、固定修订阅读和原快照下载。原有 207 项全仓类型诊断仍在。未启动多轮对抗性审查，人的验收决定保持 Pending。
