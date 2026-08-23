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
- HTTP traffic is family-pinned via `newFamClient("tcp4"/"tcp6")` (provider.go); `run()` picks the client and passes it into `fetchPublicIP(providers, client, timeout)` — no global client lookups inside the fetch loop.
- IPv6 is gated by `networkAvailable("tcp6")` (local.go): real TCP dials against hard-coded targets. Without a working v6 egress the tool skips all v6 work and prints a hint instead of timing out on every provider. The UDP-dial trick survives only in `localIP`, where its job is source-address discovery, not reachability.
- Provider responses are flattened into ordered key/value pairs (`flattenJSON`, parse.go); the CLI prints **every** returned field — known keys get Chinese labels via `fieldLabels` (output.go), unknown keys print raw. Don't trim fields when adding providers.
- To add a provider: append an entry to `defaultProviders()` plus a parser in parse.go. `parseIPInfo` serves both ipinfo.io and ipapi.co (same JSON shape). On failure parsers must return `(nil, err)` — tests assert nil info alongside errors.
- Local IPs record one interface name per family (`interface_v4`/`interface_v6` JSON keys).

## Conventions

- All user-facing strings (CLI labels, warnings, errors) are written in **Chinese**. Match this in new code.
- CLI output lines use `line()`/`padLabel()` with CJK-aware `displayWidth()` for column alignment (output.go) — reuse them for any new output line instead of manual spacing.
- Keep zero external dependencies; everything is standard library only.

## Testing

- Tests are pure unit tests over parsers and `fetchPublicIP` fallback logic via `httptest`. No network access or environment setup is required; keep it that way (add parse-level tests rather than live-provider tests).
- `fetchPublicIP` takes an injected `*http.Client`, so tests pass `http.DefaultClient` and hit loopback httptest servers regardless of family pinning.
