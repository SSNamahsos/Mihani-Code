// Mihani Code - A Go-focused coding assistant CLI
// Made by Faz Pad Studio
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/SSNamahsos/Mihani-Code/internal/chat"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/fileops"
	"github.com/SSNamahsos/Mihani-Code/internal/history"
	"github.com/SSNamahsos/Mihani-Code/internal/llm"
	"github.com/SSNamahsos/Mihani-Code/internal/scanner"
	"github.com/SSNamahsos/Mihani-Code/internal/snippets"
)

// ANSI color codes
const (
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
)

// App represents the main application.
type App struct {
	cfg           *config.Config
	llmClient     llm.Client
	chatSession   *chat.Session
	historyMgr    *history.Manager
	snippetReg    *snippets.Registry
	reader        *bufio.Reader
	currentDir    string
	isOnline      bool
}

// NewApp creates a new application instance.
func NewApp() (*App, error) {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Initialize history manager
	homeDir, _ := os.UserHomeDir()
	historyFile := filepath.Join(homeDir, ".mihanicode", "history.json")
	historyMgr, err := history.NewManager(historyFile, cfg.MaxHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize history: %w", err)
	}

	// Initialize LLM client
	var llmClient llm.Client
	var isOnline bool

	provider := llm.Provider(cfg.DefaultProvider)
	if provider == llm.ProviderNone {
		// Check if any API keys are available
		if cfg.OpenAIAPIKey != "" || os.Getenv("OPENAI_API_KEY") != "" {
			provider = llm.ProviderOpenAI
		} else if cfg.AnthropicAPIKey != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
			provider = llm.ProviderAnthropic
		}
	}

	if provider != llm.ProviderNone {
		llmClient, err = llm.NewClient(provider, "", cfg.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize LLM client: %v\n", err)
			llmClient = llm.NewStandaloneClient()
			isOnline = false
		} else {
			isOnline = true
		}
	} else {
		llmClient = llm.NewStandaloneClient()
		isOnline = false
	}

	// Initialize chat session
	chatSession := chat.NewSession(llmClient)

	// Initialize snippet registry
	snippetReg := snippets.NewRegistry()

	// Get current directory
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = "."
	}

	return &App{
		cfg:          cfg,
		llmClient:    llmClient,
		chatSession:  chatSession,
		historyMgr:   historyMgr,
		snippetReg:   snippetReg,
		reader:       bufio.NewReader(os.Stdin),
		currentDir:   currentDir,
		isOnline:     isOnline,
	}, nil
}

// printBanner displays the startup banner with branding.
func (a *App) printBanner() {
	banner := `
` + ColorBold + ColorRed + `███╗   ███╗ █████╗  ██████╗██████╗ ` + ColorReset + `██████╗  ██████╗ 
` + ColorBold + ColorRed + `████╗ ████║██╔══██╗██╔════╝██╔══██╗` + ColorReset + `██╔══██╗██╔═══██╗
` + ColorBold + ColorRed + `██╔████╔██║███████║██║     ██████╔╝` + ColorReset + `██████╔╝██║   ██║
` + ColorBold + ColorRed + `██║╚██╔╝██║██╔══██║██║     ██╔══██╗` + ColorReset + `██╔═══╝ ██║   ██║
` + ColorBold + ColorRed + `██║ ╚═╝ ██║██║  ██║╚██████╗██║  ██║` + ColorReset + `██║     ╚██████╔╝
` + ColorBold + ColorRed + `╚═╝     ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝` + ColorReset + `╚═╝      ╚═════╝ 
` + ColorCyan + `Mihani ` + ColorReset + `Code - Your Go Programming Assistant
` + ColorYellow + `Made by Faz Pad Studio` + ColorReset + `
`

	fmt.Println(banner)

	if a.isOnline {
		fmt.Printf("%s✓%s Online mode enabled (%s)\n\n", ColorGreen, ColorReset, a.cfg.DefaultProvider)
	} else {
		fmt.Printf("%s⚠%s Running in offline/standalone mode\n", ColorYellow, ColorReset)
		fmt.Printf("Set OPENAI_API_KEY or ANTHROPIC_API_KEY for full AI features\n\n")
	}
}

