# Set up sesh-fork

This fork adds an agent-focused tmux picker with OpenCode V2 status. Install it
beside upstream `sesh`, not over it. **No Go toolchain or repository clone needed.**

Release packaging is prepared here; downloadable fork assets become available
only after a maintainer publishes a release. Do not use upstream release assets
or `brew install sesh` for this feature.

## 1. Requirements

- macOS or Linux, ARM64 or x86-64.
- tmux with `display-popup` support (3.2+).
- OpenCode V2, with compatible public CLI plugin APIs (tested on 2.0.26).
- [Bun](https://bun.sh/docs/installation) for plugin dependency installation.
  Release users do not need to compile Go or TypeScript.
- Optional: zoxide for directory history; `fd` for the picker's directory search.

The picker targets local tmux panes. Remote tmux servers and headless agents are
not discovered. A local OpenCode CLI may connect to a remote OpenCode service.

## 2. Download and verify

Open [this fork's Releases](https://github.com/apaslak/sesh/releases). Download
`checksums.txt` and one **sesh-fork** archive from the same release. Do not use
GitHub's automatic source-code archives.

Run `uname -s` and `uname -m` to choose:

| System | Architecture | Archive suffix |
| --- | --- | --- |
| macOS (`Darwin`) | Apple Silicon (`arm64`) | `_darwin_arm64.tar.gz` |
| macOS (`Darwin`) | Intel (`x86_64`) | `_darwin_amd64.tar.gz` |
| Linux | ARM64 (`aarch64` or `arm64`) | `_linux_arm64.tar.gz` |
| Linux | x86-64 (`x86_64`) | `_linux_amd64.tar.gz` |

In the download directory, set `archive` to the filename you downloaded:

```sh
archive="sesh-fork_<version>_darwin_arm64.tar.gz"
```

Replace `<version>` and the OS/architecture suffix with your release's filename.
On macOS:

```sh
grep -F "  $archive" checksums.txt | shasum -a 256 -c -
```

On Linux:

```sh
grep -F "  $archive" checksums.txt | sha256sum -c -
```

Continue only when the command succeeds and reports your archive **OK**. Checksums
detect corrupted downloads, not a compromised release publisher.

## 3. Keep the bundle in a stable location

Each archive contains `sesh-fork`, this guide, a license, and
`plugins/opencode-sesh/` from the same revision. Extract it into a versioned
directory under your home. Keep the plugin there, not in a temporary download
folder.

```sh
mkdir -p "$HOME/.local/share/sesh-fork"
tar -xzf "$archive" -C "$HOME/.local/share/sesh-fork"
bundle="$HOME/.local/share/sesh-fork/${archive%.tar.gz}"
"$bundle/sesh-fork" --version
"$bundle/sesh-fork" picker --help
```

The binary can stay here; no copying into `~/.local/bin/sesh` is necessary.
`picker --help` should list `--tree`. On macOS, downloaded executables may need
approval through System Settings → Privacy & Security. These builds are not
Developer ID signed/notarized; do not globally disable Gatekeeper.

## 4. Enable the OpenCode CLI plugin

Install the bundled plugin's pinned dependencies:

```sh
cd "$bundle/plugins/opencode-sesh"
bun install --frozen-lockfile
pwd
```

Add the printed **absolute path** to the existing `plugins` array in
`~/.config/opencode/cli.json` (or `$XDG_CONFIG_HOME/opencode/cli.json`). Preserve
all other plugins and settings. Example entry, not a replacement config:

```json
{
  "plugins": [
    "/absolute/path/to/sesh-fork_<version>_darwin_arm64/plugins/opencode-sesh"
  ]
}
```

Do not put it in `opencode.json` as a server-only plugin. The local CLI owns the
tmux pane identity. **Restart each OpenCode terminal client inside tmux**, then
open a conversation. The same installed plugin supports tabs disabled.

## 5. Add a tmux shortcut

Print your absolute binary path:

```sh
printf '%s\n' "$bundle/sesh-fork"
```

Add these lines to your tmux config, replacing the example binary path with that
output. Choose a key that does not overwrite a shortcut you want to keep:

```tmux
set -g focus-events on
bind-key t display-popup -w 64% -h 70% -E "/absolute/path/to/bundle/sesh-fork picker --tree"
```

Reload the config you edited, e.g.:

```sh
tmux source-file ~/.config/tmux/tmux.conf
```

If your config lives at `~/.tmux.conf`, source that file instead. Use an absolute
binary path in bindings; tmux does not inherit shell aliases. Upstream `sesh`
remains untouched, although both binaries use the usual sesh config/cache paths.

## 6. Use and check it

Press your tmux prefix, then `t` (or your chosen key). One searchable list shows:

- OpenCode agents first: status symbol/word, `session > conversation`, and a
  right-aligned conversation directory underneath.
- Agent-free tmux sessions below: names only. No duplicate session rows for
  agent-containing sessions.

Tab/Shift-Tab or arrows select rows; Enter focuses the exact agent pane or the
session's active pane. Search matches titles, session names, status, and paths.
Ctrl-A/T/G/X/F selects all/tmux/config/zoxide/find. No previews or deletion.

Status: `○ idle`, `● working`, `! attention`, `◆ unread`, `✓ read`, or
`? unavailable`. Highlighting a row does **not** acknowledge completion.

### If an agent says unavailable

1. Confirm the tmux binding runs this bundle's `sesh-fork`, not upstream `sesh`.
2. Confirm the plugin's absolute path and successful `bun install`.
3. Restart the OpenCode **terminal client**, inside tmux, and open a conversation.
4. Look for fresh records under `$XDG_STATE_HOME/sesh/opencode`, or
   `~/.local/state/sesh/opencode` when that variable is unset. The client and picker
   must use the same state directory. Missing/invalid records or heartbeats older
   than five seconds are unavailable, never assumed idle.
5. Check OpenCode CLI plugin errors if records never appear. A missing path field
   displays `path unavailable`; the pane working directory is not substituted.

Permission/form requests and execution failures need attention. Completion
read/unread depends on actual pane visibility. Any open sesh picker temporarily
suppresses read acknowledgement server-wide; unrelated popups are not tracked.
Real completion/request/restart flows are not yet fully integration-verified.
See the bundled [plugin README](plugins/opencode-sesh/README.md) for details.

## Updates and removal

Download and verify a new release, extract beside the old bundle, install its
plugin dependencies, and update both absolute paths. Restart OpenCode clients and
reload tmux. Keep the old bundle until the new one works; do not mix a new binary
with an old plugin unintentionally.

To remove: remove only this plugin entry and your fork binding, restart clients,
and delete the bundle you installed. Do not remove shared sesh config/cache or
other plugin settings. Nothing in these steps installs or replaces upstream sesh.

## Maintainers: package without publishing

Requires Go 1.26+, GoReleaser v2, and Bun for plugin checks. From the repo root:

```sh
goreleaser check
goreleaser release --snapshot --skip=publish
```

Use a fresh output directory (default `dist/`); do not erase existing artifacts
you need. Inspect all four archives and `checksums.txt`, execute the native binary,
and test the extracted plugin rather than only the source checkout.

The release workflow runs on `v*` tags and creates a **draft release** in
`apaslak/sesh` using its repository `GITHUB_TOKEN`. It tests the Go code and plugin
before uploading. Homebrew/AUR publishing is disabled. Publishing requires a
separate deliberate tag/push and manual review/publication of the draft; local
snapshot commands above do not push tags or publish releases.
