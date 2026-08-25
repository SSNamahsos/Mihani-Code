# Mihani Code

Mihani Code is a native Go terminal AI coding agent. It provides a focused workspace for asking a model to inspect, explain, and change the project you are currently in — in the spirit of claude code and opencode.

## Highlights

- Bubble Tea terminal UI with a block-based transcript, markdown rendering (glamour), and syntax-tinted diff previews
- Streaming OpenAI-compatible responses with parallel tool-call reassembly
- Multi-turn tool execution loops: read, write, edit, delete, search files plus shell commands
- Two built-in endpoints (`hcnsec`, `seekai`) with curated default models; `/connect` discovers models from any OpenAI-compatible endpoint
- **Live cost meter**: real input/output token accounting, per-model $ pricing, rolling 24h spend per provider
- **Daily budget enforcement**: turns are refused once a provider reaches its 24h cap (default $10)
- **Key protection**: API keys are never written to config.json, never rendered in any UI surface, XOR-obfuscated in the binary, and redacted from every tool result
- Build / Plan / Research / Ask modes with permission boundaries enforced before any model call
- Permission modal for dangerous tools with approve-once, always-allow, and deny
- Message queueing while a turn is running; Esc interrupts; Ctrl+C cancels or exits
- Multi-session support: automatic resume of your latest session, `/new`, `/resume` picker
- Automatic context compaction keeps long sessions inside provider limits
- Headless print mode (`-p`) for scripts and CI, with the same budget gate and spend reporting
- Pre-write snapshots under `.mihani/snapshots` with `/undo`
- Project and user skills via `SKILL.md`, MCP stdio servers via `.mihani/mcp.json`
- Cross-platform shell execution (`cmd /C` on Windows, bash/sh elsewhere) and CRLF-tolerant file editing

## Install

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/SSNamahsos/Mihani-Code/main/install.ps1 | iex
```

**Linux / macOS:**

```sh
curl -fsSL https://raw.githubusercontent.com/SSNamahsos/Mihani-Code/main/install.sh | sh
```

**With Go 1.24+ (any platform):**

```sh
go install github.com/SSNamahsos/Mihani-Code/cmd/mihani@latest
```

All three routes give you a `mihani` binary you run inside any project directory. Prebuilt binaries for Windows, Linux, and macOS are attached to every [GitHub Release](https://github.com/SSNamahsos/Mihani-Code/releases) automatically by CI.

## Build from source

Requires Go 1.24 or newer.

```sh
go build -o mihani ./cmd/mihani
```

On Windows:

```powershell
go build -o mihani.exe ./cmd/mihani
```

Run Mihani from the project you want it to work on:

```sh
./mihani
```

## Command line

```
mihani [flags] [initial prompt]

  -p              headless print mode: run one prompt, stream the answer, exit
  -c              continue the most recent session in this workspace
  -r <id>         resume a specific session id
  -model <name>   override the model for this run
  -provider <id>  override the provider for this run
  -y              auto-approve tool use (print mode only)
  -version        print version and exit
