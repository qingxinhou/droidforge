# Changelog

本项目版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。
格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [1.8.4] - 2026-11-10

把一份真机运行日志逐条核对了一遍。问题集中在「界面上看不出来但操作已经失败」这类
静默失效：按钮能点、提示是绿的，实际上什么都没改成。

### 修复：MIUI / HyperOS 上系统调校整页白点

`settings put` 在这些 ROM 上被 `SecurityException` 拒绝（shell 角色被收回了
`WRITE_SETTINGS` / `WRITE_SECURE_SETTINGS`），但旧界面按钮照常可点，用户只能一次次
点、一次次看 Java 堆栈。现在进页面就探测三个 settings 命名空间的写权限：

- 权限不在时禁用对应控件、顶部横幅说明原因（root 才能改），不再让按钮假活；
- 沉浸模式 / Wi-Fi 检测写 global，其余写 secure，分别拦截；
- `resetAll` 按权限拆分执行，只重置改得了的项；
- `settings` 被拒时吐出的 17 行堆栈折叠成一句话，不再刷屏。

### 修复：启动应用失败时给不出能行动的原因

旧实现只有「未找到可启动的 Activity」一句。但真机上至少三种情况都会走到这里，
处理办法完全不同：没装、被 `disable-user` 禁用了、装了但没有 MAIN/LAUNCHER 入口
（WPS lite 这类插件包）。现在按「resolve-activity 解析组件 → am start → monkey 兜底
→ 精确报错」走，三种情况分开报。另外 `pm list packages` 是模糊过滤，包名匹配改为
逐行精确比较，不再把 `com.foobar` 当成 `com.foo` 装了。

### 修复：备份助手的三处假数

- **「共 1 项」永远是 1**：`TotalFiles` 被当成项数用，而文件类别的 `Files` 恒 0。
  扫描脚本加了 `@@@COUNTS` 段（`find <paths> -type f | wc -l`），现在显示
  「N 项可备份（M 个文件）」。
- **短信 0 条报成 1 条**：`content query` 查不到时往 stdout 打 `No result found.`，
  `wc -l` 把这句话也算了一行。改为 `grep -c '^Row:'`，只数真正的记录行。
- **解析逻辑不可测**：`parseScan` 从 `Scan` 里拆成纯函数，用真机输出做回归。

### 修复：同一条错误在日志里出现两遍

后端绑定方法失败时 `a.log("err", ...)` 记一份，前端 `run()` 又 push 一份，
真机日志里成对出现、每段 17 行堆栈。前端按「同文本 + 1.2 秒窗口」去重。

### 新增

- `tools/logcheck`：只读验证工具，跑一遍探测确认上述修复在真机上生效。

## [1.8.3] - 2026-11-10

接真机把 fastboot 链路走了一遍，包括进 bootloader 和 fastbootd。挖出的问题集中在
「模式切换后整个页面不可用」这类只在真机上出现的事。

### 修复：设备在 fastboot 时刷机页分区下拉是空的

刷 `boot` 本来就是进 fastboot 做的事，但 fastboot 模式下 `adb devices` 为空，
刷机页和分区页却都只走 `GetPartitions`（纯 adb 实现）——**用户连着设备，却选不出
目标分区，整个功能等于不可用**。

- 新增 `FastbootPartitions(serial)` / `FastbootVars(serial)`，从 `fastboot getvar all`
  还原分区表；刷机页与分区页按当前模式自动切换数据源，并标注「来自 bootloader /
  来自 fastbootd / 来自 ADB」。
- 进页面即自动读取，设备从系统重启进 fastboot（或反之）时自动重读。

### 修复：GetVar 的三处解析问题

- **冒号后无空格时值被丢弃**：bootloader 给 `partition-size:boot_a: 4000000`，
  fastbootd 给 `partition-size:boot_a:0x4000000`。旧解析优先找 `": "`，找不到就
  回退并把值置空——真机在 fastbootd 下因此解析出 **0 个分区**。改为按最后一个冒号
  切分并 trim。
