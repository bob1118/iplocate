# AGENTS.md

Go 1.27 标准库 CLI（`module iplocate`），代码位于仓库根目录。`main` 分布在多个文件中，运行必须使用 `go run .`，不要使用 `go run main.go`。

## 输出格式

文本输出顺序固定为：

```
本机 IPv4 (网卡名)
公网 IPv4 + 归属详情
本机 IPv6 (网卡名)          ← 仅 IPv6 可用时
公网 IPv6 + 归属详情        ← 仅 IPv6 可用时
DNS IPv4 + 探测/归属详情
DNS IPv6 + 探测/归属详情    ← 仅 IPv6 可用时
```

详细字段顺序调整见 `output.go` 的 `printText`；JSON 顺序由 `result.go` struct 决定。

## 文件职责

- `main.go`：flag 解析 + `os.Exit(run(...))`。
- `runner.go`：编排。`probeLocal`（本机地址/IPv6 可达性，串行在前）、`collectDNS`、`collectPublic`（两者并发执行，结果写入共享 `res`）、`exitCode`（退出码纯函数）、`warnf`（带锁 stderr）。
- `client.go`：`newFamClient`、全局 `clientV4`/`clientV6`、`httpGet`。
- `provider.go`：服务注册表、`providersFor`、`fetchPublicIP`、`fetchGeo`/`fetchGeoProviders`。重试有总预算（`budget` 参数），同一错误不超预算盲目重试。
- `dns_config.go`：DNS 配置读取与解析（ipconfig/resolv.conf）；`readWindowsDNSConfig` 与 `runtimeGOOSForTest` 是可注入变量，测试直接替换。
- `dns_probe.go`：DNS 探测（`probeConfiguredDNS` 按地址族固定 `udp4`/`udp6`）、归属查询、`inspectDNS`（并发探测），结果按索引写回保证顺序稳定。
- `output.go` / `result.go` / `parse.go` / `local.go`：输出、数据结构、解析、本机探测。

## 命令

- 全部测试：`go test ./...`
- 单个测试：`go test -run TestParseIPAPI`
- 校验：`go vet ./... && gofmt -l .`
- 运行：`go run .`（参数：`-json`、`-timeout`；每个服务默认超时 3 秒）
- Windows 编译：`go build -o ip.exe .`；输出到 Go bin：`go build -o "$(go env GOBIN)\ip.exe" .`

## Windows Defender 误报（重要）

本机 Defender 会将编译产物误判为病毒并隔离（症状：exe 编译后消失、启动报"文件正在被另一个程序使用"、go build 报 "contains a virus or potentially unwanted software"）。规避：编译前设置 `$env:GOTMPDIR="D:\WORK\iplocate\tmp"`（已在例外目录），产物本地命名用 `iplocate.exe`。这是 Defender 对 Go 未签名二进制的误报，与代码无关；请管理员在 Defender 排除列表中加入 `D:\WORK\iplocate` 与 `C:\Users\bob\go\bin`。

## 关键约束

- 公网查询与本机 DNS 查询并发执行（`sync.WaitGroup`）；文本输出顺序固定为：本机 IPv4 → 公网 IPv4 → 本机 IPv6 → 公网 IPv6 → DNS IPv4 → DNS IPv6。IPv6 未启用时整段跳过（输出与 JSON 中均不出现）。
- 仅当所有已尝试的公网查询都失败时返回退出码 1，部分成功返回 0；DNS 读取/探测/归属失败不改变退出码。
- 修改 `result`/`geoResult` 时同步维护 `printText` 与 `printJSON`；本机网卡名对应 JSON 字段 `interface_v4`/`interface_v6`。
- `defaultProviders()` 按顺序尝试服务，首个解析成功的结果胜出。保持 `*http.Client` 经 `fetchGeo`/`fetchGeoProviders` 注入传递；多客户端依次重试（首选匹配地址族）。
- HTTP 客户端固定网络族（`tcp4`/`tcp6`）。只有 `networkAvailable("tcp6")` 成功才执行 IPv6；直连 IPv6 失败且存在本机 IPv6 时，通过 IPv4 客户端重试带 `{ip}` 的服务（当前为 ip-api.com）。
- ip-api 免费接口受 45 次/分钟限流：`geoAPIServiceHealthy`（atomic）在收到 HTTP 429 或 `status != success` 时置 false，此后 DNS 归属查询直接跳过（返回 nil Ownership），但自动公网查询不受影响。
- DNS 从 Windows 的 `ipconfig /all` 或 Unix 的 `/etc/resolv.conf` 读取；Windows 只保留包含当前 IPv4/IPv6 本机地址的网卡区块。DNS 服务器的网络标签取自身地址族（`familyOfIP`），不取网卡区块匹配结果（避免临时 IPv6 地址先于 IPv4 行出现的错标）。Windows 解析结果为空时返回防御性错误（提示可能为 GBK 编码）。每族取配置顺序第一台 DNS，并用 `example.com` 探测；DNS/归属查询失败不能改变公网查询的退出码。
- `providersFor("")` 返回自动检测的全部服务；非空 IP 只选择 URL 含 `{ip}` 的服务。新增服务必须同时加入 `defaultProviders()` 并提供解析器。
- 解析器只填写 `IP` 和 `Fields`；`fetchPublicIP` 添加 `Source`/`Raw` 并拒绝空 IP，`fetchGeo`/`fetchGeoProviders` 添加 `Family`。解析失败必须返回 `(nil, err)`。
- JSON 响应会展平为有序字段并完整输出，不能丢弃未知字段。`parseIPInfo` 同时处理 ipinfo.io 和 ipapi.co；`parseIPIPNet` 处理 myip.ipip.net 的中文纯文本响应。

## 约定与测试

- 用户可见的标签、提示和错误使用中文。文本输出复用 `line()`/`padLabel()`，以保持中文显示宽度对齐。
- 测试位于 `parse_test.go`、`provider_test.go`、`dns_config_test.go`、`dns_probe_test.go`、`misc_test.go`，不得依赖真实网络。服务行为用注入客户端配合 `httptest` 测试；DNS 配置解析和筛选使用纯函数测试（GBK 字节样本可直接注入 `readWindowsDNSConfig`），服务选择测 `providersFor`，不要直接测试会访问真实服务的 `fetchGeo`。并发代码验证用 `go test -race ./...`。
