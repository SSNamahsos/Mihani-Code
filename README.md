# Mihani Code

**Agentic Terminal Coding Assistant**

Made by Faz Pad Studio

## Overview

Mihani Code is a powerful AI-powered coding agent that runs in your terminal. Unlike simple chat-based assistants, Mihani Code actively works on your codebase - inspecting files, making edits, running commands, and verifying results autonomously.

## Features

### 🤖 True Agentic Behavior
- **Autonomous task execution**: Describe what you want, and Mihani figures out how to do it
- **Tool-based operations**: Real file editing, command execution, and git operations
- **Iterative problem solving**: Runs tests, analyzes failures, and fixes issues automatically
- **Project awareness**: Understands your project structure and conventions

### 🛠️ Powerful Tools
- **File Operations**: Read, write, edit, and delete files with precision
- **Code Search**: Grep and glob patterns to find code quickly
- **Shell Commands**: Run any terminal command in your project directory
- **Git Integration**: Status, diff, and log operations
- **Permission System**: Configurable approval for sensitive operations

### 🎯 Agent Modes
- **Build Mode**: Full capabilities - inspect, edit, run commands, test
- **Plan Mode**: Analyze and plan without making changes

### 🔧 Flexible Configuration
- Multiple LLM providers (OpenAI, Anthropic, OpenAI-compatible APIs)
- Custom base URLs for self-hosted models
- Granular permission controls
- Session management

## Installation

### From GitHub

```bash
go install github.com/SSNamahsos/Mihani-Code/cmd/mihanicode@latest
```

### From Source

```bash
git clone https://github.com/SSNamahsos/Mihani-Code.git
cd Mihani-Code
go build -o mihanicode ./cmd/mihanicode
```

## Quick Start

### Interactive Mode

```bash
# Start in current directory
mihanicode

# Start in specific project
mihanicode /path/to/my/project
```

### One-Shot Mode

```bash
# Execute a single task
mihanicode "fix the authentication bug"
mihanicode "add user registration endpoint"
mihanicode "why are the tests failing?"
mihanicode "refactor the database layer"
```

## Configuration

### Environment Variables

```bash
# Required: Your API key
export MIHANI_API_KEY="your-api-key-here"

# Optional: Provider type (default: openai-compatible)
export MIHANI_PROVIDER="openai"

# Optional: Base URL for OpenAI-compatible APIs
export MIHANI_BASE_URL="https://api.openai.com/v1"

# Optional: Default model
export MIHANI_MODEL="gpt-4o"

# Optional: Enable debug mode
export MIHANI_DEBUG=1
```

### Configuration File

Create `~/.mihani/config.json`:

```json
{
  "provider": {
    "provider": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "api_key": "your-api-key",
    "model": "gpt-4o"
  },
  "permissions": {
    "auto_approve": false,
    "read": [],
    "write": [],
    "shell": [],
    "delete": "deny"
  },
  "limits": {
    "max_iterations": 50,
    "max_tool_calls": 100,
    "command_timeout": 120,
    "max_file_read_size": 100000
  },
  "tui": {
    "theme": "default",
    "show_tool_calls": true,
    "compact_mode": false
  }
}
```

### Project-Specific Configuration

Create `.mihani/config.json` in your project root for project-specific settings.

## Usage Examples

### Debugging

```bash
mihanicode "find and fix the bug causing the crash on startup"
```

Mihani will:
1. Inspect the project structure
2. Search for relevant code
3. Read and analyze files
4. Identify the issue
5. Apply fixes
6. Run tests to verify

### Feature Implementation

```bash
mihanicode "add a login endpoint with JWT authentication"
```

Mihani will:
1. Examine existing auth patterns
2. Create necessary files
3. Implement the feature
4. Add tests
5. Run validation

### Code Review

```bash
mihanicode "review the authentication implementation for security issues"
```

### Refactoring

