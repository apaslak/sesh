# Repository guidance

Sesh is a Go 1.26+ CLI for tmux session management. This fork also includes an
opt-in live tmux tree picker and an OpenCode V2 CLI status plugin.

## Code map

- `main.go` → `seshcli/`: Cobra commands; `seshcli/deps.go` wires dependencies.
- `lister/`, `connector/`, `namer/`: source listing, connections, session naming.
- `model/`, `configurator/`: shared types and TOML configuration.
- `tmux/`, `zoxide/`, `tmuxinator/`: external tool integrations.
- `picker/`: Bubble Tea UI. `tree.go` and `tree_filter.go` implement `--tree`;
  keep ordinary picker behavior unchanged when that flag is absent.
- `tmux/tree.go`: server-wide topology and stable session/window/pane targets.
- `agentstatus/`: validated local status snapshots and picker visibility leases.
- `plugins/opencode-sesh/`: independent TypeScript/Bun OpenCode V2 CLI plugin.
  Root `tui.ts` is required for local-directory plugin discovery.

See `CONTRIBUTING.md` for broader development and benchmark guidance.

## Development checks

Generate ignored `mock_*.go` files before testing a fresh checkout or after
changing interfaces. Do not hand-edit or commit generated mocks. If stale mocks
prevent generation, discover them with a glob and remove only the affected
generated files; do not delete unrelated work.

```sh
GOFLAGS="-buildvcs=false" go tool mockery
go test -race ./...
go vet ./...
go build ./...
```

Use focused package tests while iterating, then the full checks before claiming
completion. Run `gofmt` on changed Go files. Benchmark listing/filtering changes
using the commands in `CONTRIBUTING.md`; do not benchmark under `-race`.

For plugin changes, run these from `plugins/opencode-sesh/`:

```sh
bun install --frozen-lockfile
bun test
bun run typecheck
```

## Change constraints

- Make small changes following existing interfaces, wrappers, error handling,
  and `slog` patterns. Test behavior, not implementation details.
- Keep TOML fields/defaults and `sesh.schema.json` synchronized. Check schema
  coverage whenever changing configuration structs in `model/` or parsing in
  `configurator/`.
- Preserve TypeScript type safety: no `any`, type assertions, or non-null
  assertions. Validate boundary inputs; use discriminated unions for states.
- OpenCode integration targets V2 public APIs. Do not substitute V1 APIs or
  infer pane identity from the shared server; the local CLI owns that context.
- Missing, malformed, disconnected, or stale agent data is unavailable—not
  idle or successful. Highlighting a picker row must not acknowledge completion.
- Use stable tmux IDs and retain session ancestry for linked windows. Sanitize
  externally supplied labels before rendering them.

## Safe builds and integration tests

Avoid `go install` and `just build` unless replacing the installed `sesh` is
explicitly requested. Build this fork under a separate name instead:

```sh
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/sesh-fork" .
```

Use an absolute binary path in tmux bindings; do not rely on shell aliases.
Separate binaries still share default sesh configuration/cache locations.

Use disposable tmux servers (`tmux -L <unique-name> -f /dev/null ...`) for live
tests. Clean up only the server created by the test, never existing user panes
or sessions. Do not edit personal tmux/OpenCode configs, install the plugin,
replace binaries, or reload live configuration without explicit authorization.
Plugin setup and known limitations: `plugins/opencode-sesh/README.md`.
