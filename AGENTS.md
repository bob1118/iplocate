# AGENTS.md

Go 1.27 单二进制 CLI（`module iplocate`，只用标准库）。根目录所有 `.go` 同属 `package main`，测试文件与源文件同名配对（`parse.go` ↔ `parse_test.go`），新增文件沿用该配对。用户可见文本一律中文，注释可英文。

| 文件 | 职责 |
| --- | --- |
| `main.go` | flag 参数（`-json` / `-timeout`）与 `os.Exit` |
| `runner.go` | `run()` 编排、`probeLocal`、`collectPublic`/`collectDNS`、`exitCode`、`warnf`、测试钩子 |
| `client.go` | `clientV4`/`clientV6`（按族拨号、2s 拨号超时）、`httpGet`（UA、64 KiB 上限、非 200 报错） |
| `local.go` | 固定探测目标、`firstDial`、`localIP`（源地址 + 网卡名） |
| `provider.go` | `defaultProviders` 服务表、`providersFor`、`fetchPublicIP`/`fetchGeoProviders` 预算与并发、ip-api 健康标志 |
| `parse.go` | 各服务响应解析与 `flattenJSON` |
| `dns_config.go` | 读 `ipconfig /all` 或 `/etc/resolv.conf` 并解析 |
| `dns_probe.go` | `inspectDNS`/`probeOneDNS` 可达性探测、`lookupDNSOwnership` 归属查询 |
| `output.go` | 中文文本与 JSON 输出、`fieldLabels`、列宽对齐 |
| `result.go` | `result` / `geoResult` / `field` 结构体与 JSON tag |

## 命令
- 运行/构建必须带 `.`：`go run .` / `go build .`——根目录是多文件包，单跑 `main.go` 编译不过。参数：`-json`、`-timeout`（默认 3s）。
- 验证顺序：`go vet ./...` → `gofmt -l .` → `go test ./...`（PowerShell 中分开执行）。
- `go test -race ./...` 需要 gcc。已装 MSYS2 ucrt64 的 gcc（`C:\msys64\ucrt64\bin\gcc.exe`），**把它加进 PATH 即可**——Go 会自动探测到 gcc 并把 `CGO_ENABLED` 从 0 翻成 1，不需要额外设环境变量，更不要 `go env -w`（那是全局的，会影响本机所有 Go 构建）。确认方法：`go env CGO_ENABLED` 返回 1。改并发代码后必跑，且配合 `-count=30` 压测。
- 本机 GitHub 不可达（15s 超时），`winget install` MinGW 会在下载阶段卡死；已装好的 MSYS2 不要重装。
- 单个测试用锚点：`go test -run '^TestParseIPAPI$' -v`。不带锚点是前缀匹配，会连带跑 `TestParseIPAPIFailure`。
- PowerShell 5.1 用 GBK 解码原生进程输出，`go run .` 的中文会显示成乱码——这不是 bug，先执行 `[Console]::OutputEncoding = [Text.Encoding]::UTF8`。同理 `Select-Object -First N` 截断管道会提前关闭 go run 并改变退出码，要看退出码就别截断。
- 产物输出到 GOBIN（`go build -o (Join-Path (go env GOBIN) "ip.exe") .`，当前 GOBIN=`C:\Users\bob\go\bin`），别落在仓库（`/*.exe` 已忽略）。C: 空间紧张时可把 `$env:GOTMPDIR` 指向 `D:\WORK\iplocate\tmp`（`/tmp/` 已 gitignore）。Defender 偶尔隔离未签名 Go 产物，异常时先看隔离记录。

