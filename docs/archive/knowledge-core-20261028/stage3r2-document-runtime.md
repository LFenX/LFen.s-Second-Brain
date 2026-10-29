> 历史归档（2026-10-28）：仅保留旧阶段事实，不作为正式开发指引，不恢复旧批准、排程或写权。当前资料见[参考基线](../../knowledge-core-reference-authority.md)。

# 阶段3-R2：本机运行与性能重现

本轮沿用六个运行服务、已有卷和凭据。模型、本地推理、真实资料及在线网站取得均为0。最终计量与验收范围以同目录的阶段3-R2统一报告为准。

## 固定运行版本

| 项目 | 当前绑定 |
| --- | --- |
| Go源码 | `b4a702d0b5c5293a52dac85e7cfb39911690dfd2` |
| Go完整SHA-256 | `dfc10da5e020c83942f4f6c6f6622d8f4f214076476bb5186ddc10df8d647065` |
| Web index SHA-256 | `3cbf636d456252e23712b25af01092470074ef25478e46d2e338794b8e6d02ca`，1062个文件 |
| 应用Go GC | 持久运行环境 `GOGC=25`；原默认值100 |
| 解析child | 相同可执行文件；严格七字段最小环境，不继承parent的GOGC |
| 契约 | Document Structure 0.1.0；47件保护文件保持原摘要 |

源码与Web构建、输入、实际producer、API PID/start ticks和执行文件摘要分别核对，不能把契约合成示例中的摘要当作实际引擎版本。

## 构建与启动

继续使用现有Linux构建环境和 `build.sh` 设置原生库与CGO参数：

```bash
command -v ld.lld
ld.lld --version
bash build.sh --go
./bin/ragflow_server --api --help
./bin/ragflow_server --admin --help
./bin/ragflow_server --ingestor --help
./bin/ragflow_server --syncer --help
```

本轮实际链接器为 `/usr/local/bin/ld.lld`、LLD 22.1.8。构建退出0；四模式均打印Usage、退出1，符合当前参数解析行为。help检查不代替服务ready和实际接口验证。

Windows上的只读构建产物挂载到容器 `/run/lfen-server-artifact`。每次启动先复制、核完整摘要、设为只读可执行，再原子替换容器overlay中的程序，随后进入原有entrypoint：

```sh
umask 077
test -f /run/lfen-server-artifact
cp /run/lfen-server-artifact /ragflow/bin/ragflow_server.next
printf '%s  %s\n' 'dfc10da5e020c83942f4f6c6f6622d8f4f214076476bb5186ddc10df8d647065' '/ragflow/bin/ragflow_server.next' | sha256sum -c - >/dev/null
chmod 0555 /ragflow/bin/ragflow_server.next
mv -f /ragflow/bin/ragflow_server.next /ragflow/bin/ragflow_server
exec ./entrypoint.sh "$@"
```

该主体由现有应用的 `/bin/sh -ec` entrypoint执行，检查失败即停止。更新产物时同时更新固定摘要。此做法没有新增镜像、常驻服务或卷；child继续独立核对实际 `/proc/self/exe`。直接从Windows bind执行时，实测331,572,560字节的旧同版程序一次完整摘要读取约6.6秒，导致任务超线；不能通过删除完整性校验解决。

应用持久环境增加 `GOGC=25`，其余环境及所有凭据保持。Go标准库说明：GOGC控制新分配量与上轮存活堆之比，默认100；降低数值提高回收频率。本轮用额外CPU工作换取较低的驻留Go堆，同时重测任务、读取和RSS。它不是硬内存限制，不能控制所有原生分配，也不是测量前临时强制GC。child的七字段环境保持原值。

## 合成测量顺序

1. 核同版Go/Web、实际producer、运行环境、47份保护件、六服务与原有卷；禁止真实资料和模型调用。
2. 核固定三件容量输入，各8,126,464字节：PDF 128物理页、PPTX 128源slide、DOCX 1000段。原件SHA与源文字/padding计量见[固定输入及协议](../../stage3r2-input-readiness.md)。这些是近上限原件I/O样本，不是8MiB纯正文或最大复杂结构。
3. 新测量使用独立campaign和合成域，经正式原件保存接口准备。恢复未知结果沿用原body/原键；不得重建操作凑样本。
4. 对实际API PID/start ticks开启RSS观察，前20个无child样本均值作为该段baseline。每任务不重置baseline，不清缓存，不并行执行无关文档任务。
5. 三格式各5次串行新任务；首次POST必须201、key/task ID各不相同。计时从首次POST前到首次观察持久终态，包含接收、排队、处理、发布和不少于2秒的轮询等待。服务时间戳另列，不替代验收计时。
6. 每格式metadata、指定父级直属children首20项、根文字首个最多64KiB的UTF-8窗口各读取20次。列全部样本及最大值。fetch到完整body的时间不包含之后的客户端schema校验与证据落盘。
7. 等观察器自然结束，再封存原始日志完整SHA；未结束的摘要只能标provisional。各失败实验、原键和超限记录保留。
8. 同版Web分别测三格式可用结构/正文首屏，再独立验证取消、ABA、接管、重启、共享quota和原键；不把故障样本算作成功吞吐。

固定门槛保持：每格式5次任务最长≤10秒，三类各20读取最长≤1秒，parent与自有child合计RSS相对同段baseline增长≤256MiB，三格式浏览器首屏各≤3秒。

## 证据边界

- 本机测量工具、已绑定夹具和认证保存在ignored目录。公共源码、契约和报告可接续；另一环境须重新准备同摘要输入及本地凭据，不能把不同输入称为同一基准。
- API/存储为暖状态，第一试次不证明冷缓存；报告分别记录表示复用与实际child，未暴露的解析缓存命中保持未知。5次离散试次不建立生产p95或百万库吞吐结论。
- RSS名义间隔50ms另加观察开销，实际间隔、覆盖、父/子分量和增长均入报告。读取非原子、会漏短峰，RSS合计会重复计算共享页；不是PSS、cgroup计量或硬隔离。不得以历史VmHWM替代本轮增量。
- owned child须完整核PID/start ticks/boot、父身份、PGID、argv、exe和owner摘要。仅记录环境字段是否匹配及凭据/代理字段、socket/TCP数量；不记录token、环境值或地址。采样点未见网络不等于网络隔离。
- PDF只提取物理页文字，不识别空白/扫描页或OCR。DOCX按源结构，不能把heading/section当作Word物理页；媒体和外部引用只保存指向。
- Node编码单点对照的49节点逐字一致、8类拒绝保持、分配字节降低约21%，不等于应用RSS同比下降。此前RSS超限和任务超线均保留在统一报告的历史记录中。
