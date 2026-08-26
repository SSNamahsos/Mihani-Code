package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

// RunPrint executes one prompt headlessly: assistant text streams to stdout,
// progress goes to stderr, and the process exits when the turn completes.
func RunPrint(cfg config.Config, prompt string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if budget := cfg.BudgetEnforced(cfg.CurrentProvider); budget > 0 {
		if spend := usage.WindowSum(cfg.CurrentProvider); spend >= budget {
			return fmt.Errorf("Mihani daily limit reached: $%.2f of $%.2f used in the last 24h",
				spend, budget)
		}
	}
	a := &agent.Agent{Cfg: cfg, Root: root}
	a.MaxIterations = cfg.MaxIterations

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err = a.Send(ctx, prompt, "build",
		func(name string, _ map[string]any) bool {
			if cfg.AutoConfirm || cfg.Permissions["shell"] == "allow" && name == "bash" {
				return true
			}
			return !tools.Lookup(name).Dangerous
		},
		func(ev agent.Event) {
			switch ev.Kind {
			case "text":
				fmt.Print(ev.Text)
			case "tool_start":
				detail := summarizeInput(ev.Tool, ev.Input)
				if detail != "" {
					fmt.Fprintf(os.Stderr, "· %s %s\n", ev.Tool, detail)
				} else {
					fmt.Fprintf(os.Stderr, "· %s\n", ev.Tool)
				}
			case "tool_done":
				if strings.HasPrefix(ev.ToolResult, "ERROR") {
					fmt.Fprintf(os.Stderr, "✗ %s\n", firstLine(ev.ToolResult))
				}
			case "usage":
				if ev.CostUSD > 0 {
					usage.Add(usage.Entry{
						Time:     time.Now(),
						Provider: cfg.CurrentProvider,
						Model:    cfg.CurrentModel,
						Input:    ev.InputTok,
						Output:   ev.OutputTok,
						CostUSD:  ev.CostUSD,
					})
					fmt.Fprintf(os.Stderr, "$ %.4f this request · %.2f today\n",
						ev.CostUSD, usage.WindowSum(cfg.CurrentProvider))
				}
			}
		})
	fmt.Println()
	a.Close()
	return err
}
