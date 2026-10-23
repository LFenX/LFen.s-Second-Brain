# LFen's Second Brain

本地文本、文档与项目代码知识库：混合检索、重排、带来源的回答、代码关系增强与复用包；后续通过 Web 和 MCP 共用后端服务。

**本仓库是后续唯一开发入口，包含当前应用源码、品牌资产、方案和验证记录。** v0.14 已跑通三核心模型的原生最小闭环：6份样本、42块、5道真实来源检索题Hit@10为5/5。原生实际为1024维，完整60题／分块对照／正式性能及RSSHub增量仍未完成，详见[首轮报告](POC-首轮原生闭环验证.json)。

## 从这里接续

1. 阅读 [会话接续说明](会话接续说明.md)，了解已确认决定、待确认事项和下一步边界。
2. 阅读 [完整技术方案 v0.14](本地第二大脑知识库技术方案.md)：第 2—3 章为当前架构，第 26 章为原生 POC，第 27 章为 RSSHub 网站来源，第 28 章记录运行环境，第 29 章记录品牌改造，第 30 章记录主仓库同步与当前开发入口，第31章记录首轮POC实测及缺口。第 26.10 节集中列准备项，第 26.12 节区分复用／自研／待验证。第 23 章保留 DataTable 示例，第 25.8 节为已验收百炼验证。
3. 需要讨论背景时，查阅 [会话副本](会话副本.md)。副本保存用户消息和助手回复，不含工具日志、内部推理和密钥。
4. 当前API实测结果见 [百炼验证记录](阶段0-百炼API接入验证.json)；[Moark验证记录](阶段0-API接入验证.json) 及 [旧额度截图](moark-stage0-trial-quota-94.png) 保留历史事实。

## 已选技术方向

| 能力 | 方案与边界 |
| --- | --- |
| 产品与 Web | RAGFlow 原生后端／Web；AICS 组件和实践按兼容性移植，四页目标保留，不另建 Next.js／Prisma 后端 |
| 数据库分工 | 原生 DocEngine 负责检索，业务元数据库负责管理；本机最小环境已运行 Elasticsearch＋MySQL；Qdrant 为条件备选 |
| 解析与索引 | 原生解析／Pipeline、分块、模型 Provider 与任务；代码 AST 和原生未覆盖的结构／版本需求薄适配 |
| Embedding | 阿里云百炼 qwen3.7-text-embedding；独立API曾验证2560维，本轮原生实际1024维，维度参数透传待补 |
| 重排 | 阿里云百炼 qwen3.7-text-rerank；中文与代码合成候选排序已验证 |
| 当前 POC | 已完成小规模原生闭环与真实来源检索；完整R0/A/B/C、60题及30／30冻结未执行 |
| POC 生成 | RAGFlow已通过官方 deepseek-flash 生成真实代码回答及原生引用；usage／finish_reason透出仍待补 |
| 网站来源 | 原生 Feed／RSSHub 路由→RSS／Atom→原生 RSS 入口或保结构薄适配→标准文档；保留网址、版本、完整性、附件引用与增量规则；本轮仅AIHOT单条综述快照，未用RSSHub或测试增量 |
| Agent 接口 | 优先原生 MCP；模块／动作权限、临时 Key、到期和撤销缺口验证后补齐，尚未实现完整共享 |
| 代码检索 | 符号分块、前后端标签、有证据的代码关系扩展、依赖完整的复用包；首期不部署完整 GraphRAG |

