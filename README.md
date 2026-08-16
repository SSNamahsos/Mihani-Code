# Mihani Code

**Your Agentic Terminal Coding Assistant**

Made by Faz Pad Studio

## Overview

Mihani Code is an AI-powered coding agent that operates directly in your terminal. Unlike simple chat-based assistants, Mihani Code can actively inspect, modify, and operate on your software projects using a comprehensive set of tools.

When you give Mihani Code a task, it will:
1. Understand your project structure
2. Read relevant files
3. Make necessary changes
4. Run tests or validation commands
5. Fix any issues discovered
6. Provide a summary of what was done

## Installation

### From Source

```bash
git clone https://github.com/SSNamahsos/Mihani-Code.git
cd Mihani-Code
go install ./...
```

### Requirements

- Go 1.19 or later
- API key for your preferred LLM provider

## Configuration

Mihani Code supports multiple LLM providers through environment variables or a configuration file.

### Environment Variables

```bash
export MIHANI_API_KEY="your-api-key"
export MIHANI_BASE_URL="https://api.openai.com/v1"  # Optional, for OpenAI-compatible APIs
export MIHANI_MODEL="gpt-4o"  # Optional
export MIHANI_PROVIDER="openai-compatible"  # or "anthropic"
```

### Configuration File

Create `~/.mihani/.mihanirc`:

```json
{
  "provider": "openai-compatible",
  "base_url": "https://api.openai.com/v1",
  "api_key": "your-api-key",
  "model": "gpt-4o",
  "max_iterations": 50,
  "command_timeout": 120,
  "auto_approve": false,
  "max_tool_calls": 100,
  "max_file_size": 100000
}
```

## Usage

### Interactive Mode

```bash
mihanicode
```

Then type your task in natural language:

```
> Fix the authentication bug in this project and run the tests
```

### One-Shot Mode

```bash
mihanicode "Add a dark mode to this application"
mihanicode "Find and fix the failing tests"
mihanicode "Review the authentication implementation"
```

### Available Commands (Interactive Mode)

- `/help` - Show available commands
- `/status` - Show current configuration
- `/exit` or `/quit` - Exit Mihani Code

## Features

### Tools

Mihani Code has access to these tools:

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Create or overwrite files |
| `edit_file` | Make targeted edits to files |
| `delete_file` | Remove files |
| `list_directory` | List directory contents |
| `find_files` | Find files by pattern |
| `search_code` | Search for text in code |
| `execute_command` | Run shell commands |
| `git_status` | Check git repository status |
| `git_diff` | View git diff |
| `git_log` | View commit history |

### Supported Providers

- **OpenAI** - GPT-4, GPT-4o, GPT-3.5-turbo
- **OpenAI-Compatible** - Any API following OpenAI's format (self-hosted, etc.)
- **Anthropic** - Claude 3 models

### Project Detection

Mihani Code automatically detects project types based on:
- `go.mod` - Go projects
- `package.json` - Node.js/JavaScript projects
- `pyproject.toml`, `requirements.txt` - Python projects
- `Cargo.toml` - Rust projects
- `pom.xml`, `build.gradle` - Java projects
- And more...

## Examples

### Fix a Bug

```
> The login form crashes when submitting with empty fields. Find and fix the issue.
```

Mihani Code will:
1. Search for login-related code
2. Read the relevant files
3. Identify the crash cause
4. Apply a fix
5. Test the fix

### Add a Feature

```
> Add a REST endpoint for user profiles with GET and PUT methods
```

Mihani Code will:
1. Explore the existing API structure
2. Create or modify route handlers
3. Add necessary models/types
4. Update documentation if present

### Refactor Code

```
> Refactor the database connection handling to use a connection pool
```

### Run Tests and Fix Failures

```
> Run all tests and fix any that fail
```

## Architecture

```
cmd/mihanicode/       - CLI entry point
internal/
  agent/              - Agent orchestration loop
  tools/              - Tool registry and implementations
  llm/                - LLM interfaces and types
  providers/          - LLM provider implementations
  filesystem/         - File operations
  shell/              - Command execution
  git/                - Git operations
  config/             - Configuration management
```

## Safety

Mihani Code includes safety features:
- Configurable command timeouts
- Maximum iteration limits to prevent infinite loops
- File size limits to prevent reading huge files
- Auto-approve mode can be disabled for sensitive operations

## License

See LICENSE file for details.

## Support

For issues and feature requests, please open an issue on GitHub.

---

Made by Faz Pad Studio
