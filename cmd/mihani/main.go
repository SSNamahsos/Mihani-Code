package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/update"
	"github.com/SSNamahsos/Mihani-Code/internal/ui"
)

// version is overridden at build time with -ldflags "-X main.version=..."
var version = "v0.2.24"

func main() {
	var (
		printMode    = flag.Bool("p", false, "print mode: run one prompt headlessly, print the answer, exit")
		continueFlag = flag.Bool("c", false, "continue the most recent session in this workspace")
		resumeID     = flag.String("r", "", "resume a session by id")
		model        = flag.String("model", "", "override the model for this run")
		provider     = flag.String("provider", "", "override the provider for this run")
		yes          = flag.Bool("y", false, "auto-approve tool use (print mode only)")
		showVer      = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "mihani — terminal coding agent\n\nUsage:\n  mihani [flags] [initial prompt]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println("mihani", version)
		return
	}

	// Clear a pre-update binary left behind by an interrupted in-place swap.
	update.CleanupStale()

	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if *provider != "" {
		if _, ok := cfg.Providers[*provider]; !ok {
			fail(fmt.Errorf("unknown provider %q — try /providers inside mihani", *provider))
		}
		cfg.CurrentProvider = *provider
	}
	if *model != "" {
		cfg.CurrentModel = *model
	}
	if *yes {
		cfg.AutoConfirm = true
	}

	prompt := strings.Join(flag.Args(), " ")

	switch {
	case *printMode:
		if prompt == "" {
			fail(errors.New("-p requires a prompt, e.g. mihani -p \"summarize this repo\""))
		}
		if err := ui.RunPrint(cfg, prompt); err != nil {
			fail(err)
		}
	case *continueFlag && *resumeID != "":
		fail(errors.New("use either -c or -r, not both"))
	case *resumeID != "":
		if err := ui.Run(cfg, version, *resumeID, prompt); err != nil {
			fail(err)
		}
	default:
		if err := ui.Run(cfg, version, "", prompt); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mihani: "+err.Error())
	os.Exit(1)
}