## 执行流程与超时
- `run()`：先跑 `probeLocal`（本机 IPv4/IPv6 + IPv6 TCP 探测），再 `sync.WaitGroup` 并发跑 `collectPublic` 与 `collectDNS`，最后 `printResult`。加新步骤从这里入手。
- `probeLocal` 内部三个探测是**并发**的，`join` 后按 tcp4 → tcp6 → v6Reachable 的固定顺序回填并告警，所以 stderr 提示顺序稳定。实测三个探测合计仅约 26ms（UDP 拨号不做握手，近乎瞬时）——**别再把它当性能瓶颈**，真实耗时在下面那几轮网络查询里。
- `-timeout` 是**每次 `fetchGeo`/`fetchGeoProviders` 调用的预算**，不是整次运行总时限：公网 IPv4 一轮、公网 IPv6 一轮（直连失败再来一轮用本机 IPv6 显式查询）、每个 DNS 服务器的探测一轮 + 归属查询一轮。`fetchGeoProviders(providers, clients, timeout, budget, network)` 中 `budget` 是整轮上限（所有服务 × 所有客户端共享），`timeout` 是单次尝试上限；单轮内各服务是**并发**的，所以 `budget` 约束的是整体墙钟时间而非串行累加。
- 本机探测不受 `-timeout` 约束：`firstDial` 串行试 3 个固定目标、每目标固定 1s（`probeTimeout`），单族最长约 3s。HTTP 侧另有固定 2s 拨号超时，响应体超 64 KiB（`maxBodySize`）会显式报错而非静默截断；`http.Client` 自身未设 `Timeout`，超时全靠 context。
- `fetchGeo` 的 `network` 参数**只**用来给 `geoResult.Family` 赋值，真正决定地址族的是传入的 client。IPv6 直连失败是「用 clientV4 带 queryIP 显式查本机 IPv6」，Family 仍标 IPv6——别以为 client 和 network 一定一致。
- 本机 IPv6 源地址（`udp6` 拨号）与 IPv6 TCP 连通性探测相互独立；两者都成功才直连查公网 IPv6，TCP 探测失败**不**清除已取得的本机 IPv6。
- `probeOneDNS` 探测失败也照样做归属查询（只影响耗时，不是 bug）。`inspectDNS` 并发探测但按输入索引写回保序。
- `clientV4`/`clientV6` 走 `http.ProxyFromEnvironment`：`HTTP_PROXY`/`HTTPS_PROXY` 会改变实际查到的出口 IP，也会让 tcp6 client 改走代理。排查「公网 IP 不对」先看环境变量。

## 输出与退出码
- 文本顺序：本机 IPv4 → 公网 IPv4 →（有本机 IPv6 时）本机 IPv6 → IPv6 TCP 探测 → 公网 IPv6 → DNS IPv4 → DNS IPv6。无本机 IPv6 且 `DNSError` 为空时整段省略 DNS IPv6。`output_test.go` 的 `requireOutputOrder` 守住顺序。
- 数据写 stdout，警告/错误详情走 stderr（`warnf`，带互斥锁供并发 goroutine 共用）；测试用 `captureStdoutStderr` 分别捕获。
- 对齐靠 `line()`/`padLabel()`（目标显示宽度 18，东亚宽字符按 2 列算，超长也至少留 1 空格）。`fieldLabels` 是**服务返回字段** key→中文的映射，未命中的 key 原样显示英文；新增服务字段时改这里。
- 退出码 1 仅当公网 IPv4 **缺失或**失败、且公网 IPv6 为空或失败；DNS 失败不影响退出码。`exitCode` 已对 `res.PublicIPv4 == nil` 做防护，按「nil 即失败」处理（`TestExitCode` 覆盖 v4 缺失的各种组合）。
- JSON：`public_ipv4` 无 `omitempty` 恒存在；`public_ipv6` 未查询时为 `null`、失败时为带 `error` 的对象。`dnsResult.Network` 是展示分组，`Family` 仅作 JSON 兼容保留。
- 改 `result`/`geoResult` 字段时，文本输出与 JSON 输出两处都要检查。