- **容量前缀不一致**：bootloader 是裸十六进制（`73ff7f8000`），fastbootd 带 `0x`
  （`0x73FF7F8000`），两种都认。
- **静默失败**：`getvar` 一个不存在的变量时，bootloader 是「键回显、值为空、退出码 0」，
  fastbootd 是 `FAILED (remote: 'Could not open partition')`——两种退出码都是 0。
  现在前者返回空值（语义就是「设备没给」），后者返回带原因的错误。

### 修复：刷入失败的错误信息

旧实现把 `fastboot flash` 的整段输出（含 `Sending...`、`Warning: skip copying...`）
塞进 error，UI 上真正的失败原因被埋在中间。现在只提取 `FAILED` 那句。

### 修复：槽位叠加

分区名已带 `_a` / `_b`（bootloader 返回的名字都带后缀）时再选槽位会拼成 `boot_a_a`，
刷入必失败。现在检测到后缀就禁用槽位选择并说明原因。

### 新增：从 fastbootd 读全量分区容量

Android 10+ 在系统内封死了块设备容量的所有读法（`/proc/partitions`、sysfs、
`blockdev`），无 root 时分区页大量显示「—」。fastbootd 的 `getvar all` 一次能给出
全部分区的真实容量（真机实测 108 个，含 `super` 内的 `system_a` / `vendor_a` 等），
并用 `is-logical` 逐分区标注逻辑/物理。分区页在 fastbootd 下直接可用。

### 其他

- 新增 `internal/fastboot` 测试 10 项，其中 4 项直接跑真机抓下来的原始输出
  （`testdata/getvar_all_bootloader.txt`、`getvar_all_fastbootd.txt`、`flash_failed.txt`）。
- 真机确认：非活动槽位的逻辑分区容量确实报 `0x0`（不是读不到），按「真实为 0」处理；
  `sda` / `sdb` 只有 `partition-type` 没有容量，记为 -1。
- 新增 `tools/fbcheck`：在真机 fastboot 模式下自检 fastboot 包。

## [1.8.2] - 2026-11-10

第一次接真机跑（Redmi K60 至尊版 / Android 13 / MT6896 / A-B 槽位 / BL 已解锁）。
一轮下来挖出五个「单测全绿、真机全废」的问题，全部修掉。EDL 仍未实测，手上没有
9008 环境。

### 修复：备份（v1.8.0 起在 Windows 上完全不可用）

- **裸 tar 流经 adb 传递必坏**，两个原因叠加：Windows 版 adb 会把设备端输出的
  `\n` 改写成 `\r\n`，tar 的 512 字节头随之错位；同时 adb 会把设备端 stderr
  合并进 stdout，tar 对绝对路径打的 `removing leading '/'` 警告会直接插进数据流。
  实测设备端 `tar | wc -c` 是 3072 字节，管道收到 3111 字节 —— 备份 100% 失败。
  现在改为在设备端补 `2>/dev/null` 并 `| base64`，让管道里只跑 ASCII；
  Go 的 base64 解码器本身忽略换行，无需额外剥 `\r`。
  实机验证：15 个文件落盘，md5 与设备端完全一致。

### 修复：分区表

- **容量全是 0**：Android 10+ 起 shell 读 `/proc/partitions`、
  `/sys/class/block/*/size`、`blockdev` 一律 `Permission denied`。
  改为用 `df -k` 补已挂载分区，其余如实返回 `-1`（未知），界面显示「—」
  并提示「需 root 或 Fastboot」，不再把「读不到」伪装成 0。
- **挂载点一个都匹配不上**：by-name 给 `/dev/block/sdc43`，mapper 给分区名，
  `mount` 给 `/dev/block/dm-9`，三套命名互不相干。原先直接用字符串相等比较，
  55 个 dm 设备命中 0 个。现在解析 mapper 的符号链接建立 dm 编号映射，并处理
  verity 层（`system_a` 指向数据层 dm-3，实际挂载的是 `system-verity` 的 dm-9）。
  实机命中数 0 → 16。
- **`by-uuid` 被当成一条分区**：`ls /dev/block/mapper` 会带出这个目录，
  现按行首文件类型过滤。
