# LFen's Second Brain

本地文本、文档与项目代码知识库：结构化整理、可选词法／结构／向量检索、带来源的回答与代码复用；后续通过 Web 和 MCP 共用后端服务。当前POC仍为原生混合检索，可选策略待实施。

**本仓库是后续唯一开发入口，包含当前应用源码、品牌资产、方案和验证记录。** 当前实施主线为[v0.18完整构建链路、阶段0设计与双人计划](本地第二大脑知识库技术方案.md#35-v018-完整构建链路阶段0设计与双人开发计划)：宏观结构／原生映射→基础适配→来源接入→类型组织→可选索引与检索→MCP→人格记忆。阶段0设计已保存待验收，阶段1—6尚未实现。

本机承担A工作包：领域／来源／修订／节点／关系、策略／权限、解析／索引生产及运行装配；另一开发者承担B：Web、检索／证据消费、MCP、人格会话与记忆交互。两人共同验证一个域／来源／节点／页面最小链路后，按共享契约并行，每阶段接真实服务联调，共享文件有唯一负责人。本轮仅设计与推送，没有启动A／B业务代码。

v0.17的[知识域与可选Embedding决定](本地第二大脑知识库技术方案.md#34-v017-知识域与可选embedding决策记录)继续有效；v0.16旧S0—S7顺序由第35章替代，技术细节与任务映射保留。七项原话／台账在第32章；历史最小闭环见[首轮报告](POC-首轮原生闭环验证.json)，不能当完整质量／性能验收。此前审查拒绝保持各自范围。

## 从这里接续

1. 阅读 [会话接续说明](会话接续说明.md)，了解已确认决定、待确认事项和下一步边界。
2. 阅读 [完整技术方案 v0.18](本地第二大脑知识库技术方案.md)：第35章是唯一当前阶段顺序、宏观模型、15项RAGFlow适配、六组共享契约、双人任务及验收、本机范围和依赖；第33—34章保留前序设计与决定，第32章为七项原话，第31章为历史实测。第26章60题／30＋30及R0／A／B／C、第27章RSSHub和第28—30章运行／主仓库保持原范围；阶段0文档完成不等于业务实现或模型调用获准。
3. 需要讨论背景时，查阅 [会话副本](会话副本.md)。副本保存用户消息和助手回复，不含工具日志、内部推理和密钥。
4. 当前API实测结果见 [百炼验证记录](阶段0-百炼API接入验证.json)；[Moark验证记录](阶段0-API接入验证.json) 及 [旧额度截图](moark-stage0-trial-quota-94.png) 保留历史事实。

## 已选技术方向

| 能力 | 方案与边界 |
| --- | --- |
| 产品与 Web | RAGFlow 原生后端／Web；AICS 组件和实践按兼容性移植，四页目标保留，不另建 Next.js／Prisma 后端 |
| 数据库分工 | 原生 DocEngine 负责检索，业务元数据库负责管理；本机最小环境已运行 Elasticsearch＋MySQL；Qdrant 为条件备选 |
| 解析与索引 | 原生解析／Pipeline／Tokenizer／DocEngine；文件树／版本基础优先映射，补代码AST、确定性节点／关系和按文件／节点选择索引；源码能力须工程验证 |
| Embedding | 当前POC仍用百炼qwen3.7-text-embedding／1024维；目标按知识域、文件／节点选择，不要求全库向量化，代码优先词法与结构。无向量策略和零调用验证待实施；此前独立API的2560维结果保留 |
| 重排 | 当前POC Chat已绑定qwen3.7-text-rerank；单独API排序与原生基线报告分别留证，逐次调用审计及各入口实际路径仍待补，不能据页面观感判定未重排 |
| 当前 POC | 已完成小规模原生闭环与真实来源检索；完整R0/A/B/C、60题及30／30冻结未执行 |
| POC 生成 | RAGFlow已通过官方 deepseek-flash 生成真实代码回答及原生引用；usage／finish_reason透出仍待补 |
| 网站来源 | 原生 Feed／RSSHub 路由→RSS／Atom→原生 RSS 入口或保结构薄适配→标准文档；保留网址、版本、完整性、附件引用与增量规则；本轮仅AIHOT单条综述快照，未用RSSHub或测试增量 |
| Agent 接口 | 优先原生 MCP；模块／动作权限、临时 Key、到期和撤销缺口验证后补齐，尚未实现完整共享 |
| 代码检索与浏览 | 优先验证复用原生导航／Wiki／编译／来源回取；自研代码节点、静态关系、同版补全及复用包，组件侧分标签；首期不部署完整GraphRAG |
| 人格与记忆 | 复用系统表单、会话和Memory，适配人格版本、Chat读写绑定及作用域／生命周期；原生基础存在，产品闭环尚未实现 |

原NumPy／BM25S独立POC退出主线；CocoIndex、LlamaIndex、FastMCP等保留为研究参考。RAGFlow固定v1.0.0-rc2候选版；环境健康及原生最小闭环不能代替引用、关系、记忆、权限和完整质量／性能验收。现状纠正：5份来源为General／512-token目标，PPTX实际为presentation＋PageChunker；原生已有格式分流，定制AST与父子关系尚未落地。RSSHub输出与原生RSS缺口已按[固定源码](https://github.com/infiniflow/ragflow/blob/v1.0.0-rc2/internal/syncer/connector/rss.go)核查，接入与增量仍待执行。

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
- 用户已在实例配置模型并提供样本；下一步按第33章S0先冻结开发生效链、来源／回归／预算和原生能力映射，再推进各阶段。当前仅完成方案；具体UI、AST绑定、规模单位及记忆策略待相应阶段确认。完整评测沿用第26章，RSSHub仍单列；新预算不自动续用，历史验收及可选审查决定不扩展。
