# Mihani Code

Mihani Code is a native Go terminal AI coding agent. It provides a focused workspace for asking a model to inspect, explain, and change the project you are currently in - in the spirit of claude code and opencode.

## Highlights

- Bubble Tea terminal UI with a block-based transcript, markdown rendering (glamour), and syntax-tinted diff previews
- Streaming OpenAI-compatible responses with parallel tool-call reassembly
- **Mihani-branded tools**: `Mihani_Read_File`, `Mihani_Write_File`, `Mihani_Edit_File`, `Mihani_Delete_File`, `Mihani_List_Dir`, `Mihani_Search_Files`, `Mihani_Grep` (regex matches as `path:line:`), `Mihani_Glob`, `Mihani_Bash`, `Mihani_Web_Search`, `Mihani_Web_Fetch`, `Mihani_Image_Reader`, `Mihani_Ask_User`, `Mihani_Todo_Write`
- **Mihani Mode (Shift+Tab)**: the no-interruptions toggle — tools run without permission prompts while the header shows a red ⚡ MIHANI pill
- **Persian / RTL support**: type and read فارسی correctly — letters shaped into joined forms and text reordered right-to-left, in the composer and the whole transcript
- **Prompt power-ups**: `!command` shell passthrough, `#note` project memory, `@file` content inlining, `ctrl+up`/`ctrl+down` prompt history
- Multi-turn tool execution loops: read, write, edit, delete (files or whole directories), search files plus shell commands
- **Interactive questions**: the model can pause mid-task and ask you a question - options appear as a menu you pick from, or you type a custom answer; it may ask several in a row
- **Live todo list**: the agent maintains a visible task card (`Mihani_Todo_Write`) that updates in place with ✓/◐/○ per item as work progresses
- Two built-in endpoints with curated default models; `/connect` discovers models from any OpenAI-compatible endpoint
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

**Staying current:** from v0.2.18 on, Mihani checks GitHub for a newer release on startup - a direct HTTPS call to `api.github.com`, no API key and zero model tokens. When one is available the home page and header flag it, and `/update` shows the new version's changelog ("what's new") and the source release, with `i` to download and install it in place, `o` to open it on GitHub, or `d` to hide the notice for the session. On Windows the in-place swap completes the moment you close the window.

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
| `ctrl+j` / `alt+enter` | newline inside the composer (long lines wrap upward) |
| `/` ... | type to filter the command palette |
| `tab` | cycle modes (navigate the palette when it is open) |
| `shift+tab` | toggle **Mihani Mode** — tools run without asking (also `/mihani`) |
| `↑` / `↓` / `pgup` / `pgdn` | scroll the transcript (arrows stay in the composer while it is multiline) |
| *select with mouse* | select text with your terminal's own drag selection (native, always works - it auto-copies in Windows Terminal) |
| `/mouse` + drag | in-app selection: click menus, drag-select with auto-copy, and selections that **extend across scrolling** (wheel or drag-to-edge grows them) |
| `/paste` | insert clipboard text into the composer without sending - the safe way to paste large prompts |
| `[` / `]` on a message | keyboard menu: Copy / Fork / Revert for that message |
| `ctrl+y` or `/copy` | copy Mihani's last reply to the clipboard - toast confirmation |
| `ctrl+up` / `ctrl+down` | walk previously submitted prompts (draft preserved) |
| `!command` | run a shell command in the workspace right now (`! git status`) |
| `#note` | append a project-memory line to `.mihani.md` |
| `@file` | inline a workspace file's contents into the prompt (`explain @main.go`) |
| `esc` | interrupt request (press **twice** to terminate); otherwise clear input / close overlays |
| `ctrl+c` | cancel request → deny pending approval → quit |

Launching opens a fresh **home page / new season**; switch to past conversations with `/seasons` (aliases `/resume`, `/sessions`). Mouse capture defaults **off** so your terminal's native drag selection always works (same as opencode). If you prefer click menus and in-app drag selection instead, run `/mouse` (toggles live and remembers), or set `"use_mouse": true` in `config.json`.

While a turn is running you can keep typing: additional prompts are queued and sent automatically when the turn completes.

## Slash commands

- `/help` keyboard shortcuts and commands
- `/init` analyze the project and write `.mihani.md` project instructions
- `/clear` clear the visible conversation
- `/new` start a fresh session
- `/resume` (aliases `/seasons`, `/sessions`) pick and restore a previous conversation in this folder
- `/mode [build|plan|research|ask]` show or set the mode
- `/mihani` toggle Mihani Mode — tools run without asking (Shift+Tab)
- `/providers`, `/models`, `/connect` provider management
- `/git status`, `/git diff` repository inspection
- `/status`, `/session` workspace/session details
- `/mcp` configured MCP servers
- `/undo` restore the latest pre-change snapshot
- `/compact` compact the conversation history now
- `/todos` show the current todo list
- `/rtl` toggle Persian/RTL display (shaping + bidi)
- `/settings` auto-confirm toggle and limits
- `/update` check for a newer Mihani Code and install it
- `/quit` exit