- **APEX 混进分区表**：`com.android.*` 的 device-mapper 并非可刷分区，
  已过滤。实机条目 129 → 105，其中 94 条物理分区、12 条逻辑分区。

### 修复：应用列表

- **Android 14+ 的包名全部解析错**：安装路径形如
  `/data/app/~~AbC==/com.foo-XyZ==/base.apk` 自身带等号，原先用 `strings.Cut`
  从第一个等号切分，路径被截断、包名里塞进半截路径，后续按包名的操作全部失效。
  改为取最后一个等号，并抽成 `splitPkgLine` 加 8 组用例锁住两种布局。

### 修复：电池

- **`0 mA` / `0.00 W` 是假读数**：`/sys/class/power_supply/*/current_now`
  在 Android 10+ 同样被拒，`currentUa` 恒为 0；前端写的 `|| '—'` 对字符串 "0"
  不生效，于是把「读不到」显示成了真实测量值。现在显式判断可用性，
  无读数时显示「—」并说明需 root，功率曲线一并隐藏。

### 修复：设备信息

- **SoC 显示成 `mt6895`**：MTK 的 `ro.board.platform` 是兼容命名，
  天玑 9200+ 会报 mt6895。改为优先 `ro.soc.model`（`MT6896Z/CZA`），取不到再回退。

### 新增

- `tools/devcheck`：接上真机后一把跑完设备信息、分区、CPU、电池、应用列表与
  备份的诊断程序，用来在真机上验证解析结果。不参与打包。

### 文档

- 已知限制补四条：通讯录 / 短信导出需要 root（Android 11+ 直接抛
  `SecurityException`）、分区容量多半读不到、实时电流需 root、
  备份为什么走 base64。

## [1.8.1] - 2026-11-10

修一轮验收时挖出来的问题。全部是「代码能跑、测试能过，但真机上会坏」的类型。

### 修复：EDL 9008

- **设备枚举永远失败**：`SP_DEVICE_INTERFACE_DETAIL_DATA.cbSize` 被填成 12，
  64 位下应为 8。填错时 SetupDiGetDeviceInterfaceDetailW 直接返回参数无效，
  9008 设备一个都列不出来。
- **同步读写却带 FILE_FLAG_OVERLAPPED**：句柄带了这个标志又给 WinUsb_ReadPipe
  传 NULL OVERLAPPED，轻则报错重则「提前报告完成」，大块传输静默截断。
  改为同步句柄 + 管道超时策略。
- **没设 MAXIMUM_TRANSFER_SIZE**：WinUSB 默认上限小于 1 MB，按 1 MB 切段写入
  会被驱动以参数错误拒掉。现在显式抬到与 MaxPayload 一致。
- 管道策略常量名与值不符（`0x05` 是 ALLOW_PARTIAL_READS，不是 IGNORE_SHORT_PACKETS），
  已改名并注释清楚。
- **控制报文没有超时**：WinUSB 读超时返回 `(0, nil)`，而 `io.ReadFull` 把
  「0 字节 + nil」当正常情况继续等，设备不响应就永久卡死、界面既无进度也无错误。
  改为带空闲超时的读，控制报文 15 秒、数据传输 30 秒。
- **非扇区对齐的镜像写不完**：最后一段短读直接报 UnexpectedEOF。现在用 0 补齐；
  复用的缓冲区尾部显式清零，避免把上一段的残留写进分区末尾。
- **忽略设备协商的传输上限**：configure 响应里设备可以把
  `MaxPayloadSizeToTargetInBytes` 降到更小，之前照旧按 1 MB 切段。现在按协商值走。
- 读分区表时，传输层失败不再换扇区大小重试（管道里可能残留半截数据，
  重试会读到错位内容、把「设备没响应」伪装成「CRC 不对」）。

### 修复：下载器

- **重试导致进度重复计数**：重试从原始偏移重新请求，回调却继续累加，
  `Done` 会涨到超过分段长度。续传时判定「已完成」，文件中间留一段空洞而界面显示成功。
  改为从已写入的下一字节继续。