```

Examples:

```sh
mihani                                  # interactive TUI (auto-resumes your last session)
mihani "explain the config loader"      # open the TUI with a prefilled prompt
mihani -p "summarize this repo"         # headless one-shot for scripts/CI
mihani -p -y "run go test ./... and fix failures"
mihani --version
```

## Keys

| Key | Action |
| --- | --- |
| `enter` | send prompt (or insert the highlighted palette command) |
| `ctrl+j` / `alt+enter` | newline inside the composer |
| `/` … | type to filter the command palette |
| `tab` / `shift+tab` | cycle modes (navigate the palette when it is open) |
| `pgup` / `pgdn` | scroll the transcript |
| `esc` | interrupt the current request, clear input, close overlays |
| `ctrl+c` | cancel request → deny pending approval → quit |

While a turn is running you can keep typing: additional prompts are queued and sent automatically when the turn completes.

## Slash commands

- `/help` keyboard shortcuts and commands
- `/clear` clear the visible conversation
- `/new` start a fresh session
- `/resume` pick and restore a previous session
- `/mode [build|plan|research|ask]` show or set the mode
- `/providers`, `/models`, `/connect` provider management
- `/git status`, `/git diff` repository inspection
- `/status`, `/session` workspace/session details
- `/mcp` configured MCP servers
- `/undo` restore the latest pre-change snapshot
- `/settings` auto-confirm toggle and limits
- `/quit` exit

## Modes

Modes shape what the agent is allowed to do before tools ever run:

- **build** — make changes directly in the workspace
- **plan** — read-only exploration ending in an implementation plan
- **research** — read-only investigation and comparisons
- **ask** — explanations only

Plan, Research, and Ask refuse mutating tools (`write_file`, `edit_file`, `delete_file`, `bash`) without asking the provider to retry them.

## Providers and default models

Mihani Code ships with two built-in backends presented under Mihani branding — endpoints stay private:

| Provider | Public label | Models |
| --- | --- | --- |
| `mihani` | Mihani Cloud | `glm-5.2`, `glm-5.3` *(default)*, `kat-coder-pro-v2.5`, `MiniMax-M3`, `mimo-v2.5` *(free)* |
| `mihani-pro` | Mihani Pro | `claude-opus-5`, `claude-opus-4-8`, `claude-fable-5`, `claude-sonnet-5`, `gpt-5.6-sol`, `grok-4-5` |

Switch with `/providers` and `/models`; `/connect` adds any other OpenAI-compatible endpoint under a name you choose. Upstream identifiers from earlier releases are renamed automatically on first launch and never shown in the UI.

## Cost tracking and daily budget

Every request reports real input/output tokens from the provider. Cost is computed from a per-model rate table (`internal/pricing/pricing.go`) and shown live in the status bar:

```
$0.42/$10.00 · ~/myproject · 12.4k tokens (6%) · ready
```

- Spend is tracked per provider over a rolling 24-hour window (`~/.mihani/usage.json`)
- When a provider reaches its budget, new turns are refused until the oldest usage falls out of the window; the message tells you exactly when
- The cap defaults to `$10.00`; change or disable it in `config.json`:

```json
{ "budget_usd": 10.0 }
```

(`0` means "use the $10 default"; a negative number disables enforcement entirely.)

Rate estimates are editable per model pattern without recompiling:

```json
{
  "pricing": {
    "glm":        { "input": 0.60, "output": 2.50 },
    "mimo":       { "input": 0.00, "output": 0.00 },
    "claude-opus": { "input": 15.0, "output": 75.0 }
  }
}
```

`/settings → Reset usage window` clears today's spend record.

## API key protection

Provider credentials ship inside the binary but are deliberately hard to extract:

- XOR-obfuscated in the data section — raw keys never appear via `strings mihani.exe`
- `config.json` cannot contain keys: the field is excluded from JSON marshal/unmarshal entirely
- No UI surface (overlays, settings, status, logs) ever displays a key
- Every tool result — including `bash` output, file reads, and MCP responses — passes through a redactor before it reaches the transcript or model history, so `cat ~/.mihani/config.json` or `echo $ENV` style probes return `[redacted]`

Keys are also never exported to environment variables of child processes.

> Note: a determined attacker with full control of their own machine can still reverse-engineer a distributed binary. This design prevents *casual* extraction through the app itself — for stronger guarantees, proxy requests through your own server that holds the key.

### Distributing binaries with embedded credentials

The credential blob never lives in git. To make release builds carry it:

1. Base64-encode your local blob:
   `[Convert]::ToBase64String([IO.File]::ReadAllBytes("internal/secrets/blob.bin")) | Set-Clipboard`
2. GitHub repo → **Settings → Secrets and variables → Actions** → new repository secret named `BLOB`
3. Push a version tag: `git tag v0.2.1; git push origin v0.2.1`

CI decodes the secret back to `internal/secrets/blob.bin`, builds all five platform binaries with credentials embedded, and attaches them to the release. Without the secret, releases still build — but as credential-free placeholders (users then connect their own provider via `/connect`).

Keep the repository public so the install one-liners and release downloads are reachable without authentication.

## Configuration

The config file lives at `~/.mihani/config.json` after your first change and supports extra fields such as `context_window`, `max_iterations`, `budget_usd`, and `pricing`. Built-in provider credentials cannot be stored in or read from this file; custom providers added via `/connect` should reference an environment variable through `env_key` if they need a secret at startup.

## Skills and MCP

Skills live in `.mihani/skills/<name>/SKILL.md` per project or `~/.mihani/skills/<name>/SKILL.md` globally; their names and descriptions join the agent context automatically.

MCP stdio servers are declared in `.mihani/mcp.json`:

```json
{
  "servers": [
    { "name": "docs", "command": "npx", "args": ["-y", "some-mcp-server"] }
  ]
}
```

MCP processes start only when a turn needs them and are shut down when Mihani exits.

## Development

```sh
go test ./...
go vet ./...
gofmt -l .
go build ./cmd/mihani
```

## Architecture

```
cmd/mihani        CLI flags, version, entry point
internal/ui       bubbletea model, transcript blocks, overlays, budget meter
internal/agent    streaming loop, history compaction, cost emission, redaction hook
internal/pricing  per-model $/Mtok rates with config overrides
internal/usage    rolling-24h spend store (~/.mihani/usage.json)
internal/secrets  XOR-obfuscated embedded keys + universal redactor
internal/tools    sandboxed file/shell tools, previews, snapshots, undo
internal/session  multi-session persistence (~/.mihani/sessions)
internal/config   versioned config at ~/.mihani/config.json (key-free)
internal/mcp      safe discovery + stdio JSON-RPC client
internal/gitx     git helpers used by /git and the system prompt
internal/skills   SKILL.md discovery
```

MIT License. Made by Faz Pad Studio.