## Modes

Modes shape what the agent is allowed to do before tools ever run:

- **build** - make changes directly in the workspace
- **plan** - read-only exploration ending in an implementation plan
- **research** - investigate and compare; reads freely and can write deliverables (notes, reports, docs)
- **ask** - explanations only

Modes are independent of the provider: **the mode controls what the agent is allowed to do** (whether it may mutate files), and **you pick the provider/model separately with `/providers` / `/models` in any mode**. Both built-in endpoints run Mihani's full file/shell tool set — build mode works on Mihani Cloud and Mihani Pro alike.

Plan, Research, and Ask refuse mutating tools (`Mihani_Write_File`, `Mihani_Edit_File`, `Mihani_Delete_File`, `Mihani_Bash`) without asking the provider to retry them.

### Mihani Mode

Press **Shift+Tab** (or run `/mihani`) to arm Mihani Mode: dangerous tools run **without the permission modal** — approve-once prompts disappear, writes/deletes/shell just execute. The header pill becomes a red **⚡ MIHANI** badge and the composer border turns red while armed; the state is saved to `config.json` so it survives restarts. Modes still apply (plan/ask stay read-only — the model isn't even offered the write tools), and the tools a turn runs are unchanged otherwise. Shift+Tab again disarms it.

## Providers and default models

Mihani Code ships with two built-in backends presented under Mihani branding - endpoints stay private:

| Provider | Public label | Models |
| --- | --- | --- |
| `mihani` | Mihani Cloud | `DeepSeek-V4-Pro` *(default)*, `step-3.7-flash`, `glm-5.3-flash`, `sensenova-6.8-flash-lite`, `spark-x2.5` |
| `mihani-pro` | Mihani Pro | `claude-sonnet-5` *(default)*, `claude-haiku-4-5`, `gpt-5.6-luna` |

Switch with `/providers` and `/models`; `/connect` adds any other OpenAI-compatible endpoint under a name you choose. Upstream identifiers from earlier releases are renamed automatically on first launch and never shown in the UI.

### Persian / RTL

Mihani reads and writes Persian out of the box. Terminals without bidi support render Arabic-script text disconnected and reversed; Mihani fixes that on display — in the transcript, the composer, questions, menus, and diff previews. Typing stays natural: the composer shows your Persian ordered live (with the cursor in the right place) while the text sent to the model remains logical, and RTL lines are right-aligned so paragraphs anchor to the right edge.

Three display modes exist because terminals differ; `/rtl` cycles them (persisted as `"rtl_display"` in config.json):

- **`bidi` (default)** — reorders the text into visual order but keeps the standard Arabic codepoints, so your terminal's own text shaper joins the letters with the correct font and weight. Right for Windows Terminal and anything else that shapes but doesn't reorder.
- **`shaped`** — converts letters to their pre-joined presentation forms and reorders. For terminals that do neither (classic console, many Linux terminals).
- **`off`** — passes text through untouched. For terminals with full native bidi + shaping.

### Endpoints without native tool calling

Some gateways strip OpenAI's `tools` parameter, so models there never see file/shell tools. For those, set `"native_tools": false` on the provider (the second built-in endpoint ships this way) and Mihani drives tools through a text protocol instead: the tool catalog joins the system prompt, the model replies with `<tool_call>{...}</tool_call>` blocks, Mihani executes them locally and feeds back `<tool_result>` blocks until the task completes. This works with any chat-completions endpoint - tools become a property of Mihani, not of the API. Providers added via `/connect` that point at a known gateway are auto-detected and default to the text protocol, and `Mihani_Read_File` supports `offset`/`limit` line-paging so large files never hit a truncation wall.

Reasoning models are supported throughout: streamed `reasoning_content` (GLM/DeepSeek style) and Anthropic `thinking` deltas render in a dedicated dimmed thinking block above the answer.

## Cost tracking and daily budget

Every request reports real input/output tokens from the provider. Cost is computed from a per-model rate table (`internal/pricing/pricing.go`) and shown live in the status bar:

```
$0.42/$10.00 · ~/myproject · 12.4k tokens (6%) · ready
```

- Spend is tracked per provider over a rolling 24-hour window (`~/.mihani/usage.json`)
- **The cap applies only to the built-in Mihani endpoints** (`mihani`, `mihani-pro`) - those share your embedded credit. Providers added via `/connect` use their own credentials and are never capped or metered against this budget.
- When a built-in endpoint reaches its budget, new turns are refused until the oldest usage falls out of the window; the message tells you exactly when
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

## Bring your own key (auto-fallback)

Have your own API key from the provider's website? Store it once and Mihani uses it automatically whenever the shared daily credit runs out - no manual switching:

1. `/settings` → **Personal API key · Mihani Cloud** (or *Mihani Pro*) → `enter`
2. Paste your key, `enter` to save (`esc` cancels; submitting an empty value removes it)

From then on:

- While shared credit remains, requests use it as usual
- The moment the $10 cap trips, the next turn transparently switches to your personal key - a toast announces *"Shared limit reached - using your personal API key"* and the status meter turns cyan: `$X.XX personal`
- Personal usage has **no cap** (it is your own quota) and is tracked separately from the shared bucket
- Your key lives only in your local `config.json`, is never displayed again (shown masked as `configured ••••1234`), and is scrubbed from every tool output like all other secrets

## API key protection

Provider credentials ship inside the binary but are deliberately hard to extract:

- XOR-obfuscated in the data section - raw keys never appear via `strings mihani.exe`
- `config.json` cannot contain keys: the field is excluded from JSON marshal/unmarshal entirely
- No UI surface (overlays, settings, status, logs) ever displays a key
- Every tool result - including `bash` output, file reads, and MCP responses - passes through a redactor before it reaches the transcript or model history, so `cat ~/.mihani/config.json` or `echo $ENV` style probes return `[redacted]`

Keys are also never exported to environment variables of child processes.

> Note: a determined attacker with full control of their own machine can still reverse-engineer a distributed binary. This design prevents *casual* extraction through the app itself - for stronger guarantees, proxy requests through your own server that holds the key.

### Distributing binaries with embedded credentials

The credential blob never lives in git. To make release builds carry it:

1. Base64-encode your local blob:
   `[Convert]::ToBase64String([IO.File]::ReadAllBytes("internal/secrets/blob.bin")) | Set-Clipboard`
2. GitHub repo → **Settings → Secrets and variables → Actions** → new repository secret named `BLOB`
3. Push a version tag: `git tag v0.2.1; git push origin v0.2.1`

CI decodes the secret back to `internal/secrets/blob.bin`, builds all five platform binaries with credentials embedded, and attaches them to the release. Without the secret, releases still build - but as credential-free placeholders (users then connect their own provider via `/connect`).

Keep the repository public so the install one-liners and release downloads are reachable without authentication.

## Configuration

The config file lives at `~/.mihani/config.json` after your first change and supports extra fields such as `context_window`, `max_iterations`, `budget_usd`, and `pricing`.

**If characters show up as `?`:** on some terminals (commonly the Windows console with a basic font) the rounded box corners and the animated spinner render as `?` because the font lacks those glyphs. Set `"plain_ui": true` in config - or toggle **Plain UI (ASCII)** in `/settings` - to switch Mihani to plain ASCII borders and spinner. This only fixes Mihani's own chrome; non-Latin text (Persian/Arabic) still needs a font that contains those glyphs - run Mihani in **Windows Terminal** with a Unicode font such as **Cascadia Code** or **Noto Sans Mono** (add a fallback font with Arabic/Persian coverage in the font settings), and make sure the console is in UTF-8 (`chcp 65001`). Built-in provider credentials cannot be stored in or read from this file; custom providers added via `/connect` store their key locally in the provider's `personal_key` field (used for that endpoint's requests), or reference an environment variable through `env_key` if they prefer.

### Key-protecting gateway

To keep the built-in providers' upstream API keys out of the distributed binary, Mihani ships with a key-protecting Cloudflare Worker proxy. The worker holds the real keys server-side; the client only presents a token. **Works out of the box** — no setup required.

Want to self-host? See `gateway-worker/DEPLOY.md` (Cloudflare Worker) or `gateway/` (Go standalone for VPS/Fly.io).

`/pro/...` and `/cloud/...` forward to the Mihani Pro and Cloud upstreams respectively, streaming responses back verbatim. See `docs/gateway.md` for the design.

**Note:** distributed binaries never ship embedded keys. If you self-host a gateway, set `MIHANI_GATEWAY` to point at it.

## Skills and MCP

Skills are `SKILL.md` folders loaded from `.mihani/skills/<name>/` and `.agents/skills/<name>/` (per project) and `~/.mihani/skills/<name>/` and `~/.agents/skills/<name>/` (globally). The `.agents` location is where Claude Code / codex and most skill installers put them. Each skill's frontmatter `description` is read, and when a task matches it the AI opens that skill's `SKILL.md` and follows the instructions inside it. List what's installed with `/skills`.

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