```bash
mihanicode "refactor the database layer to use connection pooling"
```

## Interactive Commands

While in interactive mode, use these commands:

| Command | Description |
|---------|-------------|
| `/help`, `/h` | Show available commands |
| `/clear`, `/c` | Clear conversation history |
| `/exit`, `/quit`, `/q` | Exit Mihani Code |
| `/model <name>` | Change or show current model |
| `/agent <mode>` | Change agent mode (build/plan) |
| `/status` | Show current status |
| `/tasks` | Show task list |
| `/compact` | Compact conversation history |

## CLI Options

```
mihanicode [options] [directory|task]

Options:
  --version, -v      Show version information
  --help, -h         Show help message
  --model <name>     Specify AI model
  --config <path>    Config file path
  --session <id>     Resume session
  --continue, -c     Continue last session
  --auto             Auto-approve operations
  --debug            Enable debug logging
```

## Supported Providers

### OpenAI
```bash
export MIHANI_PROVIDER="openai"
export MIHANI_API_KEY="sk-..."
export MIHANI_MODEL="gpt-4o"
```

### OpenAI-Compatible APIs
```bash
export MIHANI_PROVIDER="openai-compatible"
export MIHANI_BASE_URL="https://your-api.com/v1"
export MIHANI_API_KEY="your-key"
export MIHANI_MODEL="model-name"
```

### Anthropic
```bash
export MIHANI_PROVIDER="anthropic"
export MIHANI_API_KEY="sk-ant-..."
export MIHANI_MODEL="claude-3-5-sonnet-20241022"
```

## Permission System

Mihani Code includes a robust permission system for safety:

### Permission Levels
- **allow**: Operation proceeds automatically
- **ask**: User approval required
- **deny**: Operation blocked

### Configuring Permissions

```json
{
  "permissions": {
    "read": [
      {"pattern": "*.env", "level": "ask"}
    ],
    "write": [
      {"pattern": "src/**", "level": "ask"}
    ],
    "shell": [
      {"pattern": "git status*", "level": "allow"},
      {"pattern": "git diff*", "level": "allow"},
      {"pattern": "rm -rf*", "level": "deny"}
    ],
    "delete": "deny",
    "auto_approve": false
  }
}
```

## Project Detection

Mihani automatically detects project types:
- Go (`go.mod`)
- Node.js (`package.json`)
- Python (`requirements.txt`, `pyproject.toml`)
- Rust (`Cargo.toml`)
- Java (`pom.xml`, `build.gradle`)
- And more...

Based on detection, it chooses appropriate build/test commands.

## Architecture

```
cmd/mihanicode/       # Main entry point
internal/
  agent/              # Agent orchestration
  tools/              # Tool implementations
  providers/          # LLM provider adapters
  config/             # Configuration management
  tui/                # Terminal UI
  session/            # Session persistence
  filesystem/         # File operations
  shell/              # Command execution
  git/                # Git operations
```

## Safety Features

- **Path Security**: Prevents access outside project directory
- **Permission Checks**: Configurable approval for operations
- **Iteration Limits**: Prevents infinite loops
- **Command Timeouts**: Prevents hanging processes
- **No Auto-Commit**: Changes require explicit git commands

## Troubleshooting

### No API Key Error
```bash
export MIHANI_API_KEY="your-key"
```

### Model Not Found
Check that your model name is correct and available from your provider.

### Permission Denied
Review your permission configuration or use `--auto` flag (with caution).

### Command Timeout
Increase timeout in config:
```json
{"limits": {"command_timeout": 300}}
```

## Contributing

Contributions welcome! Please read our contributing guidelines before submitting PRs.

## License

MIT License - see LICENSE file for details.

## Support

- GitHub Issues: https://github.com/SSNamahsos/Mihani-Code/issues
- Documentation: https://github.com/SSNamahsos/Mihani-Code/wiki

---

**Mihani Code** - Made by Faz Pad Studio