- **服务端多给数据会写坏相邻分段**：原来是先落盘再检查越界。改为先按区间裁掉多余字节。
- 分段下载改用复用的 Transport，并加 30 秒响应头超时；之前用
  `http.DefaultClient` 没有任何超时，连接卡住就只能等用户点取消。
- 续传前比对 ETag：同一链接换了文件时不再接着下（前半段旧文件、后半段新文件）。

### 修复：备份助手

- **adb 进程泄漏**：解包中途失败提前返回时没有回收进程，adb 堵在写 stdout 上不退出，
  设备连接一直被占着。现在关闭流会自动结束进程。
- **content query 解析错位**：原来用通用的 `\w+=` 找字段，短信正文里出现
  「订单号=A123」这类片段就会被当成新字段，后面所有列整体错位。改为只认投影里的字段名。

### 修复：AVB / vbmeta

- **hash 描述符字段顺序读错**：`flags` 紧跟 `image_size`，随后才是三个长度与
  `hash_algorithm`。按旧顺序读出来的摘要和算法全是乱码。现在两种历史布局都试，
  按自洽性（长度加起来是否正好等于尾部长度、算法名是否可识别）取分高的那个。
- **chain 描述符预留字段长度错**：`reserved` 是 64 字节不是 60，定长部分应为 80，
  读 76 会让分区名整体错位。

### 修复：超级分区

- **输入分区是 sparse 镜像时会原样拼进 super**：元数据按物理文件长度登记，
  内容是 sparse 头而不是文件系统，刷进去开不了机。现在按展开后的长度登记并就地展开。

### 修复：线刷脚本

- **动态分区之后仍在 fastbootd 里刷物理分区**：未登记在排序表里的分区排到了
  super/system 之后，`fastboot flash` 必然失败。改为先按「物理分区在前、
  动态分区在后」排序，脚本里也补上切回 bootloader 的逻辑。
- 切换 fastbootd / bootloader 后补 `if errorlevel 1 goto :fail`。
- 路径含 `&`、括号、`!` 时也加引号（`Program Files (x86)` 这类目录很常见）。

### 测试

- 新增：非对齐镜像写入补零、configure 协商上限、重试不重复计数、
  越界响应钳制、AVB hash 双布局、sparse 分区合成、动态分区排序、
  正文含等号时的 content 解析。

## [1.8.0] - 2026-11-10

补齐紫罗兰工具箱的后两批：设备助手（投屏 / 备份 / 模块）与进阶能力（AVB / ARB / EDL 9008）。

### 新增

- **投屏（新页面）**：调用本机 scrcpy 开独立窗口，可调码率、长边上限、帧率，
  支持关屏投屏、保持常亮、不转发音频、窗口置顶，以及同时录制到 mp4。
  进程生命周期由主程序管，关掉 scrcpy 窗口等同于停止投屏，状态会同步回界面。
- **备份助手（新页面）**：按类别把照片、文档、音乐拉到本机。走
  `adb exec-out tar -cf -` 流式传输，一个连接传完整棵目录，解包时逐文件判断
  「已存在且大小一致就跳过」，天然支持反复增量备份。通讯录与短信走
  content provider 查询导出为带 BOM 的 CSV，Excel 直接可读。
- **模块管理（新页面）**：批量装 Magisk / KernelSU / APatch 模块。解析 zip 里的
  `module.prop`（兼容内容套在一级子目录里的包），自动识别设备上的模块管理器
  并选用对应安装命令，逐个推送、安装、清理临时文件。
- **引导校验（新页面）**：解析 vbmeta 的签名算法、`rollback_index`、flags 与全部
  描述符；带 `AVBf` footer 的 boot / system 镜像会自动定位 vbmeta 位置。
  同时对 xbl / abl 这类引导镜像做 anti-rollback 标记扫描，给出候选版本与置信度，
  并提供两侧版本对比来判断降级是否会触发熔断。
- **EDL 9008（新页面）**：Sahara 握手 + Firehose 会话，可加载 loader、读多 LUN 的
  GPT 分区表、按分区读取 / 写入 / 擦除、发送重启。写入按 `MaxPayload` 切段，
  逐段 ACK 确认，进度实时回报。
