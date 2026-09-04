# Changelog

All notable changes to Mihani Code are documented here.

## v0.2.26

### Fixed
- **A malformed tool call silently stopped the turn.** Some models (seen on the Mihani Pro gateway) emit a tool call under a non-standard tag — e.g. `<Longcat_tool_call>…</ask_user>` — that Mihani can't parse as a tool call. Previously that ended the turn with a half-call shown to the user and no recovery. Now Mihani detects a botched tool call (an aliased `*_tool_call` tag, a `</toolname>` closing tag, or a block that was opened but not parsed) and nudges the model to resend it using the required `<tool_call>{…}` format — bounded so it never loops. The turn carries on instead of stopping.

### Changed
- **Research mode can now write.** Research previously was hard-locked read-only (and its prompt said “Do NOT modify files”), so asking it to “research and create files” silently failed. Research now reads freely **and** can create deliverables (notes, reports, docs, files); it still keeps research accurate and won't refactor or delete existing code. *Plan* and *Ask* remain read-only.

## v0.2.25

### Fixed
- **Reconnection did not kick in on timeouts / network drops.** A provider timeout (`context deadline exceeded` / `Client.Timeout`) and a raw network error were classified as *not* retriable, so the turn just failed with a red error instead of reconnecting. Provider timeouts and network errors are now retried through the reconnect loop (you'll see `reconnecting n/10 · retry in …s`), while a deliberate user cancel (Esc) still stops immediately.
- **Errors leaked the provider endpoint.** A failed request used to dump the raw Go error — e.g. `Post "https://api.…/v1/chat/completions": context deadline exceeded (Client.Timeout …)` — exposing the provider's URL/host. Error messages are now provider-neutral: a timeout shows “The model took too long to respond…”, a network drop shows “Could not reach the model (network error)…”, and after the reconnect retries are exhausted you get a clean explanation of the dropped connection and token usage — none of it names the provider or shows the link.

## v0.2.24

### Fixed
- **The auto-reopen window came up dark/blank.** The updater's relaunch used the `CREATE_NEW_CONSOLE` flag, which (when the parent console process is still winding down) hands the new process the dying console's input/output — so the new window stayed blank and the real output went to the old window. The relaunch now uses `start`, which asks the console host for a brand-new console with fresh input/output, so the reopened window actually renders. The in-place binary swap itself was already working and is unchanged.

### Notes
- After updating, the original window returns to the prompt and a **new** window opens with the updated Mihani — just close the old one. This v0.2.24 still needs to be installed manually once (the updaters before it are the broken ones); from v0.2.24 on, `/update` → `i` works end to end.

## v0.2.23

### Changed
- **Modes no longer force a provider.** The v0.2.21 mode→provider routing was too rigid — switching to build mode kicked you off your chosen provider. Now the **mode controls behavior** (whether the agent may mutate files) and the **provider is your free choice in any mode**: `tab`/`shift+tab`/`/mode` switch the mode, `/providers` and `/models` set the backend, and they don't fight each other. Each provider still remembers its own model via `/models`.
- Note: the provider determines whether the model can actually use file/shell tools. Mihani Cloud (DeepSeek) can; on the Mihani Pro gateway the models only expose that gateway's own tools, so file edits work best in **build** mode on Mihani Cloud, and Mihani Pro is best for reading/explaining/planning.

## v0.2.22

### Fixed
- **Windows self-update did nothing.** The previous updater spawned a hidden background PowerShell helper to swap the binary after the process exited, but that helper was unreliable (often killed or never ran), so the app said "reopening" and then never actually changed. The updater now does the swap **in-process** — a running Windows `.exe` is opened with `FILE_SHARE_DELETE`, so it can be renamed in place: the current binary is moved aside, the new one is installed in its place, the old file is deleted, and a fresh copy is opened in its own window. No background process, nothing to be killed. A leftover `.old` file from an interrupted update is cleaned up on the next launch.

### Notes
- This is the update that must be installed **manually once**: the in-app updater running inside v0.2.20/v0.2.21 is the old, broken one, so it cannot fix itself. Run the installer (or download the release) to get v0.2.22; from then on `/update` → `i` closes and reopens Mihani on the new version automatically.

## v0.2.21

### Added
- **Mode → provider routing.** Switching modes now binds the right backend automatically, because the two providers are genuinely different: **build** runs on **Mihani Cloud** (its model can actually receive and use the file/shell tools), while **plan / research / ask** run on **Mihani Pro** (Claude for reading and discussion). `tab` / `shift+tab`, `/mode`, and `/providers` all stay in sync (two-way): picking a provider snaps the mode to it, and switching modes picks the provider that mode is bound to. Each provider remembers its own model via `/models`, so coming back to a mode restores the model you had chosen there. A custom provider you picked by hand (e.g. Ollama) is never yanked by a mode change.

### Fixed
- **Misleading tool-availability behavior.** The agent prompt previously told the model to *never* say a tool was unavailable. On a gateway that does not forward Mihani's file/shell tools (verified on the Mihani Pro gateway, which injects its own web-search/image tools and drops the file tools for its models), that instruction made the model fight its own honest answer. The prompt now tells Mihani to be strictly honest: never claim a read/write/shell happened unless that tool actually returned a result this session, and if a tool is genuinely absent from the session, say so plainly and offer the best alternative instead of pretending.

### Notes
- File/shell tool access depends on the provider/gateway. Mihani Cloud (DeepSeek-V4-Pro) is verified to run file tools. On the Mihani Pro gateway the Claude models only expose the gateway's own tools, so keep code-changing work in **build** mode (Mihani Cloud) and use **ask/plan/research** on Mihani Pro for reading and discussion.
- If a provider answers "I have no tools," that is a gateway/key limitation, not a Mihani bug.

## v0.2.20

### Fixed
- **Windows self-update didn't actually apply.** A deferred cleanup deleted the downloaded binary the moment the download returned — before the deferred Windows swap (which runs only after the process exits, since a running .exe is locked) could move it into place. So after a restart Mihani was still on the old version. The cleanup now only happens on a failed download; the swap consumes the file.
- **Windows update now swaps reliably and restarts itself.** The replace helper waits for Mihani to fully exit, retries the file swap for a few seconds (so the file handle has time to release), then relaunches the new binary in a fresh window. After you pick **install**, the app closes and reopens on its own on the updated version — no more manual close-and-rerun.

### Notes
- If the reopened window still shows the old version, either a file lock / antivirus blocked the replace, or you have more than one `mihani` on your PATH. Run `where mihani`, and delete the stale copy so both the in-app updater and the `mihani` command point at the same file.

## v0.2.19

### Added
- **`/skills` command** — lists every installed skill (name, description, path).
- **Skills are now actually used by the AI.** Skills are discovered from `.mihani/skills/` and `.agents/skills/` (per project) and `~/.mihani/skills/` and `~/.agents/skills/` (globally). `.agents/skills` is where Claude Code / codex and most skill installers put them. The `description` field from each skill's YAML frontmatter is parsed (previously only the first line was read, which was the `---` fence and therefore useless), and the full `SKILL.md` path is passed to the model with an instruction to open it and follow it when a task matches.
- **Plain UI (ASCII) mode** — for terminals whose font can't render the rounded box corners or the animated braille spinner (they show up as `?`). Toggle **Plain UI (ASCII)** in `/settings`, or set `"plain_ui": true` in config, to switch to plain ASCII borders and spinner. Only Mihani's own chrome is affected; non-Latin text still needs a font with those glyphs.

### Fixed
- **`/update` didn't show the full changelog when you were already current.** It only fetched the detailed changelog when a *newer* release existed; otherwise it showed the sparse auto-generated GitHub release body (just a compare link). Now the full changelog for the version is always fetched from `CHANGELOG.md` and is shown in a scrollable panel (↑↓ / pgup / pgdn), with the source release link.

### Removed
- **"Reset usage window" from `/settings`** — the option and its handler are gone.

### Notes
- If Persian/Arabic (or the corners/spinner) still render as `?`, run Mihani in **Windows Terminal** with a Unicode font such as Cascadia Code or Noto Sans Mono (add a fallback font with Arabic/Persian coverage in the font settings), and keep the console in UTF-8 (`chcp 65001`). `plain_ui` fixes the corners/spinner but cannot substitute for a font that lacks the glyphs themselves.

## v0.2.18

### Added
- **In-app update check + self-update.** On startup Mihani checks GitHub for a newer release (a plain HTTPS call to `api.github.com` — no model call, zero tokens). When one exists, the home page and the header show a marker, and `/update` opens a panel with the full changelog for the new version ("what's new"), the source (the GitHub release), and one-key actions: `i` downloads the new binary and installs it over the current one (on Windows the swap happens automatically when you close the window), `o` opens the release on GitHub, `d` hides the notice for this session.
- **`/update` command** — manually re-check for a newer version and open the update panel.

### Changed
- **Mihani Cloud model lineup replaced.** Now ships `DeepSeek-V4-Pro` (default), `Qwen3.8-27B`, `step-3.7-flash`, `sensenova-6.8-flash-lite`, and `MiniMax-M3`. The previous `glm-5.2`, `glm-5.3`, `kat-coder-pro-v2.5`, and `mimo-v2.5` models are removed.
- **Mihani Pro trimmed to the Claude family.** `gpt-5.6-sol` and `grok-4-5` are removed; `claude-opus-5`, `claude-opus-4-8`, `claude-fable-5`, and `claude-sonnet-5` remain.

### Notes
- If your saved active model was one of the removed models, Mihani resets it to that provider's first model automatically on the next launch — nothing to fix by hand.
- This is the first release to include the update feature, so it will not yet notify users still on earlier versions (they do not have the checker yet). Install v0.2.18 once — via the installer script or manually — and every release after it is detected automatically in-app.

## v0.2.17

### Fixed
- **Reconnect counter continued across failure episodes.** After the AI kept working following a reconnect and the connection dropped again, the counter kept climbing (e.g. 8/10, 9/10) instead of restarting. It now counts consecutive failures since the provider last produced real output — after live progress the counter restarts from 1/10. The per-turn attempt cap of 10 is unchanged. (The optimistic "thinking" marker sent before every request does not count as progress; only streamed text/reasoning/tool calls do.)
- **Silent token usage spike after a failed turn.** Every reconnect retry re-sends the prompt, so a failed turn could burn tokens while the UI only showed a red error. The error block now states how many times the provider was retried, e.g. `(retried 9 times with growing delay - the provider kept failing, so token usage went up while reconnecting)`.

### Notes
- This release also ships all v0.2.16 fixes (message-menu/selection mouse deadlock, long-paste handling, native tools on Mihani Pro, token-limit continuation) — v0.2.16 was never published.

## v0.2.16

### Fixed
- **The AI "having no tools" on Mihani Pro.** The gateway now supports real OpenAI function calling (verified by probe: streamed `tool_calls` with `finish_reason: tool_calls`), so Mihani Pro and any seekai-host provider send native tools instead of the fragile text-based tool protocol. Opus models frequently refused the text protocol and answered in prose ("I can't write files here") — that failure mode is gone. Older configs' stale `native_tools: false` is upgraded automatically on load.
- **Turns dying without continuing when the reply hit the output token limit.** A response cut off mid tool-call (truncated JSON arguments) or an empty stream at `finish_reason: length` now continues the same turn with an in-conversation nudge to split work into smaller chunks (budget: 3 nudges per turn) instead of surfacing a dead error. Default max output tokens raised 8192 → 16384 so long single-file writes stop getting cut off.
- **Provider credit exhaustion was a silent-ish hard stop.** 402 / credit / balance / quota errors are now recognized and surfaced with the exact fix: reset the usage window in /settings, add a personal key, or switch provider. They are never retried.
- **Text selection deadlocked after clicking a message.** While the message menu was open, every mouse event was dropped — after one click the user could not drag-select any text until pressing esc. Clicking a menu item row now selects it, clicking anywhere else closes the menu, and a click outside the menu box closes it AND immediately arms a drag selection.
- **Long pastes were split and half-sent.** Bracketed paste (one clipboard block) is now inserted into the composer as one multiline message with a "pasted N lines" indicator and never auto-sent. For terminals without bracketed paste, a fast paste burst ending in Enter on a multiline composer keeps the newline instead of submitting the first half.

### Added
- **`web_search` tool** — web search results (title, URL, snippet) without any API key, so the agent can find sources and image URLs on its own.
- **`web_fetch` tool** — fetch any URL as readable text, or pass `save_to` to download raw bytes (images, assets) into the workspace, e.g. for a website's photos.
- **`glob` tool** — find files by pattern (`**/*.go`, `*.png`) with vendored directories skipped.
- **`bash` timeout parameter** — long-running commands (downloads, builds) can pass `timeout` up to 300s instead of being cut at 60s.

## v0.2.15

### Added
- **Live effort state while the model is reasoning.** The status line now shows `thinking · effort:off` (provider default) or `thinking · effort:high` etc. for every model that exposes levels — non-reasoning models show no effort state at all.
- **`ctrl+r` — quick effort toggle.** Cycles the active model through the levels it exposes (off → low → medium → high → off) without opening any menu; works while composing or mid-turn (applies from the next request onward), and refuses with a toast for models that have no effort levels. Documented in `/help`.

### Notes
- Effort support is detected from the model name family (same approach Claude Code and opencode take — OpenAI-compatible gateways expose no capability API). Effort levels are the real OpenAI `reasoning_effort` values; there is no "extra high" in the API, so the top level is `high`.

## v0.2.14

### Added
- **Automatic reconnect.** When a turn fails with a retriable provider error (network drop/timeout, HTTP 5xx, 429, 408), Mihani now retries with a growing backoff — 5s, 10s, 15s, then 30s and beyond — up to 10 attempts, showing progress in the status line like `reconnecting 3/10 · retry in 15s`. After the 10th failure the underlying error is shown in red. Non-retriable failures (bad key 401/403, bad request 400, user interrupt) fail immediately — a bad key is never retried ten times. Failed attempts are rolled back in history so the prompt is never sent twice.
- **`/effort` — per-model reasoning effort.** `/effort` opens a menu of the levels the active model actually exposes (`none / low / medium / high` for reasoning models, only `none` for plain models — no fake options); `/effort high` sets it directly. The level is stored per model, sent to the provider as `reasoning_effort` (omitted entirely when unset), and shown next to the model name in the header, e.g. `Mihani Pro · claude-opus-5 · effort:high`.

## v0.2.13

### Fixed
- **Provider and API key data loss with multiple windows open.** Every config save wrote the whole file from memory, so a stale instance (started before `/connect`, or a second open window) could silently delete recently added providers and their keys on its next save. Saves now merge: providers that exist on disk but not in memory are preserved; in-memory values win on conflict; migration-driven deletions still win.

## v0.2.12

### Fixed
- **Custom providers that answered with a web page instead of a model stream.** Base URLs pasted without the `/v1` suffix (e.g. `https://seekai.cc`) now get normalized for both model discovery and chat requests, so a bare domain no longer produces an instant silent "cancelled" turn.
- **API keys entered in `/connect` were lost on restart.** Keys now persist in the provider's `personal_key` field and are resolved by `Key()` after a save/reload cycle. They are also scrubbed from tool output like every other secret.
- **HTML responses are no longer swallowed.** If a provider answers `chat/completions` with 200 + `text/html` (gateway web app), the turn now fails with an explicit base-URL error instead of an empty reply.
- Up/down arrow keys navigate the `/` command palette (previously the keys were dropped into the text input and only Tab worked).
- Command palette: Enter no longer crashes when typing narrows the list under a stale highlight.
- Message menu readability, real revert semantics, and a click dead-zone for Windows Terminal's spurious 1-cell motion events.

### Added
- **`/refresh`** — re-fetches the model lists of all custom (user-added) providers, saves them, and reports per-provider results. Stale active models reset to the first fresh one.
- **Live local model discovery on startup.** When the active provider points at `localhost`/`127.0.0.1` (e.g. Ollama), the app pulls the models the server actually has instead of trusting a stored (possibly fake) list. A stopped server keeps the stored list.
- Visible todo list (`todo_write`) rendered as an in-place updating card.
- `ask_user` question menus with answer delivery back into the agent loop.
- `mihani-probe` diagnostic input probe and `MIHANI_DEBUG` mouse logging.

### Removed
- Legacy built-in providers `openai`, `openrouter`, and `anthropic` are dropped from saved configs on load; a selection pointing at one falls back to the Mihani Cloud built-in.

### Notes
- If a custom provider was added before this release, run `/connect` once more with the same name and key (old keys were not persisted), or `/refresh` afterwards to re-list models.
- Ollama lists only models that are actually pulled (`ollama pull <model>`); an empty local installation keeps its stored list rather than blanking out.

## v0.2.11
- Selection-first by default, home page = new season, `/seasons` for past work.