// printPrompt displays the input prompt with branding.
func (a *App) printPrompt() {
	status := "[offline]"
	if a.isOnline {
		status = "[online]"
	}
	prompt := fmt.Sprintf("%sMihani%s Code %s > ", ColorRed, ColorReset, status)
	fmt.Print(prompt)
}

// Run starts the interactive REPL.
func (a *App) Run() {
	a.printBanner()

	// Start a new session
	sessionID := fmt.Sprintf("session-%d", os.Getpid())
	a.historyMgr.StartSession(sessionID)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Printf("\n\n%sMihani%s Code: Saving session and exiting...\n", ColorRed, ColorReset)
		a.cleanup()
		os.Exit(0)
	}()

	// Main REPL loop
	for {
		a.printPrompt()
		input, err := a.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// End of input (e.g., piped input or Ctrl+D)
				fmt.Println()
				a.cleanup()
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Add to history
		a.historyMgr.Add("command", input, "")

		// Process command
		if err := a.processCommand(input); err != nil {
			fmt.Fprintf(os.Stderr, "%sError:%s %v\n\n", ColorRed, ColorReset, err)
		}

		// Save history periodically
		if a.cfg.AutoSaveSession {
			if err := a.historyMgr.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", err)
			}
		}
	}
}

// processCommand handles user input and dispatches to appropriate handlers.
func (a *App) processCommand(input string) error {
	// Check for slash commands
	if strings.HasPrefix(input, "/") {
		return a.handleSlashCommand(input)
	}

	// Regular chat message
	ctx := context.Background()
	
	fmt.Printf("\n%sMihani%s Code is thinking...\n\n", ColorRed, ColorReset)
	
	response, err := a.chatSession.Chat(ctx, input)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}

	fmt.Println(response)
	fmt.Println()

	return nil
}

// handleSlashCommand processes slash commands.
func (a *App) handleSlashCommand(input string) error {
	parts := strings.Fields(input)
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/help", "/h", "/?":
		return a.cmdHelp(args)
	case "/quit", "/exit", "/q":
		fmt.Printf("%sMihani%s Code: Goodbye!\n", ColorRed, ColorReset)
		a.cleanup()
		os.Exit(0)
	case "/clear", "/cls":
		a.chatSession.Clear()
		fmt.Println("Conversation cleared.")
	case "/explain", "/exp":
		return a.cmdExplain(args)
	case "/refactor", "/ref":
		return a.cmdRefactor(args)
	case "/generate", "/gen":
		return a.cmdGenerate(args)
	case "/read", "/cat":
		return a.cmdRead(args)
	case "/write", "/edit":
		return a.cmdWrite(args)
	case "/scan":
		return a.cmdScan(args)
	case "/snippets":
		return a.cmdSnippets(args)
	case "/snippet":
		return a.cmdSnippet(args)
	case "/history", "/hist":
		return a.cmdHistory(args)
	case "/config":
		return a.cmdConfig(args)
	case "/about":
		return a.cmdAbout(args)
	case "/status":
		return a.cmdStatus(args)
	default:
		return fmt.Errorf("unknown command: %s (use /help for available commands)", cmd)
	}

	return nil
}

// cmdHelp displays help information.
func (a *App) cmdHelp(args []string) error {
	helpText := `
%sMihani%s Code - Available Commands
=====================================

%sChat Commands:%s
  (no prefix)     Send a message to the AI assistant
  /clear          Clear conversation history

%sFile Operations:%s
  /read <file>    View contents of a file
  /write <file>   Create or overwrite a file (enters multi-line mode)
  /scan [dir]     Scan directory for Go files and show summary

%sCode Assistance:%s
  /explain <file> Explain code in a file
  /refactor <file> [instruction]  Refactor code with specific instructions
  /generate <prompt>  Generate Go code from description

%sSnippets:%s
  /snippets [category]  List available code snippets
  /snippet <name> [var=value...]  Insert a code snippet

%sHistory & Config:%s
  /history [query]  Show command history, optionally search
  /config           Show current configuration
  /status           Show current session status

%sOther:%s
  /help             Show this help message
  /about            About Mihani Code
  /quit, /exit      Exit the application

%sTips:%s
- Press Ctrl+C to interrupt a long operation
- Use /clear to reset the conversation context
- Configure API keys via .mihanirc or environment variables

`
	fmt.Printf(helpText,
		ColorRed, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset)

	return nil
}