- **OFP 识别**：欧加系 `.ofp` 刷机包按 zip 容器识别，能直接列出内部镜像；
  OPS 因没有公开规范，明确拒绝而不是猜。

### 实现

- `internal/avb`：按 AOSP `libavb` 的头文件布局实现 vbmeta 解析。描述符 tag 的
  编码在不同年代工具链出现过两种布局，解析时两种都认；CRC 与 block 长度的
  一致性只作提示，不阻断解析。
- `internal/arb`：分块扫描（块间留重叠避免标记被切断），ASCII 与 UTF-16LE 两种
  编码都找，命中后在标记附近取第一个落在 `1..0xFFFF` 的 4 字节小端整数。
  置信度按「多个标记是否指向同一个值」判定。
- `internal/edl`：协议层与传输层分离。Sahara 与 Firehose 的报文编解码是纯函数，
  用 `net.Pipe` 模拟设备把整个握手与读写时序跑通；传输层在 Windows 上直接调
  `setupapi.dll` + `winusb.dll`（`syscall` + `LazyDLL`，无 cgo），非 Windows 返回
  明确的不支持提示。
- `internal/edl` 的 GPT 解析支持 4096 与 512 两种扇区大小，CRC 不符时照常返回结构
  并在备注里说明。
- `internal/backup`：解包时对每个路径做上跳与盘符过滤，tar 里的绝对路径会被
  规整成相对路径，避免写到备份目录之外。
- `internal/magisk`：文件名进 shell 命令前先白名单过滤，避免模块名里的特殊字符
  破坏命令。
- `internal/runner` 新增 `Stream`，返回原始字节通道，供 tar 流与 Firehose 使用。

### 测试

- 新增 avb 7 项、arb 6 项、backup 7 项、magisk 9 项、edl 18 项、screen 3 项。
- Sahara 用例完整模拟设备侧状态机，逐字节比对收到的 loader；
  Firehose 用例覆盖 ACK / NAK、多段写入、分区读取与上下文取消。
- GPT 用例自造 header 与表项并计算 CRC，覆盖 CRC 不符与签名缺失。

## [1.7.0] - 2026-11-10

刷机包深加工：super 动态分区可拆可合，线刷脚本可一键生成，整包下载改为多线程断点续传。

### 新增

- **超级分区（新页面）**：解析 `super.img` 里的 `lp_metadata`，列出全部逻辑分区的
  名称、大小、所属组、extent 数量与属性（只读 / 带槽位），按需导出为独立 `.img`。
  直接补上了此前「super 内部逻辑分区暂未拆解」的限制，`system.img` / `vendor.img`
  不再只能从 payload 或线刷包里找。
- **散包合成 super**：扫描目录里的分区镜像，按 `super_def` / `dynamic_partitions_info.txt`
  自动归类到正确的分区组（没有定义文件时统一放进 `default` 组），合成带完整
  geometry + 双槽位 metadata + 备份的 `super.img`，可直接 `fastboot flash super`。
- **线刷脚本生成**：把镜像目录渲染成 `flash-all.bat` / `flash-all.sh` / `flash-all.txt`。
  bat 每步都带 `if errorlevel` 中断，动态分区处自动插入 `fastboot reboot fastboot` 与等待，
  可指定槽位后缀、末尾重启、刷前 `fastboot -w`。
- **固件下载（新页面）**：粘贴 OTA / 线刷包直链即可下载，支持多线程分段与断点续传。
  进度每 2 秒落盘到 `.dpart` 状态文件，中途关掉程序再点一次会从断点继续；
  服务端不支持 `Range` 时自动降级为单流下载并明确提示。
- **链接失效可读化**：403 / 404 / 410 直接提示「链接已过期，请重新获取」，不再抛裸状态码。

### 实现

- `internal/lp`：按 AOSP `liblp` 的 `metadata_format.h` 实现 `lp_metadata` 读写。
  顺序尝试主 metadata、备 metadata 与冗余槽位；geometry 与表区都做 SHA-256 校验，
  **校验和不符只告警不拒绝解析**（第三方工具改过的镜像很常见）。
