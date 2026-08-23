# AGENTS.md

Go CLI (`module iplocate`, stdlib-only) that prints local IPv4/IPv6 plus public IP and the full provider response.

## Commands

- Build/run: `go run .` — NOT `go run main.go`; `main` spans multiple files in the root (`main.go`, `local.go`, `public.go`).
- Test: `go test ./...`
- Single test: `go test -run TestParseIPAPI`
- Vet/format: `go vet ./... && gofmt -l .` (both must stay clean)

## Architecture notes

- Two output modes must stay in sync: human-readable (`printResult`) and `-json` (the `result`/`geoResult` structs in main.go). New data means updating both. The `-timeout` flag sets the per-provider HTTP timeout (default 3s).
- Public IP lookup tries providers sequentially in the order returned by `defaultProviders()` (public.go); first success wins, failures aggregate into one error. Preserve this fallback behavior — it's covered by tests using `httptest` servers.
- All HTTP traffic is family-pinned: `newFamClient("tcp4"/"tcp6")` (public.go) dials only that address family. `run()` exits non-zero only when **both** the IPv4 and IPv6 public lookups fail.
- All IPv6 work is gated by `networkAvailable("tcp6")` — UDP probes against hard-coded public targets in local.go; without v6 connectivity the tool skips IPv6 queries entirely.
- Provider responses are flattened into ordered key/value pairs (`flattenJSON` in public.go); the CLI prints **every** returned field — known keys get Chinese labels via `fieldLabels` (main.go), unknown keys print raw. Don't trim fields when adding providers.
- To add a provider: append an entry to `defaultProviders()` plus a parser. `parseIPInfo` serves both ipinfo.io and ipapi.co (same JSON shape). On failure parsers must return `(nil, err)` — tests assert nil info alongside errors.

## Conventions

- All user-facing strings (CLI labels, warnings, errors) are written in **Chinese**. Match this in new code.
- CLI output lines use `padLabel()` with CJK-aware `displayWidth()` for column alignment — reuse them for any new output line instead of manual spacing.
- Keep zero external dependencies; everything is standard library only.

## Testing

- Tests are pure unit tests over parsers and `fetchPublicIP` fallback logic via `httptest`. No network access or environment setup is required; keep it that way (add parse-level tests rather than live-provider tests).
- In `fetchPublicIP` tests pass network `"tcp4"` — httptest servers bind IPv4 loopback, which the family-pinned `clientV6` cannot reach.
