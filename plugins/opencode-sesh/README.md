# OpenCode V2 → sesh status bridge

For a prebuilt fork binary plus this plugin, start with
[the fork setup guide](../../FORK_SETUP.md). Source-checkout development/setup is
described below.

CLI-only plugin for `sesh picker --tree`. Requires OpenCode **2.0.26** or later
with compatible public CLI APIs, tmux, and focus events enabled:

```tmux
set -g focus-events on
```

From this directory:

```sh
bun install --frozen-lockfile
bun test
bun run typecheck
```

Add this directory's absolute path to `plugins` in
`~/.config/opencode/cli.json`, preserving other entries/settings:

```json
{
  "plugins": ["/absolute/path/to/sesh/plugins/opencode-sesh"]
}
```

Restart each OpenCode terminal client. Do not install this as a server-only
plugin: OpenCode's shared service does not own a tmux pane.

The plugin observes the displayed conversation (no tab rows), execution outcome,
and pending permissions/forms in its conversation family. Metadata includes the
conversation's `location.directory`, not the pane's working directory. Restart
existing clients after updating the plugin to publish that optional field;
older snapshots remain readable but display `path unavailable`. It writes private,
atomic, versioned metadata under `$XDG_STATE_HOME/sesh/opencode` (default
`~/.local/state/sesh/opencode`), refreshed every second. No prompts, messages,
permission contents or errors are copied into records. Owner-specific files
prevent a reloading instance from deleting its replacement's status.

- Idle: no execution/completion; interrupted work returns here.
- Working: execution underway, including retries.
- Needs attention: permission, form/question, or failed execution. Focus does
  not dismiss failure; the next execution does.
- Finished · unread/read: successful outcome and whether it was viewed.

Read means the displayed conversation's pane is visible to a focused attached
tmux client. External navigation counts too. The sesh tree picker holds a
short-lived lease preventing popup-covered completions from being marked read.
Other tmux popups are not tracked by that lease; terminal focus events may cover
them, but do not rely on that. Multiple clients are conservative: an open sesh
picker on this server temporarily prevents acknowledgement in all clients.

Disconnected, stale (>5 seconds), invalid, or absent records are unavailable,
never guessed idle. Observations have a three-second deadline; cleanup cancels
pending requests, and late responses cannot publish or acknowledge completion.
Unknown focus does not count as viewed. A successful
completion is recovered from V2's explicit outcome and idle revision after
reconnect; acknowledgement is persisted in CLI plugin storage.

This targets local OpenCode terminals inside local tmux. A remote OpenCode
service is accessed through the terminal's configured client; remote tmux and
headless OpenCode instances are not discovered. Outside tmux the plugin is inert.

Development dependencies are local to this package; Go builds need none of them.