- `internal/lp` 自带 sparse 读取器，把 super.img 的 sparse 块映射成逻辑地址，
  geometry 与分区数据都能按逻辑偏移直接读，不需要先解成 raw 占掉几个 GB。
- `internal/download`：先发一次 `Range: bytes=0-0` 探测大小与分段能力，
  再把文件切成若干段并发写入；`.dpart` 里记录每段完成量，续传时只补没下完的部分。
  分段进度用互斥锁保护，落盘协程与下载协程不会读到半更新的状态。
- `internal/flashscript`：渲染逻辑按目标 shell 分支，bat 用 CRLF + `chcp 65001`，
  sh 用 `set -e`；分区名只在已知带槽位的白名单后追加 `_a` / `_b`
  （`super` / `userdata` / `modem` 等保持原名）。
- 合成前会校验分区名重复、文件存在性、指定大小是否小于数据总量，
  并保证每个分区起点落在 1 MB 对齐处。

### 兼容

- 新增页面挂在「刷机」分组：固件下载 → 刷机包提取 → 超级分区 → 刷入镜像 → 线刷整包 → 分区管理，
  顺序即推荐的工作流顺序。
- 命令面板可用 `super` / `动态分区` / `断点续传` / `合并` 等关键词检索到新页面。

---

## [1.6.0] - 2026-11-10

重新整理功能分类，默认主题改为跟随系统。

### 变更

- **导航按「操作性质与风险」重新分组**，替代原来按技术栈划分的 设备 / 刷机 / 系统 / 工具：
  - **设备**：应用管理、文件传输、ADB 终端、重启与模式 —— 连着 ADB 就能做，随时可中止
  - **调校**：显示控制、界面定制、电池管理、性能监控 —— 只改系统参数，能改回去
  - **刷机**：刷机包提取、刷入镜像、线刷整包、分区管理 —— 会写分区，做错不可逆
  - **进阶**：Root 方案、控机脚本 —— 风险最高
  
  越往下越危险，侧栏顺序即风险顺序；「设置」从功能分组里移到底部单独一区。
- **新增命令面板（Ctrl / Cmd + K）**：按名称、说明、路径与英文别名检索全部功能，
  例如输入 `payload` 找到「刷机包提取」、输入 `状态栏` 找到「界面定制」。
  当前设备状态下用不了的功能会标注「需 ADB」「需 Fastboot」，高危页面标注「有风险」。
- **仪表盘新增「功能总览」**：四列对应四个分组，一眼看清全部入口，不用靠记侧栏位置。
- **默认主题改为跟随系统**：首次启动不再强制深色。手动选过主题的用户不受影响。

### 实现

- 导航抽成唯一数据源 `frontend/src/nav.ts`（分组、说明、图标、所需设备状态、风险标记、搜索别名），
  侧栏、命令面板、仪表盘总览、设置页的「启动页」下拉全部从这一处读取，加页面只改一个文件。
- 侧栏条目高度收到 34px、分组标题配细分隔线，15 个入口在 860 高的窗口里不用滚动。

### 兼容

- 配置新增 `themeExplicit` 字段。1.5.x 里默认值被写死成 `dark` 且没有该标记，
  升级时这类「从没主动选过主题」的配置会回到跟随系统；用户自己点过主题的一律保留原值。

---

## [1.5.1] - 2026-11-10

深色 / 浅色主题切换。

### 新增

- **三档主题**：深色、浅色、跟随系统。顶栏右侧有一键切换按钮，设置页「界面 → 主题」为三选一，
  选择即时生效并写入配置。
- **「跟随系统」**监听 `prefers-color-scheme`，系统在日间 / 夜间模式之间切换时界面自动跟随。
- **首帧防闪**：`index.html` 内联脚本在样式表加载前先按本地缓存套上主题，
  避免浅色用户启动时先闪一下深色背景。

### 实现

