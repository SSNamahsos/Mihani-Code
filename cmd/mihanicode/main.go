package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/providers"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	version = "0.1.0"
	appName = "Mihani Code"
)

func main() {
	// Parse command line flags
	var (
		showVersion bool
		showHelp    bool
		modelFlag   string
		configFlag  string
		sessionFlag string
		continueFlag bool
		autoApprove bool
		debug       bool
	)

	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.StringVar(&modelFlag, "model", "", "Specify model to use")
	flag.StringVar(&configFlag, "config", "", "Path to config file")
	flag.StringVar(&sessionFlag, "session", "", "Resume session by ID")
	flag.BoolVar(&continueFlag, "continue", false, "Continue last session")
	flag.BoolVar(&continueFlag, "c", false, "Continue last session")
	flag.BoolVar(&autoApprove, "auto", false, "Auto-approve all operations")
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")

	flag.Parse()

	// Handle version
	if showVersion {
		printBanner()
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}

	// Handle help
	if showHelp {
		printBanner()
		printHelp()
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Apply command-line overrides
	if modelFlag != "" {
		cfg.Provider.Model = modelFlag
	}
	if autoApprove {
		cfg.Permissions.AutoApprove = true
	}
	if debug {
		cfg.Limits.Debug = true
	}

	// Check for API key
	if cfg.Provider.APIKey == "" {
		fmt.Println()
		printBanner()
		fmt.Println()
		fmt.Println("⚠️  No API key configured.")
		fmt.Println()
		fmt.Println("Set your API key using one of these methods:")
		fmt.Println("  1. Environment variable: export MIHANI_API_KEY=your-key")
		fmt.Println("  2. Config file: ~/.mihani/config.json")
		fmt.Println("  3. Command line: mihanicode --config /path/to/config.json")
		fmt.Println()
		fmt.Println("Supported providers:")
		fmt.Println("  - OpenAI (export MIHANI_PROVIDER=openai)")
		fmt.Println("  - OpenAI-compatible APIs (set MIHANI_BASE_URL)")
		fmt.Println("  - Anthropic (export MIHANI_PROVIDER=anthropic)")
		fmt.Println()
		fmt.Println("Starting in offline/demo mode...")
	}

	// Determine working directory
	workDir := "."
	if flag.NArg() > 0 {
		workDir = flag.Arg(0)
		info, err := os.Stat(workDir)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: '%s' is not a valid directory\n", workDir)
			os.Exit(1)
		}
		if err := os.Chdir(workDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to change directory: %v\n", err)
			os.Exit(1)
		}
		workDir, _ = os.Getwd()
	} else {
		workDir, _ = os.Getwd()
	}

	cfg.WorkDir = workDir

	// Create provider
	provider, err := providers.CreateProvider(
		cfg.Provider.Provider,
		cfg.Provider.BaseURL,
		cfg.Provider.APIKey,
		cfg.Provider.Model,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	// Create tool registry
	toolRegistry := tools.BuildDefaultRegistry()

	// Create agent
	ag := agent.NewAgent(cfg, provider, toolRegistry)

	// Handle one-shot mode
	if flag.NArg() > 0 && !strings.HasPrefix(flag.Arg(0), "-") {
		// One-shot mode: execute task and exit
		task := strings.Join(flag.Args(), " ")
		runOneShotMode(ag, task)
		return
	}

	// Interactive TUI mode
	runInteractiveMode(ag, cfg, sessionFlag)
}

func printBanner() {
	// Branding: "Mihani" in red, "Code" in default
	fmt.Printf("\n\x1b[31m%s\x1b[0m%s\n", "Mihani", " Code")
	fmt.Println("Made by Faz Pad Studio")
	fmt.Println(strings.Repeat("─", 40))
}

func printHelp() {
	help := `
Mihani Code - Agentic Terminal Coding Assistant

USAGE:
  mihanicode [options] [directory]
  mihanicode [options] "task description"

OPTIONS:
  --version, -v      Show version information
  --help, -h         Show this help message
  --model <name>     Specify the AI model to use
  --config <path>    Path to configuration file
  --session <id>     Resume a specific session
  --continue, -c     Continue the last session
  --auto             Auto-approve all operations (use with caution)
  --debug            Enable debug logging

EXAMPLES:
  # Start interactive mode in current directory
  mihanicode

  # Start in a specific project directory
  mihanicode /path/to/project

  # One-shot mode: execute a single task
  mihanicode "fix the authentication bug"
  mihanicode "add user registration endpoint"
  mihanicode "why are the tests failing?"

  # Use specific model
  mihanicode --model gpt-4o "refactor the database layer"

  # Continue previous session
  mihanicode --continue

CONFIGURATION:
  Mihani Code can be configured via:
  
  1. Environment variables:
     MIHANI_API_KEY       - Your API key
     MIHANI_BASE_URL      - API base URL (for OpenAI-compatible)
     MIHANI_MODEL         - Default model name
     MIHANI_PROVIDER      - Provider type (openai, anthropic)
     MIHANI_DEBUG         - Enable debug mode (1 or 0)

  2. Configuration file (~/.mihani/config.json):
     {
       "provider": {
         "provider": "openai-compatible",
         "base_url": "https://api.openai.com/v1",
         "api_key": "your-key",
         "model": "gpt-4o"
       },
       "permissions": {
         "auto_approve": false
       },
       "limits": {
         "max_iterations": 50,
         "command_timeout": 120
       }
     }

INTERACTIVE COMMANDS:
  /help, /h          Show available commands
  /clear, /c         Clear conversation history
  /exit, /quit, /q   Exit Mihani Code
  /model <name>      Change or show current model
  /agent <mode>      Change agent mode (build/plan)
  /status            Show current status
  /tasks             Show task list
  /compact           Compact conversation history

For more information, visit: https://github.com/SSNamahsos/Mihani-Code
`
	fmt.Println(help)
}

func runOneShotMode(ag *agent.Agent, task string) {
	printBanner()
	fmt.Printf("\n📋 Task: %s\n\n", task)
	fmt.Println("Processing...")

	eventChan := make(chan agent.AgentEvent, 100)

	go func() {
		_, err := ag.Run(task, eventChan)
		if err != nil {
			eventChan <- agent.AgentEvent{
				Type: agent.EventTypeError,
				Data: err.Error(),
			}
		}
		close(eventChan)
	}()

	for event := range eventChan {
		handleEvent(event)
	}

	fmt.Println("\n✅ Task completed.")
}

func runInteractiveMode(ag *agent.Agent, cfg *config.Config, sessionFlag string) {
	// Generate session ID if not provided
	sessionID := sessionFlag
	if sessionID == "" {
		sessionID = fmt.Sprintf("session_%d", os.Getpid())
	}

	// Create TUI model
	tuiModel := newInteractiveModel(ag, cfg, sessionID)

	// Run the TUI
	p := tea.NewProgram(tuiModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func handleEvent(event agent.AgentEvent) {
	switch event.Type {
	case agent.EventTypeUserMessage:
		fmt.Printf("\x1b[32m>\x1b[0m %v\n\n", event.Data)
	case agent.EventTypeAssistantMessage:
		fmt.Printf("%v\n", event.Data)
	case agent.EventTypeToolStart:
		if te, ok := event.Data.(agent.ToolEvent); ok {
			fmt.Printf("\x1b[33m●\x1b[0m %s...\n", te.Name)
		}
	case agent.EventTypeToolComplete:
		if te, ok := event.Data.(agent.ToolEvent); ok {
			fmt.Printf("\x1b[32m✓\x1b[0m %s completed\n", te.Name)
		}
	case agent.EventTypeToolError:
		if te, ok := event.Data.(agent.ToolErrorEvent); ok {
			fmt.Printf("\x1b[31m✗\x1b[0m %s failed: %s\n", te.Name, te.Error)
		}
	case agent.EventTypePermissionRequest:
		if pe, ok := event.Data.(agent.PermissionEvent); ok {
			fmt.Printf("\x1b[33m⚠\x1b[0m Permission required for %s\n", pe.Tool)
		}
	case agent.EventTypeError:
		fmt.Printf("\x1b[31mError:\x1b[0m %v\n", event.Data)
	}
}

// Simple interactive model for basic terminal interaction
type simpleModel struct {
	agent      *agent.Agent
	config     *config.Config
	sessionID  string
	input      string
	messages   []string
	quitting   bool
	width      int
	height     int
}

func newInteractiveModel(ag *agent.Agent, cfg *config.Config, sessionID string) *simpleModel {
	return &simpleModel{
		agent:     ag,
		config:    cfg,
		sessionID: sessionID,
		messages:  make([]string, 0),
	}
}

func (m simpleModel) Init() tea.Cmd {
	return nil
}

func (m simpleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.input != "" {
				m.messages = append(m.messages, "> "+m.input)
				m.input = ""
			}
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		case tea.KeyRunes:
			m.input += msg.String()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m simpleModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("\x1b[31m%s\x1b[0m%s", "Mihani", " Code"))
	b.WriteString(" | Made by Faz Pad Studio\n")
	b.WriteString(strings.Repeat("─", 50) + "\n\n")

	// Messages
	for _, msg := range m.messages {
		b.WriteString(msg + "\n")
	}

	// Input prompt
	b.WriteString(fmt.Sprintf("\x1b[31mMihani\x1b[0m> %s", m.input))

	return b.String()
}