// cmdExplain explains code in a file.
func (a *App) cmdExplain(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /explain <file>")
	}

	filePath := args[0]
	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("Explaining: %s (%d lines)\n\n", info.Path, info.Lines)

	ctx := context.Background()
	response, err := a.chatSession.ExplainCode(ctx, info.Content)
	if err != nil {
		return fmt.Errorf("explanation failed: %w", err)
	}

	fmt.Println(response)
	return nil
}

// cmdRefactor refactors code in a file.
func (a *App) cmdRefactor(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /refactor <file> [instruction]")
	}

	filePath := args[0]
	instruction := "Improve this code following Go best practices"
	if len(args) > 1 {
		instruction = strings.Join(args[1:], " ")
	}

	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("Refactoring: %s\nInstruction: %s\n\n", info.Path, instruction)

	ctx := context.Background()
	response, err := a.chatSession.RefactorCode(ctx, info.Content, instruction)
	if err != nil {
		return fmt.Errorf("refactoring failed: %w", err)
	}

	fmt.Println(response)
	fmt.Printf("\n%sTip:%s Use /write to save the refactored code\n", ColorYellow, ColorReset)
	return nil
}

// cmdGenerate generates code from a description.
func (a *App) cmdGenerate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /generate <description>")
	}

	prompt := strings.Join(args, " ")
	fmt.Printf("Generating code for: %s\n\n", prompt)

	ctx := context.Background()
	response, err := a.chatSession.GenerateCode(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	fmt.Println(response)
	return nil
}

// cmdRead reads and displays a file.
func (a *App) cmdRead(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /read <file>")
	}

	filePath := args[0]
	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("%s=== %s (%d lines, %s) ===%s\n\n", ColorCyan, info.Path, info.Lines, fileops.FormatFileSize(info.Size), ColorReset)
	fmt.Println(info.Content)
	return nil
}

// cmdWrite creates or overwrites a file.
func (a *App) cmdWrite(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /write <file>")
	}

	filePath := args[0]
	fmt.Printf("Enter content for %s (type 'EOF' on a new line to finish):\n", filePath)

	var content strings.Builder
	scanner := bufio.NewScanner(a.reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "EOF" {
			break
		}
		content.WriteString(line)
		content.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	if err := fileops.WriteFile(filePath, content.String()); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("%s✓%s File written: %s\n", ColorGreen, ColorReset, filePath)
	return nil
}

// cmdScan scans a directory for Go files.
func (a *App) cmdScan(args []string) error {
	dir := a.currentDir
	if len(args) > 0 {
		dir = args[0]
	}

	fmt.Printf("Scanning: %s\n\n", dir)

	summary, err := scanner.GetContextSummary(dir)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	fmt.Println(summary)
	return nil
}

// cmdSnippets lists available snippets.
func (a *App) cmdSnippets(args []string) error {
	category := ""
	if len(args) > 0 {
		category = args[0]
	}

	snips := a.snippetReg.List(category)
	
	if len(snips) == 0 {
		fmt.Println("No snippets found.")
		return nil
	}

	fmt.Printf("%sAvailable Snippets:%s\n\n", ColorBold, ColorReset)
	for _, s := range snips {
		fmt.Printf("  %s%-20s%s [%s] - %s\n", ColorCyan, s.Name, ColorReset, s.Category, s.Description)
	}
	fmt.Println()

	return nil
}

// cmdSnippet renders and displays a specific snippet.
func (a *App) cmdSnippet(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /snippet <name> [var=value...]")
	}

	name := args[0]
	vars := make(map[string]string)
	for _, arg := range args[1:] {
		if idx := strings.Index(arg, "="); idx > 0 {
			vars[arg[:idx]] = arg[idx+1:]
		}
	}

	rendered, err := a.snippetReg.Render(name, vars)
	if err != nil {
		return fmt.Errorf("snippet error: %w", err)
	}

	fmt.Printf("%s=== Snippet: %s ===%s\n\n", ColorCyan, name, ColorReset)
	fmt.Println(rendered)
	return nil
}

