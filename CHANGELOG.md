# Changelog

All notable changes to Mihani Code are documented here.

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
