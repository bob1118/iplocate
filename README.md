# iplocate

一个 Go CLI，用于查看本机出口 IPv4/IPv6、公网 IP 归属，以及系统 DNS 的可达性和归属信息。程序使用 Go 标准库，无第三方 Go 依赖。

## 运行

```powershell
go run .
go run . -json
go run . -timeout 5s
```

`-timeout` 默认 `3s`，用于每轮公网查询，以及每次 DNS 可达性探测和 DNS 归属查询。单轮内四个归属服务是并发请求的，`-timeout` 约束的是该轮的整体墙钟时间，每个服务的单次尝试同样不超过它。IPv6 直连失败后还可能进行一轮显式 IP 回退查询。它**不是整次程序运行的总时限**：公网 IPv4 与 IPv6 两轮之间顺序执行，DNS 探测之后还可能执行独立的归属查询。本机地址和 IPv6 连通性预探测使用固定的每目标 `1s` 超时。

## IPv6 判定

程序分别记录路由选择出的本机 IPv6 源地址和对固定目标的 TCP 连通性探测。TCP 探测失败不会清除已取得的本机 IPv6 地址；但只有探测通过且取得本机 IPv6 时，才会直连查询公网 IPv6。结果 JSON 中新增的 `ipv6_probe_reachable` 表示该 TCP 探测结果，不等价于对所有 IPv6 站点的普遍可达性；`local_ipv6` 表示用于出口路由的源地址，不代表枚举到的所有网卡地址。

## 数据流与隐私

- 公网归属查询**并发**请求 ip-api.com、ipinfo.io、ipapi.co、myip.ipip.net 四家，取第一个成功返回的结果。这意味着无论哪家先响应，**每次自动查询都会把出口 IP 发给全部四家服务**，而不是只发给其中一家。DNS 归属查询会向支持显式 IP 查询的服务发送 DNS 服务器地址（当前只有 ip-api.com）。
- DNS 可达性检查向系统配置的 DNS 服务器查询 `example.com`。Windows 从 `ipconfig /all` 读取配置；Unix 从 `/etc/resolv.conf` 读取。
- ip-api.com 当前使用 HTTP；该连接不具备 HTTPS 的传输加密与完整性保护。避免在不可信网络中把该服务响应视为经过认证的数据。

## 输出与退出码

默认输出中文文本；`-json` 输出 JSON。JSON 中 `public_ipv4` 始终为结果对象；`public_ipv6` 在未查询时为 `null`，查询失败时为带 `error` 的对象。DNS 读取、探测或归属失败不会单独导致非零退出码；只有公网 IPv4 失败且公网 IPv6 为空或失败时，退出码为 `1`。

## 开发验证

```powershell
go test ./...
go vet ./...
gofmt -l .
```

并发逻辑改动后额外运行 `go test -race ./...`（需要 CGO 与 gcc，配置见 AGENTS.md）。测试不依赖真实公网服务或 DNS。
