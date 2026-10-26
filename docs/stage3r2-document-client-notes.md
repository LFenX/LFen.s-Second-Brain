# 阶段3-R2：文档结构客户端交付

2026-10-27，v0.62。Document Structure 0.1.0 客户端已实现并完成合成 Mock、组件及隔离构建验证；**实际同版浏览器／服务联调与人的工程验收尚未完成**。完整请求原话、逐条覆盖、保护件和验证摘要见 [验证记录](stage3r2-document-client-verification.json)。

## 版本与范围

- 后端接收基线：`2a2b6c0df650d98ebb87c20c27ab57dce16bd020`，分支 `codex/stage3r2-document`；未切换、拉取、重置或改写历史。最终完整交付 HEAD 由 Git 历史、同轮交接消息及本机 ignored `after.json` 标识。
- 消费正式 `knowledge-document-structure-v1.openapi.json`／examples／freeze。旧41件、候选三件和正式三件共47件，前后实际 SHA 均与后端交接一致。
- 28个 Web 文件和本片两份报告位于交接白名单内。旧图功能 `document-structure-service.ts`、后端、运行配置、依赖、共享UI和根中文计划均保持原字节。
- 本片只执行合成 Mock／隔离组件／构建，没有应用请求、浏览器登录、部署、Go／native运行、真实资料传输、模型或本地推理；没有启动审查。

## 使用与语义

知识目录新增懒加载的“文档结构”入口。选择已保存的 PDF／PPTX／DOCX 后，核对固定原件、当前 active 世代、基础及独立文档策略、实际 profile／producer 和 source head；处理上限8MiB，原件保存上限仍为64MiB。准备和确认分开；提交前重新核对，任一变化停止并取消旧确认。文档许可默认关闭，不要求开启代码结构，不启用模型或索引。

正式 DC01—DC12 均经 service→hook→局部页面，使用基础1.0.0及Document0.1.0双头。首次写201、重放200；未知结果只恢复首次原键。task／policy／cancel每主体共8个恢复槽、每槽16KiB，POST前存完整请求、目标、原键和规范摘要。损坏、满额或存储失败阻止新操作；401／404／412／503、超时及错误hint不能清槽。只有完整匹配的GET固定拒绝可供人明确放弃，之后重新准备、换新键并认可。

Task分别显示状态、phase和未知进度；取消回执不代表终止，stopping／cleaning保留非终态提示。前台轮询间隔不少于2秒，25秒结束本轮等待；隐藏、离页、终态停止。离页中止请求并丢弃迟到结果，保留原owner的恢复数据；同revision不接受矛盾内容，不回退旧Task版本。

固定表示和当前head独立缓存。按需展开直接子节点，每页最多20；不预拉全树。固定链接使用 `document_structure=1`、`document_source`、`document_revision`、`document_original_sha`、`document_representation` 和可选 `document_node`；身份缺失／重复／不符直接报错，不回退当前head。全部stale原因明确显示，归档不等同撤权，有权固定历史仍可读取。

React只显示转义文字及源结构。PDF页和PPTX slide保留物理位置；DOCX不提供Word页码。表格保留源行列及空cell，列表按源item及已知编号属性，媒体不加载URL或执行原文。内联媒体按绝对派生UTF-8零宽anchor插入；unsupported、空文字、无可提取文字与未OCR分别显示。技术证据默认收起。

heading默认读取自身 `own_reading_range`，只有明确选择才读整节聚合文字。`complete`相对节点聚合范围，自身范围读满即停止，即使complete=false也不自动继续。剩余大于64KiB时由服务端选择UTF-8安全窗；最后一窗显式使用已知own_end，不猜测字节端点。零长自身范围不发无前进读取。每窗核片段SHA，收齐对应范围后核own或aggregate SHA；没有全件字节时不声称已核全件文字。

原件回链先取得完整固定 `.5` 修订，核同domain／source／revision／SHA／长度／MIME，再复用既有完整SHA授权下载。不使用当前preview、匿名URL或C1根。

## 验证结果

| 项目 | 结果与界限 |
| --- | --- |
| 正式契约 | 101 schemas；140正例、148反例、28身份向量、10字串向量、7 raw JSON、11 query、8 semantic case消费通过；不是生产器或实际后端证明 |
| 本片定向 | 8套、446项通过；含12操作、三类恢复、精确准备与变化重认、轮询／取消、迟到／离页、own窗口、源表格／媒体及固定链接 |
| 相关旧功能回归 | 41套、1097项通过；新旧合计49套、1543项通过，无跳过的测试 |
| 格式与lint | 28个Web文件通过oxfmt检查，oxlint错误0、警告0 |
| 全量类型检查 | 当前实测207→207，新增0、消失0；命令退出2，原有错误未扩修，不能记作全局类型通过 |
| 生产构建 | 通过；Vite39.69秒，命令40.61秒；隔离dist共1062文件；构建期间1989个Web输入未改变 |
| 保护件 | 47／47实际SHA与交接相同 |
| 正常提交检查 | 保持默认hooks，在正常提交中执行；原始输出与最终clean HEAD见本机commit.log／after.json |

最终回归在 `web/` 目录按明确的 `*.test.ts/tsx` 文件清单运行，夹具保留为夹具。已有测试工具会对含特定名称的源码走Babel路径；本片使用可正常编译的语义命名，未修改测试配置、安装依赖或绕过检查。

## 本机产物与后续

目录：`D:/LFEN_project/dev_project/LFEN的第二大脑-知识库/.local-data/stage3r2-client-20261027/`。

| 产物 | SHA-256 |
| --- | --- |
| `dist/index.html` | `3cbf636d456252e23712b25af01092470074ef25478e46d2e338794b8e6d02ca` |
| `dist-manifest.json` | `9e8a26408288cfa975237b747d388affcd2b9ef2a62bf85288f24fe32490ee1d` |
| `web-input-manifest.json` | `15b41b1147aadd5cbc9c11105a1ba9d142e07a5d79205075ded986ac8c780d1a` |

目录另含测试JSON／日志、类型前后日志、格式／lint、47件保护前后快照及构建记录；提交后的 `after.json` 绑定正常提交的完整HEAD、源码／产物、检查结果和清洁状态。

A收回唯一写权后继续后端最小修复、真实同版浏览器／HTTP／故障与三格式容量、RSS、首屏验证。A已报告PPTX实际容量初轮native_timeout，本片没有放宽预算或更改契约，不能因客户端Mock通过称其已解决。R2人的工程决定、R1人的验收、真实资料退出和阶段3整体分别保持Pending；没有合并或推送main。
