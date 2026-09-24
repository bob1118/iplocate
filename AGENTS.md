# AGENTS.md

Go 1.27 CLI（`module iplocate`）；根目录源码同属 `package main`。运行/构建用 `go run .` / `go build .`，不要只指定 `main.go`。

## 命令
- 全部测试：`go test ./...`；单个测试：`go test -run TestParseIPAPI`。
- 改动并发逻辑后运行 `go test -race ./...`；验证顺序为 `go vet ./...`，再运行 `gofmt -l .`（PowerShell 中分开执行）。
- 运行：`go run . [-json] [-timeout 3s]`；超时预算按公网查询轮次、DNS 探测和归属查询分别计算（IPv6 回退还可能多一轮），不是整次运行的总时限；本机探测固定每个目标 1 秒。
- Windows 可将 `$env:GOTMPDIR` 设为 `D:\WORK\iplocate\tmp`；编译产物放在 GOBIN（`go build -o (Join-Path (go env GOBIN) "ip.exe") .`），不要留在仓库。Defender 可能隔离未签名 Go 产物，异常时先检查隔离记录。

## 执行与输出
- `runner.go` 串行探测本机地址，再并发收集 DNS 和公网结果；显示顺序由 `output.go` 控制。
- 文本顺序：本机 IPv4 → 公网 IPv4 →（有本机 IPv6 时）本机 IPv6 → 公网 IPv6 → DNS IPv4 → DNS IPv6。无本机 IPv6 且 DNS 读取正常时省略 DNS IPv6。
- IPv6 本机源地址探测独立于固定目标的 TCP 连通性探测；只有两者都成功时才直连查询公网 IPv6，直连失败时通过 IPv4 客户端显式查询本机 IPv6。
- 仅公网 IPv4 失败且公网 IPv6 为空/失败时返回退出码 1；DNS 错误不影响退出码。JSON 的 `public_ipv4` 始终是对象，`public_ipv6` 不可用时为 `null`。
- 修改 `result`/`geoResult` 时同步检查文本和 JSON 输出；网卡名字段为 `interface_v4`/`interface_v6`。用户可见文本用中文，并通过 `line()`/`padLabel()` 对齐。

## 服务与 DNS
- `defaultProviders()` 顺序尝试并以首个解析成功者为准；解析器只填 `IP`/`Fields`，抓取层补 `Source`/`Raw`/`Family`。新增服务要注册并提供解析器。
- `providersFor("")` 给自动查询返回全部服务；显式 IP 只返回 URL 含 `{ip}` 的服务。HTTP 客户端固定使用 `tcp4`/`tcp6`，同一总预算覆盖所有服务及客户端重试。
- 仅 ip-api 的 HTTP 429/鉴权限流错误会置全局不健康标志；之后跳过 DNS 归属查询，但公网服务回退流程不变。JSON 展平保留未知字段、忽略 `null`；`parseIPInfo` 兼容 ipinfo.io/ipapi.co，`parseIPIPNet` 解析中文文本和压缩 IPv6。
- Windows 从 `ipconfig /all` 解析并仅保留含本机地址的网卡区块；Unix 读 `/etc/resolv.conf`。DNS 网络族按 DNS 服务器地址判断，每族按配置顺序取首个服务器。
- DNS 用 `example.com` 经 `udp4`/`udp6` 探测；探测并发但按输入索引保序。显式归属结果必须与查询 IP 一致；IPv6 DNS 归属先用 IPv6 HTTP 客户端，再回退 IPv4。

## 测试注意
- 测试不得依赖真实网络：用 `httptest`/伪造 Transport、注入 DNS 配置或替换 `runtimeGOOSForTest`；服务选择测 `providersFor`，行为测可注入客户端的 `fetchPublicIP`/`fetchGeoProviders`，不要直接调用真实网络版 `fetchGeo`。
- Windows DNS 解析可注入 GBK 样本；若 `ipconfig` 输出解析为空，代码会给出编码相关防御性错误。
