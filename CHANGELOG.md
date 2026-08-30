# Changelog

All notable changes to Mihani Code are documented here.

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