// cmdHistory shows command history.
func (a *App) cmdHistory(args []string) error {
	var entries []history.Entry
	
	if len(args) > 0 {
		query := strings.Join(args, " ")
		entries = a.historyMgr.Search(query)
		fmt.Printf("Search results for '%s':\n\n", query)
	} else {
		entries = a.historyMgr.GetRecent(20)
		fmt.Println("Recent commands:")
	}

	if len(entries) == 0 {
		fmt.Println("No history found.")
		return nil
	}

	for i, e := range entries {
		timeStr := e.Timestamp.Format("15:04:05")
		fmt.Printf("[%s] %s: %s\n", timeStr, e.Type, e.Content)
		if i >= 19 && len(args) == 0 {
			fmt.Println("... (use /history <query> to search)")
			break
		}
	}
	fmt.Println()

	return nil
}

// cmdConfig shows current configuration.
func (a *App) cmdConfig(args []string) error {
	configPath := config.GetConfigPath()
	
	fmt.Printf("%sCurrent Configuration:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Printf("Default provider: %s\n", a.cfg.DefaultProvider)
	fmt.Printf("Model: %s\n", a.cfg.Model)
	fmt.Printf("Max history: %d\n", a.cfg.MaxHistory)
	fmt.Printf("Git integration: %v\n", a.cfg.EnableGitIntegration)
	fmt.Printf("Auto-save session: %v\n", a.cfg.AutoSaveSession)
	
	hasOpenAI := a.cfg.OpenAIAPIKey != "" || os.Getenv("OPENAI_API_KEY") != ""
	hasAnthropic := a.cfg.AnthropicAPIKey != "" || os.Getenv("ANTHROPIC_API_KEY") != ""
	
	fmt.Printf("\n%sAPI Keys:%s\n", ColorBold, ColorReset)
	fmt.Printf("OpenAI configured: %v\n", hasOpenAI)
	fmt.Printf("Anthropic configured: %v\n", hasAnthropic)
	fmt.Println()

	return nil
}

// cmdAbout shows about information.
func (a *App) cmdAbout(args []string) error {
	aboutText := `
%sMihani Code%s
Version: 1.0.0

%sDescription:%s
Mihani Code is a terminal-based coding assistant specialized for Go development.
It provides AI-powered code explanations, refactoring suggestions, and code
generation capabilities, along with offline tools for file operations and
code scanning.

%sFeatures:%s
- Interactive chat/REPL mode for coding questions
- File reading and editing from the terminal
- Code explanation and refactoring (Go-focused)
- Project/directory scanning for codebase context
- Code snippet templates for common Go patterns
- Command history and session persistence
- Support for OpenAI and Anthropic APIs
- Graceful offline/standalone mode

%sCreator:%s
Made by %sFaz Pad Studio%s

%sConfiguration:%s
Create a ~/.mihanirc file (JSON format) to customize settings:
{
  "openai_api_key": "your-key-here",
  "anthropic_api_key": "your-key-here",
  "default_provider": "openai",
  "model": "gpt-4o-mini",
  "max_history": 1000
}

Or set environment variables:
- OPENAI_API_KEY
- ANTHROPIC_API_KEY

`
	fmt.Printf(aboutText,
		ColorRed, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset)

	return nil
}

// cmdStatus shows current session status.
func (a *App) cmdStatus(args []string) error {
	stats := a.chatSession.GetStats()
	
	fmt.Printf("%sSession Status:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("Mode: %s\n", map[bool]string{true: "online", false: "offline"}[a.isOnline])
	fmt.Printf("Provider: %s\n", a.cfg.DefaultProvider)
	fmt.Printf("Working directory: %s\n", a.currentDir)
	fmt.Printf("\n%sChat Statistics:%s\n", ColorBold, ColorReset)
	fmt.Printf("Messages exchanged: %d\n", stats.MessageCount)
	fmt.Printf("User messages: %d\n", stats.UserMessages)
	fmt.Printf("Assistant responses: %d\n", stats.AssistantMessages)
	fmt.Printf("Total characters: %d\n", stats.TotalCharacters)
	fmt.Println()

	return nil
}

// cleanup performs cleanup before exit.
func (a *App) cleanup() {
	a.historyMgr.EndSession()
	if a.cfg.AutoSaveSession {
		if err := a.historyMgr.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", err)
		}
	}
}

func main() {
	app, err := NewApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize Mihani Code: %v\n", err)
		os.Exit(1)
	}

	app.Run()
}
