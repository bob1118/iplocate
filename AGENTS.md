# AGENTS.md

Go CLI (`module iplocate`, stdlib-only) that prints local IPv4/IPv6 plus public IP and the full provider response.

## Commands

- Build/run: `go run .` — NOT `go run main.go`; `main` spans multiple files in the root (`main.go`, `local.go`, `public.go`).
- Test: `go test ./...`
- Single test: `go test -run TestParseIPAPI`
- Vet/format: `go vet ./... && gofmt -l .` (both must stay clean)

## Conventions

- All user-facing strings (CLI labels, warnings, errors) are written in **Chinese**. Match this in new code.
- CLI output lines use `padLabel()` with CJK-aware `displayWidth()` for column alignment — reuse them for any new output line instead of manual spacing.
- Keep zero external dependencies; everything is standard library only.
- Provider responses are flattened into ordered key/value pairs (`flattenJSON` in public.go); the CLI prints **every** returned field — known keys get Chinese labels via `fieldLabels` (main.go), unknown keys print raw. Don't trim fields when adding providers.
- Public IP lookup tries providers sequentially in the order returned by `defaultProviders()` (public.go); first success wins, failures aggregate into one error. Preserve this fallback behavior — it's covered by tests using `httptest` servers.

## Testing

- Tests are pure unit tests over parsers and `fetchPublicIP` fallback logic via `httptest`. No network access or environment setup is required; keep it that way (add parse-level tests rather than live-provider tests).
