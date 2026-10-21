# LFen.s-Second-Brain

本地文本与项目代码知识库：混合检索、重排、带来源的回答、代码关系增强与复用包，通过 Web 和 MCP 共用后端服务。

**当前为方案与阶段 0 API 验证仓库，尚无可启动的业务应用。** v0.4 技术方案、v0.5 开发计划、v0.7 免费体验 API 配置及基础验证已验收；阶段 0 整体尚未完成。

## 从这里接续

1. 阅读 [会话接续说明](会话接续说明.md)，了解已确认决定、待确认事项和下一步边界。
2. 阅读 [完整技术方案 v0.7](本地第二大脑知识库技术方案.md)：第 23 章为 AICS DataTable 示例，第 24 章为阶段任务及验收标准，第 25 章为当前阶段 0 状态。
3. 需要讨论背景时，查阅 [会话副本](会话副本.md)。副本保存用户消息和助手回复，不含工具日志、内部推理和密钥。
4. API 实测结果见 [阶段 0 验证记录](阶段0-API接入验证.json)；当时的免费额度截图见 [验证截图](moark-stage0-trial-quota-94.png)。

## 已选技术方向

| 能力 | 方案与边界 |
| --- | --- |
| Web 与业务层 | 参考 AICS 的 Next.js / React / TypeScript，复用可用组件和后端实践；知识库采用独立领域模型与权限 |
| 管理数据库 | Prisma + SQLite：模块、来源、任务、授权、代码关系及发布信息 |
| 检索数据库 | Qdrant：dense / sparse 索引与过滤，与管理数据库分工明确 |
| 增量与检索编排 | Python、CocoIndex、LlamaIndex；联合兼容性与契约尚待验证 |
| Embedding | Moark 免费体验 API，Qwen3-Embedding-8B，4,096 维 |
| Agent 接口 | FastMCP，多 Agent 独立身份、模块授权、临时且可撤销的密钥；尚未实现 |
| 代码检索 | 符号分块、前后端标签、有证据的代码关系扩展、依赖完整的复用包；首期不部署完整 GraphRAG |

## 换一台机器

```powershell
git clone https://github.com/LFenX/LFen.s-Second-Brain.git
Set-Location LFen.s-Second-Brain
if (-not (Test-Path -LiteralPath .env.local)) {
    Copy-Item -LiteralPath .env.example -Destination .env.local
}
```

在新机器的 `.env.local` 中自行填写 `KB_EMBEDDING_API_KEY`。真实 Key 仅保留在原机器被 Git 忽略的 `.env.local`，不会随克隆下载；通过自己的安全渠道迁移或从服务商控制台获取，不粘贴进会话、文档或 Git。

`.env.example` 是配置约定，当前没有加载器、应用入口或自动限额执行逻辑。先阅读方案并确认运行环境，不执行不存在的安装或启动脚本。免费额度与供应商规则以调用当时为准；本仓库记录的 100 次/日、验证后剩余 94 次是 2026-10-21 的历史观察。

AICS 原仓库未复制进本仓库。新机器继续 AICS 相关工作前，需准备该仓库并确认新路径、实际版本及未提交修改。旧文档中的原机器绝对路径保留为历史证据，不能直接当作新机器路径。

## 交付范围

- GitHub：[LFenX/LFen.s-Second-Brain](https://github.com/LFenX/LFen.s-Second-Brain)。
- 保留现有方案及验证记录；新增 README、会话副本、接续说明和不含密钥的历史额度截图。
- 未编写业务代码，未创建 AICS 快照或数据目录，未部署 Qdrant 或其他服务。
- 当前其余阶段 0 事项继续待用户确认，不能把本次仓库迁移视为开始业务开发的指令。
