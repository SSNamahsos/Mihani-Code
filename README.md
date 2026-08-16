# Mihani Code

**Mihani Code** is a terminal-based coding assistant specialized for **Go development**. Built with ❤️ by **Faz Pad Studio**.

```
███╗   ███╗ █████╗  ██████╗██████╗ ██████╗  ██████╗ 
████╗ ████║██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔═══██╗
██╔████╔██║███████║██║     ██████╔╝██████╔╝██║   ██║
██║╚██╔╝██║██╔══██║██║     ██╔══██╗██╔═══╝ ██║   ██║
██║ ╚═╝ ██║██║  ██║╚██████╗██║  ██║██║     ╚██████╔╝
╚═╝     ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝╚═╝      ╚═════╝ 
Mihani Code - Your Go Programming Assistant
Made by Faz Pad Studio
```

## Features

- **Interactive Chat/REPL Mode**: Ask coding questions and get AI-powered assistance
- **File Operations**: Read, write, and edit files directly from the terminal
- **Code Explanation**: Get detailed explanations of Go code
- **Code Refactoring**: Request refactoring suggestions with custom instructions
- **Code Generation**: Generate Go code from natural language descriptions
- **Project Scanning**: Scan directories to understand codebase structure
- **Code Snippets**: Access built-in Go code templates for common patterns
- **Command History**: Persistent history with search capabilities
- **Session Management**: Automatic session saving and restoration
- **Offline Mode**: Graceful degradation when no API is configured

## Installation

### Using `go install` (Recommended)

```bash
go install github.com/fazpadstudio/mihanicode/cmd/mihanicode@latest
```

The binary will be installed to `$GOPATH/bin/mihanicode`.

### Building from Source

```bash
git clone https://github.com/fazpadstudio/mihanicode.git
cd mihanicode
go build -o mihanicode ./cmd/mihanicode
```

## Usage

Start Mihani Code by running:

```bash
mihanicode
```

### Basic Commands

| Command | Description |
|---------|-------------|
| `/help` | Show all available commands |
| `/about` | Display information about Mihani Code |
| `/quit` | Exit the application |
| `/clear` | Clear conversation history |

### File Operations

| Command | Description |
|---------|-------------|
| `/read <file>` | View contents of a file |
| `/write <file>` | Create or overwrite a file |
| `/scan [dir]` | Scan directory for Go files |

### Code Assistance

| Command | Description |
|---------|-------------|
| `/explain <file>` | Explain code in a file |
| `/refactor <file> [instruction]` | Refactor code with specific instructions |
| `/generate <prompt>` | Generate Go code from description |

### Snippets

| Command | Description |
|---------|-------------|
| `/snippets [category]` | List available code snippets |
| `/snippet <name> [var=value...]` | Display a specific snippet |

### History & Configuration

| Command | Description |
|---------|-------------|
| `/history [query]` | Show command history, optionally search |
| `/config` | Show current configuration |
| `/status` | Show session status |

## Configuration

Mihani Code supports configuration through multiple methods:

### Environment Variables

```bash
export OPENAI_API_KEY="your-openai-api-key"
export ANTHROPIC_API_KEY="your-anthropic-api-key"
```

### Configuration File

Create a `~/.mihanirc` file:

```json
{
  "openai_api_key": "your-openai-api-key",
  "anthropic_api_key": "your-anthropic-api-key",
  "default_provider": "openai",
  "model": "gpt-4o-mini",
  "max_history": 1000,
  "enable_git_integration": true,
  "auto_save_session": true
}
```

Configuration file locations (checked in order):
1. `.mihanirc` in current directory
2. `~/.mihanirc` in home directory
3. `~/.config/mihanicode/config.json` (XDG config directory)

## Available Snippets

Mihani Code includes built-in Go code snippets:

| Name | Category | Description |
|------|----------|-------------|
| `main` | boilerplate | Standard main function with error handling |
| `http_server` | web | Basic HTTP server with graceful shutdown |
| `cli_app` | cli | Basic CLI application structure |
| `struct_json` | types | Struct with JSON tags and methods |
| `interface_repo` | patterns | Repository interface pattern |
| `test_function` | testing | Standard test function template |
| `goroutine_worker` | concurrency | Worker pool pattern with goroutines |
| `middleware_chain` | web | HTTP middleware chain |
| `error_handling` | errors | Custom error types and wrapping |
| `config_loader` | utility | Configuration loading from env and file |

### Using Snippets

List all snippets:
```
/snippets
```

List snippets by category:
```
/snippets web
```

Display a specific snippet:
```
/snippet main
```

Display a snippet with variable substitution:
```
/snippet struct_json NAME=User
```

## API Integration

Mihani Code supports multiple LLM providers:

### OpenAI

Set your OpenAI API key:
```bash
export OPENAI_API_KEY="sk-..."
```

Or in config:
```json
{
  "default_provider": "openai",
  "model": "gpt-4o-mini"
}
```

### Anthropic

Set your Anthropic API key:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

Or in config:
```json
{
  "default_provider": "anthropic",
  "model": "claude-sonnet-4-5-20250929"
}
```

### Offline Mode

When no API key is configured, Mihani Code runs in offline mode with reduced capabilities:
- File viewing and editing
- Code scanning
- Snippet templates
- Command history

## Examples

### Explaining Code

```
/explain main.go
```

### Refactoring Code

```
/refactor handler.go optimize for performance
```

### Generating Code

```
/generate a function that reads a JSON file and unmarshals it into a struct
```

### Reading Files

```
/read internal/chat/chat.go
```

### Scanning Project

```
/scan ./internal
```

## Keyboard Shortcuts

- `Ctrl+C`: Interrupt current operation / Exit application
- `Ctrl+D`: End of input (in multi-line mode)

## Project Structure

```
mihanicode/
├── cmd/
│   └── mihanicode/
│       └── main.go          # Application entry point
├── internal/
│   ├── chat/                # Chat/REPL functionality
│   ├── config/              # Configuration management
│   ├── fileops/             # File operations
│   ├── history/             | Command history
│   ├── llm/                 # LLM API clients
│   ├── scanner/             # Code scanning
│   └── snippets/            # Code snippet templates
├── go.mod
├── go.sum
└── README.md
```

## Error Handling

Mihani Code includes comprehensive error handling:
- Graceful API failure handling with fallback to offline mode
- File operation error reporting
- Input validation
- Session persistence with error recovery

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## License

This project is licensed under the MIT License.

## Credits

**Made by Faz Pad Studio**

Mihani Code is inspired by tools like GitHub Copilot CLI, Claude Code, and OpenCode, designed specifically for Go developers.