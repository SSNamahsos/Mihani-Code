package main

import (
"bufio"
"fmt"
"os"
"strings"

"github.com/SSNamahsos/Mihani-Code/internal/agent"
"github.com/SSNamahsos/Mihani-Code/internal/config"
"github.com/fatih/color"
)

var (
redColor    = color.New(color.FgRed)
greenColor  = color.New(color.FgGreen)
yellowColor = color.New(color.FgYellow)
cyanColor   = color.New(color.FgCyan)
)

func printBanner() {
fmt.Println()
redColor.Print("  __  __ _   _ _   _ _____ ____  ")
redColor.Print(" |  \\/  | \\ | | \\ | | ____|  _ \\ ")
redColor.Print(" | |\\/| |  \\| |  \\| |  _| | |_) |")
redColor.Print(" | |  | | |\\  | |\\  | |___|  _ < ")
redColor.Print(" |_|  |_|_| \\_|_| \\_|_____|_| \\_\\")
fmt.Println()
fmt.Println("  Made by Faz Pad Studio")
fmt.Println()
}

func printBrandedName() {
redColor.Print("Mihani")
fmt.Print(" Code")
}

func main() {
cfg, err := config.Load()
if err != nil {
fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
os.Exit(1)
}

printBanner()

if cfg.APIKey == "" {
yellowColor.Println("Warning: No API key configured. Set MIHANI_API_KEY environment variable.")
yellowColor.Println("Mihani Code will run in offline mode with limited functionality.")
fmt.Println()
} else {
greenColor.Println("API configured")
fmt.Printf("Provider: %s\n", cfg.Provider)
fmt.Printf("Model: %s\n", cfg.Model)
fmt.Println()
}

var task string
if len(os.Args) > 1 {
task = strings.Join(os.Args[1:], " ")
runOneShot(task, cfg)
return
}

fmt.Println("Starting interactive agent...")
fmt.Println("Type your task or /help for commands")
fmt.Println()

interactiveMode(cfg)
}

func runOneShot(task string, cfg *config.Config) {
if cfg.APIKey == "" {
yellowColor.Println("Error: API key required for one-shot mode")
os.Exit(1)
}

ag, err := agent.NewAgent(cfg)
if err != nil {
fmt.Fprintf(os.Stderr, "Failed to initialize agent: %v\n", err)
os.Exit(1)
}

callback := &simpleCallback{}
result, err := ag.Run(task, callback)
if err != nil {
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
os.Exit(1)
}

fmt.Println()
fmt.Println(result)
}

func interactiveMode(cfg *config.Config) {
var ag *agent.Agent
if cfg.APIKey != "" {
var err error
ag, err = agent.NewAgent(cfg)
if err != nil {
fmt.Fprintf(os.Stderr, "Failed to initialize agent: %v\n", err)
os.Exit(1)
}
}

reader := bufio.NewReader(os.Stdin)

for {
fmt.Print("> ")
input, err := reader.ReadString('\n')
if err != nil {
break
}

input = strings.TrimSpace(input)
if input == "" {
continue
}

if input == "/exit" || input == "/quit" {
fmt.Println("Goodbye!")
break
}

if input == "/help" {
printHelp()
continue
}

if input == "/status" {
printStatus(cfg)
continue
}

if ag == nil {
yellowColor.Println("Agent not available. Configure API key to use agent features.")
continue
}

callback := &terminalCallback{}
result, err := ag.Run(input, callback)
if err != nil {
redColor.Printf("Error: %v\n", err)
continue
}

if result != "" {
fmt.Println()
fmt.Println(result)
}
fmt.Println()
}
}

func printHelp() {
fmt.Println()
cyanColor.Println("Available commands:")
fmt.Println("  /help     - Show this help message")
fmt.Println("  /status   - Show current configuration status")
fmt.Println("  /exit     - Exit Mihani Code")
fmt.Println()
fmt.Println("Or type any natural language task:")
fmt.Println("  \"Fix the authentication bug\"")
fmt.Println("  \"Add a new endpoint for user profiles\"")
fmt.Println("  \"Run the tests and fix any failures\"")
fmt.Println()
}

func printStatus(cfg *config.Config) {
fmt.Println()
printBrandedName()
fmt.Println(" Status:")
fmt.Println()
fmt.Printf("  Provider:       %s\n", cfg.Provider)
fmt.Printf("  Model:          %s\n", cfg.Model)
fmt.Printf("  Max Iterations: %d\n", cfg.MaxIterations)
fmt.Printf("  Command Timeout: %ds\n", cfg.CommandTimeout)
fmt.Printf("  Auto Approve:   %v\n", cfg.AutoApprove)
fmt.Println()
}

type simpleCallback struct{}

func (c *simpleCallback) OnToolStart(name string, args map[string]interface{}) {}
func (c *simpleCallback) OnToolComplete(event *agent.ToolCallEvent)            {}
func (c *simpleCallback) OnMessage(content string)                             {}

type terminalCallback struct{}

func (c *terminalCallback) OnToolStart(name string, args map[string]interface{}) {
cyanColor.Printf("[%s] Starting...\n", name)
}

func (c *terminalCallback) OnToolComplete(event *agent.ToolCallEvent) {
if event.Error != "" {
redColor.Printf("[%s] Failed: %v\n", event.Name, event.Error)
} else {
greenColor.Printf("[%s] Completed in %v\n", event.Name, event.Duration)
}
}

func (c *terminalCallback) OnMessage(content string) {
fmt.Println(content)
}
