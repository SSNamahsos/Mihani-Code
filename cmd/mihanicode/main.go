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
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/chat"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/fileops"
	"github.com/SSNamahsos/Mihani-Code/internal/git"
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
	ColorMagenta = "\033[35m"
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
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
	providerName  string
	showTips      bool
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
	historyDir := filepath.Join(homeDir, ".mihanicode")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %w", err)
	}
	historyFile := filepath.Join(historyDir, "history.json")
	historyMgr, err := history.NewManager(historyFile, cfg.MaxHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize history: %w", err)
	}

	// Initialize LLM client
	var llmClient llm.Client
	var isOnline bool
	var providerName string

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
			providerName = "offline"
		} else {
			isOnline = true
			providerName = string(provider)
		}
	} else {
		llmClient = llm.NewStandaloneClient()
		isOnline = false
		providerName = "offline"
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
		providerName: providerName,
		showTips:     true,
	}, nil
}

// printBanner displays the startup banner with branding.
func (a *App) printBanner() {
	banner := `
` + ColorBold + ColorRed + `╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   ███╗   ██╗ ██████╗  ████████╗██████╗ ` + ColorReset + `██████╗  ██████╗ ` + ColorRed + `
║   ████╗ ████║██╔═══██╗╚══██╔══╝██╔══██╗` + ColorReset + `██╔══██╗██╔═══██╗` + ColorRed + `
║   ██╔████╔██║███████║   ██║   ██████╔╝` + ColorReset + `██████╔╝██║   ██║` + ColorRed + `
║   ██║╚██╔╝██║██╔══██║   ██║   ██╔══██╗` + ColorReset + `██╔═══╝ ██║   ██║` + ColorRed + `
║   ██║ ╚═╝ ██║██║  ██║   ██║   ██║  ██║` + ColorReset + `██║     ╚██████╔╝` + ColorRed + `
║   ╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝` + ColorReset + `╚═╝      ╚═════╝ ` + ColorRed + `
║                                                           ║
╚═══════════════════════════════════════════════════════════╝` + ColorReset + `

` + ColorCyan + `Mihani ` + ColorReset + `Code - Your Go Programming Assistant
` + ColorYellow + `Made by Faz Pad Studio` + ColorReset + `
`

	fmt.Println(banner)

	if a.isOnline {
		fmt.Printf("%s✓%s Online mode enabled (%s)%s\n\n", ColorGreen, ColorReset, ColorBold+a.providerName+ColorReset, ColorDim)
	} else {
		fmt.Printf("%s⚠%s Running in %soffline/standalone mode%s\n", ColorYellow, ColorReset, ColorBold, ColorReset)
		fmt.Printf("%sSet OPENAI_API_KEY or ANTHROPIC_API_KEY for full AI features%s\n\n", ColorDim, ColorReset)
	}

	// Show quick tips
	a.showQuickTips()
}

// showQuickTips displays helpful tips for new users.
func (a *App) showQuickTips() {
	if !a.showTips {
		return
	}

	tips := []string{
		fmt.Sprintf("%s/%sTry %s/help%s to see all commands", ColorDim, ColorReset, ColorBold, ColorReset),
		fmt.Sprintf("%s/%sUse %s/scan%s to analyze your codebase", ColorDim, ColorReset, ColorBold, ColorReset),
		fmt.Sprintf("%s/%sPress %sCtrl+C%s to interrupt long operations", ColorDim, ColorReset, ColorBold, ColorReset),
	}

	fmt.Print(ColorDim)
	for i, tip := range tips {
		if i > 0 {
			fmt.Print("  •  ")
		}
		fmt.Print(tip)
		if i < len(tips)-1 {
			fmt.Print("\n         ")
		}
	}
	fmt.Println(ColorReset)
	fmt.Println()
}

// printPrompt displays the input prompt with branding.
func (a *App) printPrompt() {
	status := ColorYellow + "[offline]" + ColorReset
	if a.isOnline {
		status = ColorGreen + "[online]" + ColorReset
	}
	
	// Truncate long paths
	dir := a.currentDir
	homeDir, _ := os.UserHomeDir()
	if strings.HasPrefix(dir, homeDir) {
		dir = "~" + dir[len(homeDir):]
	}
	if len(dir) > 30 {
		dir = "..." + dir[len(dir)-27:]
	}
	
	prompt := fmt.Sprintf("%sMihani%s Code %s %s%s%s › ", ColorRed, ColorReset, status, ColorDim, dir, ColorReset)
	fmt.Print(prompt)
}