## 服务、DNS 与网络
- `defaultProviders()` 顺序 ip-api.com → ipinfo.io → ipapi.co → myip.ipip.net。`fetchPublicIP` **并发**请求全部服务，**按完成顺序**取第一个成功者并 `cancelAll()` 取消其余——不是按声明顺序。声明顺序只用于**全失败时**的错误文本拼接，保证报错可复现（`TestFetchPublicIPAllFailErrorKeepsDeclarationOrder` 守住）。因此不要给「哪个服务胜出」写依赖声明顺序的测试；测试应通过控制「哪个服务成功」来保证确定性。
- 代价（已获用户确认）：**每次自动查询都会把出口 IP 发给全部四家服务**，README 已明写。client 回退（`ownershipClients` 给出 v6→v4）会再乘一轮请求。
- channel 容量等于 `len(providers)` + `defer cancelAll()`，保证成功路径提前返回后剩余 goroutine 仍能无阻塞写入、不泄漏。改这段时务必保留这两个性质。
- 新增服务需在 `defaultProviders()` 注册并配解析器；解析器只填 `IP`/`Fields`，`Source`/`Raw`/`Family` 由 `fetchOneProvider` 补。
- `providersFor("")` 返回全部服务（自动查询）；显式 IP 只返回 URL 含 `{ip}` 的服务——实际只有 ip-api.com，所以 **DNS 归属查询永远只走 ip-api**；它在本机网络下很慢，`-timeout 2s` 时 `context deadline exceeded`（公网 IPv4 与 DNS 归属一起失败），给到 8s 才通——排查「归属查不到」先怀疑预算而不是代码。
- ip-api 用明文 HTTP 且带 `lang=zh-CN` + `fields=` 白名单（隐私取舍见 README），`parseIPAPI` 依赖 `status`/`message`/`query` 字段名。`parseIPInfo` 同时兼容 ipinfo.io 与 ipapi.co 两种形状；`parseIPIPNet` 用正则解析中文文本与压缩 IPv6；`flattenJSON` 把嵌套展平成 `a.b.0` 形式的 key，保留未知字段、忽略 `null`。
- ip-api 的 HTTP 429 / `requires an authentication key`（`respErrIsLimited` 匹配错误字符串）会置全局 `geoAPIServiceHealthy=false`，之后 `lookupDNSOwnership` 直接返回 nil（输出「（未查询）」），公网服务回退不受影响。429 来自 `httpGet` 的非 200 判断。
- DNS 归属结果必须过 `ensureExplicitIP`：返回 IP 与被查 DNS 服务器 IP（`net.IP.Equal`，容忍 IPv6 不同写法）不一致时，整个结果被换成 `error`、IP 清空。
- `ownershipClients`：IPv6 DNS 归属先试 clientV6 再回退 clientV4；IPv4 只有 clientV4。
- `configuredDNS` 用本机 IPv4/IPv6 过滤 `ipconfig /all` 的网卡区块：两者都为空时 `active` 为空、**不过滤**，会返回所有网卡的 DNS——改这里会静默改变服务器选择。`primaryDNS` 按配置顺序每族取第一个。Unix 读 `/etc/resolv.conf`。
- DNS 探测目标是 `example.com`（`dnsProbeDomain`），网络族由 DNS 服务器地址决定（`udp4`/`udp6`）。

## 测试约束
- 禁止真实网络/DNS：用 `httptest`、`failTransport`（`RoundTrip` 直接返回错误），或把 client 注入 `fetchPublicIP` / `fetchGeoProviders` / `inspectDNS`。测服务选择测 `providersFor`，**不要**直接调真实网络版 `fetchGeo`。
- 可替换的包级钩子：`localIPForProbe`、`networkAvailableForProbe`、`fetchGeoForPublic`（`runner.go`）、`readWindowsDNSConfig`、`runtimeGOOSForTest`（`dns_config.go`）、`geoAPIServiceHealthy`（`provider.go`）。除最后一个是 `atomic.Bool` 外都是普通全局变量——测试改完必须 defer 恢复原值（或用 `t.Cleanup`），且不要在 `t.Parallel()` 中改写。
- 并发测试注意：`probeLocal` 现在并发调用 `localIPForProbe`，测试钩子里收集调用记录**必须加锁**，否则测试自身就有数据竞争。
- Windows 解析测试注入 GBK 字节样本（`readWindowsDNSConfig` 返回原始字节即可）；`ipconfig` 输出解析为空时 `configuredDNS` 会给出编码相关的防御性错误，改解析器时保住这条失败路径。
- 无第三方断言库，断言直接 `t.Fatalf`。
