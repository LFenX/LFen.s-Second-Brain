# 阶段3-R1 Structure 0.1.0 客户端交付

基于固定提交49fe2d47a340706331d34fb2f28c1d827f487f02，在codex/stage3r1-structure唯一写窗内完成白名单客户端。状态为客户端实施和Mock验证完成；整R1、阶段3及真实资料退出未验收。R6与R7纯合成沿用已有真人Approved。

## 已实现

- 正式57类型、15操作独立service→hook→局部界面；严格双头、201/200、范围、身份摘要、预算和no-store。结构ID64hex只走结构接口。
- 从.4固定manifest明确选择，再用.7当前active tuple取公开世代，核C1根/SHA/字节、路径与分类；不自动替换修订或期望。32文件、2MiB单件、16MiB合计和8条精确别名约束。
- 结构策略明确确认；task/policy/cancel三类原body/key/digest在POST前持久保存。丢失响应后原键GET，完整固定拒绝核对后才可显式放弃；未知结果、错误hint或单独rejection_id保留原槽。取消不丢原任务跟踪。
- 取消revision CAS、25秒等待及前台2秒轮询；隐藏/卸载/终态停止。默认确认false，输入、刷新及冲突复位，迟到旧准备丢弃。
- 固定view与当前head独立；显示空affected_sources下的全局stale原因。复用组织选择、成员列表、派生树、分栏及只读本地Monaco；有界原文与一跳关系按需分页。Markdown/HTML/图片URL仅为代码文本。
- 实际401隐藏当前私有可见数据/查询，物理owner恢复槽保留，迟到旧session响应不进入新界面。

仅局部修改catalogue入口、zh/en及ProjectPicker的export；新增文件均在约定目录。Go、配置/锁、README/中文主方案及41件保护字节不变。

## 验证

| 项目 | 实际结果 |
| --- | --- |
| 相关回归 | 39套、1082项通过，失败0 |
| 新增Structure | 6套、153项；包含39合法/37拒绝/8摘要静态例与向量 |
| 既有相关测试 | 33套、929项通过 |
| 格式与lint | 27个Web文件通过，lint 0错误/0告警 |
| 类型检查 | 新取得基线207→207，新增0；全项目仍退出2 |
| 生产构建 | Vite 41.75秒，命令总耗时43.496秒，退出0 |
| 保护摘要 | 41/41一致 |
| 模型/本地推理/外部抓取 | 本会话均0 |

初次回归发现默认目录预加载结构分栏依赖，已改为React.lazy/Suspense；最终整组通过。静态schema、Mock和隔离渲染不作为Go AST、租约、ABA、发布CAS或真实浏览器运行证明。34个冻结语义场景的消费/后端责任与实际测试路径逐项记于[验证报告](stage3r1-structure-client-verification.json)。

## 同版构建与回交

- dist绝对路径：`D:\LFEN_project\dev_project\LFEN的第二大脑-知识库\.local-data\stage3r1-client-20261026\dist`。现有web/dist未覆盖，未部署。
- index.html SHA256：`f80e55041291b9d7c7b2e8618a33118dfbec24d47303ecd4c7f333e45596cc66`。
- 1961个Web输入摘要清单SHA256：`a4b803ff56d964efa878547f05bf9ade1c9bbeb7751e66137aaf6e1cbc7ec97c`；逐文件清单在`.local-data/stage3r1-client-20261026/build-inputs.json`。27个改动源码与测试/构建前快照一致，其余Web输入与49fe2d4一致；最终提交只加入同一源码及本两份报告。
- 正常hooks提交、最终HEAD、已验证源码与提交对应及清洁状态见忽略目录`.local-data/stage3r1-client-20261026/after.json`和回交消息；回交后A接回tracked/Git写权，客户端停止源码修改。
- A使用本dist进行真实同版浏览器闭环并汇合独立后端/故障/性能证据。未操作A准备的合成域，未调用实际后端，未新增审查轮次或全面测试范围。