// Run starts the interactive REPL.
func (a *App) Run() {
	a.printBanner()

	// Start a new session
	sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
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
				fmt.Fprintf(os.Stderr, "%sWarning:%s Failed to save history: %v\n", ColorYellow, ColorReset, err)
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
	
	fmt.Printf("\n%sMihani%s Code is thinking%s...\n\n", ColorRed, ColorReset, ColorDim)
	
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
		fmt.Printf("%sMihani%s Code: Goodbye! Happy coding! 🚀\n", ColorRed, ColorReset)
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
	case "/review":
		return a.cmdReview(args)
	case "/debug":
		return a.cmdDebug(args)
	case "/test":
		return a.cmdTest(args)
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
	case "/git":
		return a.cmdGit(args)
	case "/tools":
		return a.cmdTools(args)
	default:
		return fmt.Errorf("unknown command: %s (use /help for available commands)", cmd)
	}

	return nil
}

// cmdHelp displays help information.
func (a *App) cmdHelp(args []string) error {
	helpText := `
%s╔══════════════════════════════════════════════════════════╗
║           Mihani Code - Available Commands                ║
╚══════════════════════════════════════════════════════════╝%s

%s📌 Chat & AI Commands:%s
  (no prefix)      Send a message to the AI assistant
  /clear           Clear conversation history
  /explain <file>  Explain code in a file
  /refactor <file> [instruction]  Refactor code
  /generate <prompt>  Generate Go code from description
  /review <file>   Review code for issues and improvements
  /debug <problem> <code>  Debug a specific issue
  /test <file>     Generate tests for code

%s📁 File Operations:%s
  /read <file>     View contents of a file
  /write <file>    Create or overwrite a file (multi-line mode)
  /scan [dir]      Scan directory for Go files and show summary

%s🧩 Code Snippets:%s
  /snippets [category]  List available code snippets
  /snippet <name> [var=value...]  Insert a code snippet
  
  Categories: basic, web, cli, types, patterns, testing, concurrency, utility, database

%s🔧 Tools & Utilities:%s
  /history [query] Show command history, optionally search
  /config          Show current configuration
  /status          Show current session status
  /tools           List all available tools and capabilities
  /git [status|log|diff]  Git integration commands

%sℹ️  Other:%s
  /help            Show this help message
  /about           About Mihani Code
  /quit, /exit     Exit the application

%s💡 Tips:%s
  • Press Ctrl+C to interrupt a long operation
  • Use /clear to reset the conversation context
  • Configure API keys via ~/.mihanirc or environment variables
  • All code examples are Go-focused

`
	fmt.Printf(helpText,
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

	fmt.Printf("%sExplaining:%s %s (%d lines)\n\n", ColorCyan, ColorReset, info.Path, info.Lines)

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

	fmt.Printf("%sRefactoring:%s %s\n%sInstruction:%s %s\n\n", 
		ColorCyan, ColorReset, info.Path, ColorCyan, ColorReset, instruction)

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
	fmt.Printf("%sGenerating code for:%s %s\n\n", ColorCyan, ColorReset, prompt)

	ctx := context.Background()
	response, err := a.chatSession.GenerateCode(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	fmt.Println(response)
	return nil
}

// cmdReview reviews code in a file.
func (a *App) cmdReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /review <file>")
	}

	filePath := args[0]
	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("%sReviewing:%s %s (%d lines)\n\n", ColorCyan, ColorReset, info.Path, info.Lines)

	ctx := context.Background()
	response, err := a.chatSession.ReviewCode(ctx, info.Content)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	fmt.Println(response)
	return nil
}

// cmdDebug helps debug an issue.
func (a *App) cmdDebug(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: /debug <problem_description> <code_file>")
	}

	// Last arg is the file, rest is the problem description
	filePath := args[len(args)-1]
	problem := strings.Join(args[:len(args)-1], " ")

	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("%sDebugging:%s %s\n%sProblem:%s %s\n\n", 
		ColorCyan, ColorReset, info.Path, ColorCyan, ColorReset, problem)

	ctx := context.Background()
	response, err := a.chatSession.DebugHelp(ctx, problem, info.Content)
	if err != nil {
		return fmt.Errorf("debug failed: %w", err)
	}

	fmt.Println(response)
	return nil
}

