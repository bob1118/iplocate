# AGENTS.md

Go 1.27 标准库 CLI（`module iplocate`），代码位于仓库根目录。`main` 分布在多个文件中，运行必须使用 `go run .`，不要使用 `go run main.go`。

## 命令

- 全部测试：`go test ./...`
- 单个测试：`go test -run TestParseIPAPI`
- 校验：`go vet ./... && gofmt -l .`
- 运行：`go run .`（参数：`-json`、`-timeout`；每个服务默认超时 3 秒）
- Windows 编译：`go build -o ip.exe .`；输出到 Go bin：`go build -o "$env:GOBIN\ip.exe" .`

## 关键约束

- `run()` 负责本机地址、IPv6 可达性、公网查询和输出；仅当所有已尝试的公网查询都失败时返回退出码 1，部分成功返回 0。
- 修改 `result`/`geoResult` 时同步维护 `printText` 与 `printJSON`；本机网卡名对应 JSON 字段 `interface_v4`/`interface_v6`。
- `defaultProviders()` 按顺序尝试服务，首个解析成功的结果胜出。保持 `*http.Client` 经 `fetchGeo`/`fetchPublicIP` 注入传递，不要在查询循环中改用全局客户端。
- HTTP 客户端固定网络族（`tcp4`/`tcp6`）。只有 `networkAvailable("tcp6")` 成功才执行 IPv6；直连 IPv6 失败且存在本机 IPv6 时，`run()` 会通过 IPv4 客户端重试带 `{ip}` 的服务（当前为 ip-api.com）。
- DNS 从 Windows 的 `ipconfig /all` 或 Unix 的 `/etc/resolv.conf` 读取；Windows 只保留包含当前 IPv4/IPv6 本机地址的网卡区块，并标记适用网络。输出每个网络族配置顺序中的第一台 DNS，并用 `example.com` 探测；DNS/归属查询失败不能改变公网查询的退出码。
- `providersFor("")` 返回自动检测的全部服务；非空 IP 只选择 URL 含 `{ip}` 的服务。新增服务必须同时加入 `defaultProviders()` 并提供解析器。
- 解析器只填写 `IP` 和 `Fields`；`fetchPublicIP` 添加 `Source`/`Raw` 并拒绝空 IP，`fetchGeo` 添加 `Family`。解析失败必须返回 `(nil, err)`。
- JSON 响应会展平为有序字段并完整输出，不能丢弃未知字段。`parseIPInfo` 同时处理 ipinfo.io 和 ipapi.co；`parseIPIPNet` 处理 myip.ipip.net 的中文纯文本响应。

## 约定与测试

- 用户可见的标签、提示和错误使用中文。文本输出复用 `line()`/`padLabel()`，以保持中文显示宽度对齐。
- 测试位于 `parse_test.go`、`provider_test.go` 和 `dns_test.go`，不得依赖真实网络。服务行为用注入客户端配合 `httptest` 测试；DNS 配置解析和筛选使用纯函数测试，服务选择测 `providersFor`，不要直接测试会访问真实服务的 `fetchGeo`。
