# AGENTS.md

Go CLI (`module iplocate`, stdlib-only) that prints local IPv4/IPv6 plus public IP and the full provider response.

## Commands

- Build/run: `go run .` — NOT `go run main.go`; `main` spans multiple files in the root (`main.go`, `local.go`, `provider.go`, `parse.go`, `output.go`, `result.go`).
- Test: `go test ./...`
- Single test: `go test -run TestParseIPAPI`
- Vet/format: `go vet ./... && gofmt -l .` (both must stay clean)

## Architecture notes

- Two output modes must stay in sync: human-readable (`printText`) and `-json` (`printJSON`), both in output.go over the `result`/`geoResult` structs in result.go. New data means updating both. The `-timeout` flag sets the per-provider HTTP timeout (default 3s).
- Public IP lookup tries providers sequentially in the order returned by `defaultProviders()` (provider.go); first success wins, failures aggregate into one error that `fetchGeo` prefixes with the address family. Preserve this fallback behavior — it's covered by tests using `httptest` servers.
- HTTP traffic is family-pinned via `newFamClient("tcp4"/"tcp6")` (provider.go); `run()` picks `clientV4`/`clientV6`, and the client flows through `fetchGeo(client, timeout, network, queryIP)` into `fetchPublicIP(providers, client, timeout)` — no global client lookups inside the fetch loop. If the direct IPv6 sweep fails while a local v6 address exists, `run()` retries once via `providersFor(v6Addr)`: only URLs containing the `{ip}` placeholder (currently ip-api.com), dialed over `clientV4` since the queried address travels in the URL itself.
- IPv6 is gated by `networkAvailable("tcp6")` (local.go): real TCP dials against hard-coded targets. Without a working v6 egress the tool skips all v6 work and prints a hint instead of timing out on every provider. The UDP-dial trick survives only in `localIP`, where its job is source-address discovery, not reachability.
- Provider responses are flattened into ordered key/value pairs (`flattenJSON`, parse.go); the CLI prints **every** returned field — known keys get Chinese labels via `fieldLabels` (output.go), unknown keys print raw. Don't trim fields when adding providers.
- To add a provider: append an entry to `defaultProviders()` plus a parser in parse.go. URLs may embed `{ip}` — `providersFor` always substitutes it: empty queryIP restores the auto-detect URL, non-empty selects that provider (and only those) for explicit-address queries. `parseIPInfo` serves both ipinfo.io and ipapi.co (same JSON shape); `myip.ipip.net` is the exception — plain Chinese text parsed by regex in `parseIPIPNet`, which synthesizes `country`/`region`/`city`/`isp` fields instead of flattening; its IP match falls back to an IPv6 pattern validated with `net.ParseIP`. Parsers fill only `IP` + `Fields`: `Source`/`Raw` are attached by `fetchPublicIP`, `Family` by `fetchGeo`, and a parsed result with an empty `IP` is rejected there. On failure parsers must return `(nil, err)` — tests assert nil info alongside errors.
- Exit code: `run()` returns 1 only when every attempted public-IP lookup failed (IPv4 failure plus either no IPv6 attempt or IPv6 failure too); partial success exits 0.
- Local IPs record one interface name per family (`interface_v4`/`interface_v6` JSON keys).

## Conventions

- All user-facing strings (CLI labels, warnings, errors) are written in **Chinese**. Match this in new code.
- CLI output lines use `line()`/`padLabel()` with CJK-aware `displayWidth()` for column alignment (output.go) — reuse them for any new output line instead of manual spacing.
- Keep zero external dependencies; everything is standard library only.

## Testing

- Tests live in parse_test.go and provider_test.go, split by source file. They are pure unit tests over parsers and `fetchPublicIP` fallback logic via `httptest`. No network access or environment setup is required; keep it that way (add parse-level tests rather than live-provider tests).
- `fetchPublicIP` takes an injected `*http.Client`, so tests pass `http.DefaultClient` and hit loopback httptest servers regardless of family pinning.
- Don't try to unit-test `fetchGeo` directly: its provider list comes from hard-wired `defaultProviders()`/`providersFor()`, so a direct call would hit the real network. Test URL selection via `providersFor` and HTTP/fallback behavior at the `fetchPublicIP` level with synthetic provider slices instead.