- 全部语义色（`ink-*` / `slate-*` / `accent` / `ok` / `warn` / `danger` / `muted` / `shadow-panel`）
  改为 CSS 变量 + Tailwind 的 `rgb(var(--x) / <alpha-value>)`，深浅两套值集中在 `src/style.css`。
  **组件里没有新增或修改任何颜色类**，换主题只切换 `<html>` 上的一个 class。
- 浅色不是简单反色：`slate` 文字层级整体反转（深色下 100 最亮，浅色下 100 最深），
  强调色改用更深的蓝 / 绿 / 橙 / 红保证白底对比度，弹窗遮罩从纯黑改为淡蓝灰避免整屏发脏。

### 修复

- 日志时间戳在浅色下几乎不可见 —— 由 `slate-700` 调整为 `slate-600`，并重新标定两档弱化文字的取值。
- 按钮文字在窄容器里会被压成竖排（如设置页的「选择」）—— `.btn` 基类补 `whitespace-nowrap`。
- 设置页的主题选中态在顶栏切换后不同步 —— 监听全局配置变化回写本地副本。

---

## [1.5.0] - 2026-11-10

补齐风险告知与免责链条。刷机、Root 都是不可撤销的固件级操作，软件必须把「可能失去什么」讲清楚，
并让用户显式确认，而不是靠一句藏在 README 里的话。

### 新增

- **首次启动的强制协议门**（`DisclaimerGate.vue`）：全屏显示完整条款，正文滚动到底部才能勾选，
  勾选后才能点「同意并继续」；也可以选择「不同意并退出」，程序直接关闭。
- **协议版本化管理**：`frontend/src/legal.ts` 的 `DISCLAIMER_VERSION` 与条款内容绑定，
  条款变更后递增版本号，已同意旧版本的用户会在下次启动重新看到协议。
- **高危操作确认升级**（`ConfirmDialog.vue`）：危险动作会逐条列出具体风险，
  并要求勾选「我已核对设备、目标与文件…」后才解锁确认按钮，防止误触和点穿。
- **页面级风险横幅**（`RiskBanner.vue`）：刷入镜像、线刷整包、一键 Root、控机脚本四个页面
  顶部常驻可折叠的风险说明，写清该页面独有的失败模式。
- **脚本破坏性命令检测**：控机脚本执行前会扫描 `fastboot erase/flash`、`adb shell rm -rf`、
  `pm clear`、`dd` 等模式，命中才弹确认，正常脚本不受打扰。
- **「设置 → 风险与合规」**：显示当前已同意的协议版本与时间，可随时重读完整声明，
  或撤销同意（撤销后立即退出，下次启动重新弹协议）。
- README 的免责章节重写为完整条款：明确的后果清单、合法用途、无担保与责任限制、软件边界。

### 变更

- `store.Config` 新增 `disclaimerVersion` / `disclaimerAcceptedAt` 字段。
- `SaveConfig` 对免责声明版本做**单向保护**：前端旧快照不会把已同意的记录覆盖回未同意状态。
- 一键 Root 的「生成补丁镜像」由普通确认改为高危确认（含风险清单与勾选确认）。

---

## [1.4.0] - 2026-11-10

以「敢让别人用」为目标的一轮工程化加固，功能无删减。

### 修复

- **并发安全（严重）**：`App` 的 `cancel` / `cfg` / `tools` 原为裸字段，无任何锁。
  Wails 的绑定调用与子进程回调分散在多个 goroutine，存在数据竞争。现已全部收敛到 `internal/session` 的加锁对象。
  - 「取消操作」与「新建操作」交错时，新任务可能被立刻取消 —— 改为**先登记新上下文再取消旧的**。
  - 旧任务收尾时会误清新任务的 cancel 登记 —— 引入任务代际序号，只有自己仍是当前任务才清空。
  - 配置保存与后台读取并发时的竞争 —— `Config()` 统一返回副本。
- **CI 形同虚设**：原工作流只跑 `go vet`，不跑测试也不做前端类型检查；Linux 与 macOS 的构建从未被真实验证。
  现已增加独立的 `check` job，并让构建产物为空时直接失败。

### 新增

