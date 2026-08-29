// Command mihani-probe is a tiny diagnostic for mouse input problems.
//
// It opens a plain bubbletea screen with mouse capture ON (the same way the
// real TUI does) and writes every input event it receives — clicks, drags,
// wheel, keys — to %TEMP%\mihani-probe.log.
//
// Usage: run `mihani-probe` in the terminal where mihani misbehaves, click a
// few times, drag over text, scroll, wait for "PROBE DONE", then share the
// log file. The probe never touches files, config, or the network.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct{ quit bool }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		if key := msg.(tea.KeyMsg).String(); key == "q" || key == "ctrl+c" {
			m.quit = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		fmt.Printf("t=%s SIZE %dx%d\n", time.Since(start).Round(time.Millisecond), msg.(tea.WindowSizeMsg).Width, msg.(tea.WindowSizeMsg).Height)
	case tea.MouseMsg:
		x := msg.(tea.MouseMsg)
		fmt.Printf("t=%s MOUSE button=%v action=%v x=%d y=%d\n",
			time.Since(start).Round(time.Millisecond), x.Button, x.Action, x.X, x.Y)
	}
	return m, nil
}

func (m model) View() string {
	return "\n  mihani-probe: mouse capture is ON.\n" +
		"  click · drag · scroll, then press q (auto-exits in 25s).\n" +
		"  every event is logged — see the path printed at the end.\n"
}

var start = time.Now()

func main() {
	out := filepath.Join(os.TempDir(), "mihani-probe.log")
	logf, err := os.Create(out)
	if err != nil {
		fmt.Println("cannot create log:", err)
		os.Exit(1)
	}
	// Mirror output to the log file as well.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	p := tea.NewProgram(model{},
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	go func() { _, _ = logf.ReadFrom(r) }()
	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(25 * time.Second):
	}
	_ = w.Close()
	_ = logf.Close()
	os.Stdout = oldStdout
	fmt.Println("PROBE DONE. Log written to: " + out)
	fmt.Println("Share that file (or paste its contents) so the input path can be diagnosed.")
	_ = strings.TrimSpace
}
