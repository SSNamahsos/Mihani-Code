# Mihani Code

**Your Go Programming Assistant in the Terminal**

*Made by Faz Pad Studio*

Mihani Code is a powerful terminal-based coding assistant specialized for **Go development**. It provides AI-powered code explanations, refactoring suggestions, and code generation capabilities, along with offline tools for file operations and codebase analysis.

## ✨ Features

### 🤖 AI-Powered Assistance
- **Interactive Chat/REPL** - Ask coding questions and get instant answers
- **Code Explanation** - Understand complex Go code with clear explanations
- **Code Refactoring** - Get suggestions to improve your Go code
- **Code Generation** - Generate Go code from natural language descriptions
- **Code Review** - Find issues and improvements in your code
- **Debug Helper** - Troubleshoot problems with detailed guidance
- **Test Generation** - Automatically create unit tests for your code

### 📁 File Operations
- Read and view file contents directly in the terminal
- Create and edit files with multi-line input mode
- Scan directories to understand codebase structure

### 🧩 Snippet Library
Built-in Go code templates including:
- `main` - Basic main function with error handling
- `http_server` - HTTP server setup with routing
- `cli_app` - CLI application structure
- `struct_json` - Struct with JSON tags
- `interface_repo` - Repository pattern interface
- `test_function` - Table-driven test template
- `goroutine_worker` - Worker pool with goroutines
- `middleware_chain` - HTTP middleware chain
- `error_handling` - Custom error types
- `config_loader` - Configuration loading
- `sql_database` - Database connection setup

### 🔧 Utilities
- Command history with search functionality
- Session persistence across restarts
- Git integration (status, log, diff)
- Configuration via environment variables or config file
- Graceful offline mode when no API key is configured

## 🚀 Installation

### From GitHub

```bash
go install github.com/SSNamahsos/Mihani-Code/cmd/mihanicode@latest
```

Make sure `$GOPATH/bin` is in your PATH:

```bash
# Linux/macOS
export PATH=$PATH:$GOPATH/bin

# Windows PowerShell
$env:Path += ";$env:GOPATH\bin"
```

### From Source

```bash
git clone https://github.com/SSNamahsos/Mihani-Code.git
cd Mihani-Code
go build -o mihanicode ./cmd/mihanicode
./mihanicode
```

## ⚙️ Configuration

### Environment Variables

Set your API keys as environment variables:

```bash
# OpenAI
export OPENAI_API_KEY=your_openai_api_key_here

# Anthropic
export ANTHROPIC_API_KEY=your_anthropic_api_key_here

# Optional: Custom model
export MIHANI_MODEL=gpt-4o-mini
export MIHANI_DEFAULT_PROVIDER=openai
```

### Configuration File

Create `~/.mihanirc` or `~/.config/mihanicode/config.json`:

```json
{
  "default_provider": "openai",
  "model": "gpt-4o-mini",
  "max_history": 1000,
  "enable_git_integration": true,
  "auto_save_session": true
}
```

## 📖 Usage

Start Mihani Code:

```bash
mihanicode
```

### Commands

#### Chat & AI Commands
- `(message)` - Send a message to the AI assistant
- `/clear` - Clear conversation history
- `/explain <file>` - Explain code in a file
- `/refactor <file> [instruction]` - Refactor code
- `/generate <prompt>` - Generate Go code from description
- `/review <file>` - Review code for issues
- `/debug <problem> <file>` - Debug a specific issue
- `/test <file>` - Generate tests for code

#### File Operations
- `/read <file>` - View contents of a file
- `/write <file>` - Create or overwrite a file
- `/scan [dir]` - Scan directory for Go files

#### Code Snippets
- `/snippets [category]` - List available snippets
- `/snippet <name> [var=value...]` - Insert a snippet

Categories: basic, web, cli, types, patterns, testing, concurrency, utility, database

#### Tools & Utilities
- `/history [query]` - Show command history
- `/config` - Show current configuration
- `/status` - Show session status
- `/tools` - List all available tools
- `/git [status|log|diff]` - Git commands

#### Other
- `/help` - Show help message
- `/about` - About Mihani Code
- `/quit`, `/exit` - Exit the application

## 💡 Examples

```
/explain main.go
/refactor handler.go make it more idiomatic
/generate a function that reads a JSON file
/review database.go
/test calculator.go
/snippet http_server
/scan ./cmd
/git status
```

## 🎨 Branding

Mihani Code features distinctive branding:
- **"Mihani"** is always displayed in **red** text
- **"Code"** appears in the default terminal color
- **"Made by Faz Pad Studio"** attribution in splash screen and about

## 🔒 Offline Mode

When no API key is configured, Mihani Code runs in offline mode with:
- File operations (`/read`, `/write`)
- Code scanning (`/scan`)
- Snippet library
- Command history
- Git integration

## 🛠️ Development

```bash
go build -o mihanicode ./cmd/mihanicode
go test ./...
go vet ./...
```

## 📄 License

MIT License - Made by Faz Pad Studio
