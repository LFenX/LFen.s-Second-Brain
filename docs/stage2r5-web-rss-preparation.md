# S2-R5 网页／RSS前置能力调查

日期：2026-10-26；基线main `38ce003`。此文是**本地只读调查和待定边界，不是新契约、实现或网络验证**。在R4待确认期间，主会话安排“阶段2知识消费与前端”会话接续调查，A核实下述关键源码事实；不把等待另一个会话当停工原因。只用已有合成准备记录，无业务请求、网站抓取或模型调用。

## 1. 已核实的复用点与缺口

| 范围 | 事实及源码 | R5处理方向，尚未实现 |
| --- | --- | --- |
| Feed解析 | [rss.go](../internal/syncer/connector/rss.go)已有RSS／Atom字段解析；原生条目键取id/link/title的MD5，键本身不含Feed范围 | 复用解析，不直接采用标题fallback或原生键作为正式来源身份；登记＋Feed范围＋稳定条目键绑定Document |
| 原文保真 | 原生`renderText`去HTML、压缩空白并输出.txt；[knowledge_source.go](../internal/syncer/connector/knowledge_source.go)私有离线适配另保留原Feed、条目HTML、网页HTML、附件引用和派生阅读 | 原快照、所选条目内容和阅读表示分开；Feed XML解析后的HTML不能称原始条目XML字节；当前适配未装配到路由／同步器 |
| 增量输入 | 私有适配指纹包含正文、URL、canonical、完整性、附件及发布时间／更新时间，排除采集时刻；author/categories在指纹后附入元信息 | 明确作者／标签等是否影响修订，不能概括为任何元信息变化都增版；同时间正文变化不能漏收 |
| 完整性 | summary为摘要，encoded／普通网页为unknown，Atom外部content为external_reference；读取Feed没有自动取得链接全文的已验行为 | 不凭200或content字段宣称full；显式补抓文章需独立范围／预算／结果；RSSHub只作可选Feed生产端 |
| canonical与链接 | 私有URL规则做HTTP(S)语法／相对解析、主机小写与去fragment；canonical缺失回退pageURL，当前不保留“未声明”与“同URL声明”的区别 | 原请求URL、最终URL、原始href、规范定位和canonical声明分别存；canonical不合并来源、不扩大抓取授权；base/xml:base和fragment语义待定 |
| 删除风险 | 原生RSS `OpenPrune`构造当前Feed完整集合；[prune_runner.go](../internal/syncer/prune_runner.go)随后`DeleteStale` | R5必须隔离这条删除路径；窗口缺项、空Feed、过滤缩小、404／超时／解析失败均不删除旧知识，不仅依赖UI默认不勾选 |
| 取得安全 | [upload.go](../internal/utility/upload.go)每跳重新校验URL、钉选已验IP、禁环境代理，有重定向和字节上限，返回headers／最终URL；正文使用有界ReadAll | 复用安全原语并补当前域／本次抓取授权、整体预算和来源证据；原生RSS丢弃headers／最终URL，不是已实现R5完整抓取链，也不是R4流式大文件能力 |
| 消费安全 | [SafeImg](../web/src/components/safe-img.tsx)过滤危险scheme和事件属性，但合法http(s)仍是真img；[数据源表单](../web/src/pages/user-setting/data-source/constant/index.tsx)实际默认sync_deleted_files=false | 阅读页单独禁止自动远程图片／iframe／附件取得；sanitize不等于无外连。普通数据源的auto_parse／rebuild／prune不能直接接知识域 |

既有[内部准备说明](stage2a-source-preparation.md)第4节、相邻Go合成测试和[历史报告](stage2a-c2-verification.json)是此前NotWired证据。本批没有重跑，不报新通过数；历史千／万目录采集性能也不是网站抓取性能。

## 2. 下一候选应明确的边界

1. **层次**：抓取生产端→不可变原网页／Feed快照→条目标准化→原生来源／修订→固定阅读表示。正式身份沿用领域模型，不把`prep:`临时键交给客户端。
2. **范围**：先单网页／单Feed显式观察、明确选入与确认；RSSHub、AIHOT或其他真实地址需明确访问授权与实测。登录站、JS渲染、多站自动爬取、历史回溯和周期调度不默认开启。
3. **身份**：GUID／Atom ID在Feed范围内有效，链接fallback标明弱身份，只有标题拒绝。Feed改址／重定向／ID冲突规则须固定；canonical不改源。原文摘要做完整性／修订，不跨域／登记去重来源。
4. **版本**：FetchedAt、条目重排、无关条目变更不自动增加本条版本；内容／指定元信息的变化明确增版。正文原字节、阅读Markdown各有摘要和表示版本；转换器升级不静默重写历史阅读。
5. **恢复**：确认绑定固定观察清单、选择及预期头；原键查询不触发fetch或保存。变化中的Feed顺序和原生锚点不当固定任务快照；锚点丢失／窗口滚动只触发明确重观察或约定恢复，不删旧项、不自动换键。
6. **网络／解析预算**：声明页面／Feed／条目字节、条目数、编码／压缩／XML规则、总重试时限和每跳范围；304／ETag／Last-Modified是否支持须明文，不能把原站404与本系统权限404混为一类。
7. **阅读／附件**：代码缩进、多行文本及HTML表格结构保留；原HTML不执行，远程资源只作引用且不自加载。现有附件提取不是任意网页资源发现器，不承诺srcset、CSS和普通PDF链接全覆盖。
8. **契约隔离**：不塞旧inline_text或改C1 2MiB／root约束、不自动加入旧0.4清单，也不把R4未实现的0.5当现成抓取服务。所有新接口先形成具体候选；归档／GC归R6，索引和完整阅读处理按后续阶段。

## 3. 最小流程和验收输入

拟定流程：已有域／分组→登记来源范围→显式一次有界观察→查看固定原／最终URL、稳定键、SHA、时间、完整性、代码／表格与附件指向→明确选入／分类／认可→按原确认键保存→回执与固定修订。刷新产生新观察，不自动确认；窗口缺项显示“本次未观察到，旧版保留”。这只是流程建议，未创建共享DTO或HTTP操作。

后续合成验收至少包括：

- 网页／RSS2／Atom两版原字节与SHA；正文HTML、Feed XML及阅读表示区分。
- 同GUID跨Feed／登记不合并；同条变化身份稳定、重复／冲突明确；只有标题拒绝；canonical不合并。
- summary／unknown／external_reference不冒充全文；Atom text／html／xhtml及外部内容引用不自抓。
- 代码、合并单元格、相对链接和附件指向保留；脚本／事件／iframe不执行、远程图片零自加载。
- A+B→只剩B、空／错误Feed、过滤缩小、404／超时都保留A；原生prune不可误接。
- 同／倒退／缺更新时间而正文变化、条目重排、锚点丢失、观察过期、丢响应／重启／并发：无漏项和重复发布，失败不推进未提交头或水位。
- 后续获授权时仅用合成HTTP夹具验证每跳URL／IP／大小／压缩／超时和总重试预算；不以本次静态调查宣称SSRF或网络性能已测。
- 当前权限、固定历史、旧契约及经批准的原件边界回归；实际模型请求0、旧POC／用量不变。

R4仍是当前实施顺序首项；本调查不要求等待R5来阻塞R4。未启动多轮审查，不声称候选冻结或工程验收。