原 NumPy／BM25S 独立 POC 退出主线；CocoIndex、LlamaIndex、FastMCP 等保留为研究参考。RAGFlow 已固定 v1.0.0-rc2 候选版并通过本机环境健康检查；三个模型的原生最小联合链路已验证；2560维参数透传、细粒度权限及完整评测尚未通过。已有单独API合成结果与本轮原生结果分别留证。RSSHub 的输出与原生 RSS 缺口已按[固定源码](https://github.com/infiniflow/ragflow/blob/v1.0.0-rc2/internal/syncer/connector/rss.go)核查。

## 本机运行入口

- Web：[LFen's Second Brain](http://127.0.0.1:18080)。仅本机监听。
- 源码：[LFenX/LFen.s-Second-Brain](https://github.com/LFenX/LFen.s-Second-Brain)，当前仓库根目录；上游固定基线 ac29b4b29928ca65d3efca19b6cfae3f86069b81。旧同级 ragflow 目录保留作回退。
- 本机使用本仓库 docker/docker-compose.poc.local.json 和 lfen-ragflow-poc 项目，沿用现有数据卷；六服务验证与切换结果见技术方案第 30 章及[同步报告](阶段0-主仓库同步验证.json)。
- [前四步环境验证](阶段0-RAGFlow环境启动验证.json) 为品牌改造前的固定快照。真实凭据与私有 Compose 均不入库；换机使用以下公开模板重新准备运行条件。

## 代码布局与上游来源

| 位置 | 用途 |
| --- | --- |
| cmd/、internal/ | Go 服务、解析、索引、检索、Agent 与原生后端 |
| web/ | React／TypeScript／Vite 前端及当前品牌资产 |
| docker/ | 上游运行配置、无秘密 POC 示例、本机忽略的私有配置 |
| 根目录中文 Markdown／验证 JSON | 已有方案、POC、会话接续与实际证据；原位置保留 |
| LICENSE | 上游 Apache-2.0 许可证，原文保留 |

来源为 [infiniflow/ragflow v1.0.0-rc2](https://github.com/infiniflow/ragflow/tree/v1.0.0-rc2)，为候选版。保留上游内部模块／协议名、依赖和原有开发脚本；具体迁移与业务开发另行推进。原有 Git 历史保留，上游以固定源码快照导入，不宣称继承全部上游提交历史。上游自托管CI、定时和发布任务仅对官方仓库启用；本项目尚未建设CI。

## 换一台机器

准备 Git、Node.js 22、Docker Desktop Linux Engine。原生 Go 后端暂用已固定摘要的官方同版本镜像；前端从本仓库构建。需要改 Go 后端时，按 AGENTS.md 的 build.sh／原生依赖流程构建对应二进制或镜像，当前挂载不会自动编译 Go 源码。

```powershell
git clone https://github.com/LFenX/LFen.s-Second-Brain.git
Set-Location LFen.s-Second-Brain
npm --prefix web ci --no-audit --no-fund
$env:NODE_OPTIONS='--max-old-space-size=8192'
$env:VITE_BUILD_SOURCEMAP='false'
$env:VITE_MINIFY='esbuild'
npm --prefix web run build
if (-not (Test-Path -LiteralPath 'docker/.env.poc.local')) {
    Copy-Item -LiteralPath 'docker/.env.poc.example' -Destination 'docker/.env.poc.local'
}
if (-not (Test-Path -LiteralPath '.env.local')) {
    Copy-Item -LiteralPath '.env.example' -Destination '.env.local'
}
```

在本机编辑 docker/.env.poc.local，将7个空值填写为独立随机秘密，设置受限文件权限；不要将值发到会话或提交。原有文件不得用模板覆盖。然后从仓库根运行公开的最小模板：

```powershell
docker compose -p lfen-ragflow-poc --env-file docker/.env.poc.local -f docker/docker-compose.poc.example.json up -d --wait --wait-timeout 300
```

模板将缺失的秘密作为错误处理，使用相对路径、固定镜像摘要、仅127.0.0.1端口及只读前端挂载。此机器已准备的私有JSON继续使用以下命令，不与上游默认.env混用：

```powershell
docker compose -p lfen-ragflow-poc -f docker/docker-compose.poc.local.json ps
docker compose -p lfen-ragflow-poc -f docker/docker-compose.poc.local.json stop
docker compose -p lfen-ragflow-poc -f docker/docker-compose.poc.local.json up -d --wait --wait-timeout 300
```

Git不包含本机数据库卷、日志、构建产物、依赖、真实Key或私有JSON；换机不等于迁移已有数据。当前默认管理员邮箱为admin@ragflow.io，口令来自你在本机设置的DEFAULT_SUPERUSER_PASSWORD。

在新机器的 `.env.local` 中填写 `KB_MODEL_WORKSPACE_ID`、两个模型的原生API根地址及 `KB_EMBEDDING_API_KEY` / `KB_RERANK_API_KEY`；两字段可使用同一把同时授权两个模型的百炼专用Key。北京业务空间API根地址格式见模板注释，具体值从该空间控制台获取。真实Key只保存在Git忽略的本机文件，不会随克隆下载；通过自己的安全渠道迁移，不粘贴进会话、文档或Git。

POC 另需在本机追加 KB_GENERATION_API_KEY；DeepSeek Key 与百炼 Key 独立，不需要百炼业务空间 ID。已有 .env.local 时只追加缺失配置，不能用模板覆盖已有凭据。模型、请求格式、推荐实验限额和资料准备以技术方案第 26 章为准；本轮不启用收费或真实来源调用。

`.env.example` 是模型与实验配置约定，RAGFlow 不会自动读取其中 KB_ 字段；当前没有 KB_ 加载器或自动限额执行逻辑。此前5次百炼验证仅为单独授权的合成实验；模板中的调用边界不等于原生框架已执行限额。配置 Provider 时还须保护实例元数据／备份里的凭据。本轮没有改模板或原有模型 Key；模型由用户在实例配置，首轮实测见第31章。前端依赖按锁文件安装，见第 29 章。Moark 历史体验额度不适用于百炼。

没有复制整个AICS仓库；本轮选定文件快照仅在被Git忽略的私有目录。新机器继续 AICS 相关工作前，需准备该仓库并确认新路径、实际版本及未提交修改。旧文档中的原机器绝对路径保留为历史证据，不能直接当作新机器路径。

## 交付范围

- GitHub：[LFenX/LFen.s-Second-Brain](https://github.com/LFenX/LFen.s-Second-Brain)。
- 同步原生源码、当前品牌、方案、验证记录及换机启动模板，保留原有会话副本、资料位置与Git历史。
- 已启动隔离的本机 POC 环境，并按用户后续指令修改产品品牌。v0.14已在私有本地冻结选定代码、导入真实PDF／PPTX及网站摘要并调用三核心模型；未部署Qdrant。
- 用户已在实例配置模型并提供本轮样本；下一步先处理第31章六项缺口，完整评测仍以第26章为准，网站标准来源见第27章。正式产品设施与知识库业务功能未建设。主仓库同步的实际提交／远端一致性见同步报告；既有前四步、品牌与本轮交付的 user_decision 分别保持原状态。