// cmdTest generates tests for a file.
func (a *App) cmdTest(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /test <file>")
	}

	filePath := args[0]
	info, err := fileops.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("%sGenerating tests for:%s %s\n\n", ColorCyan, ColorReset, info.Path)

	ctx := context.Background()
	response, err := a.chatSession.GenerateTests(ctx, info.Content)
	if err != nil {
		return fmt.Errorf("test generation failed: %w", err)
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

	fmt.Printf("%s=== %s (%d lines, %s) ===%s\n\n", 
		ColorCyan, info.Path, info.Lines, fileops.FormatFileSize(info.Size), ColorReset)
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

	fmt.Printf("%sScanning:%s %s\n\n", ColorCyan, ColorReset, dir)

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
		if category != "" {
			fmt.Printf("No snippets found in category '%s'.\n", category)
		} else {
			fmt.Println("No snippets found.")
		}
		return nil
	}

	fmt.Printf("%sAvailable Snippets:%s\n\n", ColorBold, ColorReset)
	for _, s := range snips {
		fmt.Printf("  %s%-20s%s [%s] - %s\n", 
			ColorCyan, s.Name, ColorReset, s.Category, s.Description)
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
%s╔══════════════════════════════════════════════════════════╗
║                    Mihani Code                            ║
║              Made by Faz Pad Studio                       ║
╚══════════════════════════════════════════════════════════╝%s

Version: 1.0.0

%sDescription:%s
Mihani Code is a terminal-based coding assistant specialized for Go development.
It provides AI-powered code explanations, refactoring suggestions, and code
generation capabilities, along with offline tools for file operations and
code scanning.

%sFeatures:%s
• Interactive chat/REPL mode for coding questions
• File reading and editing from the terminal
• Code explanation and refactoring (Go-focused)
• Project/directory scanning for codebase context
• Code snippet templates for common Go patterns
• Command history and session persistence
• Git integration for version control
• Offline mode with reduced capabilities

%sAPI Support:%s
• OpenAI (GPT-4, GPT-4o-mini, etc.)
• Anthropic (Claude models)
• Graceful offline fallback

%sConfiguration:%s
• Environment variables: OPENAI_API_KEY, ANTHROPIC_API_KEY
• Config file: ~/.mihanirc or ~/.config/mihanicode/config.json
• Per-session customization

`
	fmt.Printf(aboutText,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset)

	return nil
}

// cmdStatus shows current session status.
func (a *App) cmdStatus(args []string) error {
	fmt.Printf("%sSession Status:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("Mode: %s\n", a.providerName)
	fmt.Printf("Online: %v\n", a.isOnline)
	fmt.Printf("Current directory: %s\n", a.currentDir)
	fmt.Printf("Messages in conversation: %d\n", a.chatSession.GetMessagesCount())
	
	stats := a.historyMgr.Stats()
	fmt.Printf("Total history entries: %v\n", stats["total_entries"])
	fmt.Printf("Total sessions: %v\n", stats["total_sessions"])
	fmt.Println()

	return nil
}

// cmdGit handles git commands.
func (a *App) cmdGit(args []string) error {
	if !a.cfg.EnableGitIntegration {
		return fmt.Errorf("git integration is disabled in config")
	}

	if !git.IsGitRepo(a.currentDir) {
		return fmt.Errorf("not a git repository")
	}

	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}

	switch subcmd {
	case "status", "st", "":
		status, err := git.GetStatus(a.currentDir)
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}
		fmt.Println(git.FormatStatus(status))

	case "log", "l":
		count := 5
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &count)
		}
		commits, err := git.GetRecentCommits(a.currentDir, count)
		if err != nil {
			return fmt.Errorf("failed to get commits: %w", err)
		}
		fmt.Printf("%sRecent Commits:%s\n\n", ColorBold, ColorReset)
		for _, c := range commits {
			fmt.Printf("%s[%s]%s %s - %s\n", 
				ColorCyan, c.Hash[:7], ColorReset, c.Author, c.Message)
		}
		fmt.Println()

	case "diff", "d":
		staged := len(args) > 1 && args[1] == "--staged"
		diff, err := git.GetDiff(a.currentDir, staged)
		if err != nil {
			return fmt.Errorf("failed to get diff: %w", err)
		}
		if diff == "" {
			fmt.Println("No changes.")
		} else {
			fmt.Println(diff)
		}

	default:
		return fmt.Errorf("unknown git subcommand: %s (use: status, log, diff)", subcmd)
	}

	return nil
}

// cmdTools lists available tools.
func (a *App) cmdTools(args []string) error {
	toolsText := `
%s╔══════════════════════════════════════════════════════════╗
║                  Available Tools                          ║
╚══════════════════════════════════════════════════════════╝%s

%s🤖 AI-Powered Tools:%s
  • Chat/REPL        - Interactive coding assistant
  • Code Explanation - Understand complex code
  • Code Refactoring - Improve code quality
  • Code Generation  - Generate Go code from descriptions
  • Code Review      - Find issues and improvements
  • Debug Helper     - Troubleshoot problems
  • Test Generation  - Create unit tests

%s📁 File Tools:%s
  • Read files       - View file contents
  • Write files      - Create/edit files
  • Code scanning    - Analyze codebase structure

%s🧩 Snippet Library:%s
  • main             - Basic main function template
  • http_server      - HTTP server setup
  • cli_app          - CLI application structure
  • struct_json      - Struct with JSON tags
  • interface_repo   - Repository pattern
  • test_function    - Table-driven tests
  • goroutine_worker - Worker pool pattern
  • middleware_chain - HTTP middleware
  • error_handling   - Custom error types
  • config_loader    - Configuration loading
  • sql_database     - Database connection

%s🔧 Utility Tools:%s
  • History tracking - Command history management
  • Session export   - Export conversations
  • Git integration  - Version control commands

`
	fmt.Printf(toolsText,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset,
		ColorBold, ColorReset)

	return nil
}

// cleanup performs cleanup before exit.
func (a *App) cleanup() {
	a.historyMgr.EndSession()
	if err := a.historyMgr.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save history: %v\n", err)
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