- `internal/session` 包：配置快照 + 工具链状态 + 长任务取消登记，独立可测。
- 并发契约测试：`internal/session/session_test.go`（10 个用例）与 `app_test.go`（7 个用例），
  CI 中以 `-race -count=2` 执行。
- `internal/root` 测试：伪造 APK 验证 Magisk / KernelSU / APatch 的二进制解包命名、
  补丁命令完整性、错误安装包与未知方案的拒绝行为。
- `internal/store` 测试：默认值、损坏 JSON 容错、旧版本配置缺失字段回填、保存往返。
- README 增加「Windows 上被杀软拦截」说明与「并发模型」实现说明。
- `.github/workflows/build.yml` 增加 `check` job：gofmt / go vet / go test / go test -race / 前端 typecheck。

### 变更

- `app.go` 由 959 行拆分为 162 行核心 + 9 个 `bind_*.go`（按领域分组：设备、ADB、刷机、镜像、OTA、Root、工具、配置、系统调校）。
- 贡献指南补充测试要求与各模块的扩展入口。

## [1.3.0] - 2026-11-09

### 新增

- 显示控制：分辨率、DPI、字体缩放、三个动画倍率、自动锁屏、按键与文本输入。
- 系统界面：状态栏图标隐藏（18 项）、时钟显秒、电量百分比、快捷开关数量、沉浸模式、关闭 Wi-Fi 检测。
- 电池管理：电压 / 电流 / 功率 / 温度、定时采样曲线、CSV 导出、参数模拟与恢复。
- 性能监控：各核心实时频率、大小簇分组、governor、可调刷新间隔。
- 控机脚本：BAT 子集解释器（echo / set / set /a / if / for /l / goto / timeout / pause / exit + adb 透传）。
- 应用管理重写：5 种筛选、详情面板、批量勾选、Google 套件批量启停、操作记录。
- 文件传输：批量安装、拆分安装 `install-multiple`、推送、按路径拉取、按包名提取 split。

### 修复

- CPU 频率解析用 `SplitN` 分段数不足，导致首个小核频率被当作 0 丢弃。
- 脚本循环变量 `%%i` 未走变量替换规则，循环体拿到的是字面量。
- `goto` 跳到标签行后被当作命令执行。
- `if ... goto label` 无法跳转（`execOne` 未返回跳转目标）。
- 单行块判定会误伤 `set /a n=(1+2)`。
- 样式表缺少 `.panel-header` 别名，新页面标题栏无边框。

## [1.2.0] - 2026-11-09

### 移除

- **完全移除 Bootloader 解锁能力**（非隐藏入口，代码已删除）：
  `internal/unlock` 整包、三个绑定方法、fastboot 的 `UnlockStatus` / `QueryUnlock` / `Unlock`、
  前端解锁页面与路由。README 新增「不做的事」章节明确边界。

### 保留

- 仪表盘仍显示 `ro.boot.flash.locked` 的「已解锁 / 已锁定」状态（属设备信息，非解锁帮助）。

## [1.1.0] - 2026-11-09

### 新增

- **OTA 直链提取**：粘贴 full OTA 链接，用 HTTP Range 远程定位 `payload.bin`，
  只下载选中分区实际用到的字节。支持自定义 UA / Referer，也可整包下载。

### 修复

- `PartitionUpdate` 字段号错误（大小在 `new_partition_info = 7`、操作在 `operations = 8`，
  原实现用了 13 / 15），导致真实 OTA 解析出的分区大小与操作数全为 0。旧测试夹具跟随了错误定义，未能暴露。
- `extentWriter` 只写第一个 `dst_extents`，遇到拆分操作会丢数据。
- 进度事件因每个 4 KiB 块都上报而刷屏，改为整数百分比变化时才发。

## [1.0.0] - 2026-11-09

### 新增

- 首个版本：设备总览、ADB 终端、重启控制、单分区刷入、线刷整包、刷机包提取、
  一键 Root（KernelSU / KernelSU Next / Magisk / Magisk Alpha / Magisk Delta / APatch）、
  分区管理、应用管理、boot 镜像解析、sparse 转换、无线调试、日志导出。
