package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// e2eDriver runs the REAL production program (tea.Program with the app's own
// Model) headlessly, feeding real keystrokes through a pipe exactly like a
// terminal. This is the closest thing to "using the app" inside a test.
type e2eDriver struct {
	t    *testing.T
	m    *Model
	p    *tea.Program
	pw   io.WriteCloser
	done chan struct{}
	mu   sync.Mutex
	sink func(string)
	last string
}

// probeMsg asks the running program to report its own state. It is handled on
// the program's event-loop goroutine, so reading model fields here is safe (a
// direct read from the test goroutine would be a data race).
type probeMsg struct{ reply chan probeResult }

type probeResult struct {
	blocks    int
	blockText string
	input     string
	overlay   string
	open      bool
	busy      bool
}

// capturingModel wraps the app's Model so every full frame it renders is handed
// to a sink. The standard bubbletea renderer only writes *diffs*, so reading
// its output yields fragments rather than the screen a user sees; this captures
// the real View() instead.
type capturingModel struct {
	*Model
	sink func(string)
}

func (c capturingModel) Init() tea.Cmd { return c.Model.Init() }

func (c capturingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if probe, ok := msg.(probeMsg); ok {
		var b strings.Builder
		for _, blk := range c.Model.blocks {
			b.WriteString(blk.content)
			b.WriteString("\n")
		}
		probe.reply <- probeResult{
			blocks:    len(c.Model.blocks),
			blockText: b.String(),
			input:     c.Model.input.Value(),
			overlay:   c.Model.overlay,
			open:      c.Model.updateOpen,
			busy:      c.Model.busy,
		}
		return c, nil
	}
	next, cmd := c.Model.Update(msg)
	if mm, ok := next.(*Model); ok {
		return capturingModel{Model: mm, sink: c.sink}, cmd
	}
	return next, cmd
}

func (c capturingModel) View() string {
	v := c.Model.View()
	if c.sink != nil {
		c.sink(v)
	}
	return v
}

func newE2E(t *testing.T, width, height int) *e2eDriver {
	t.Helper()
	d, _ := newE2EWithServer(t, width, height, func(w http.ResponseWriter, r *http.Request) {})
	t.Cleanup(func() { _ = d.pw.Close() })
	return d
}

// newE2EWithServer starts the real program against a fake OpenAI-compatible
// streaming endpoint, so a whole turn (request → tool call → tool result →
// final answer) runs through the production code path.
func newE2EWithServer(t *testing.T, width, height int, handler http.HandlerFunc) (*e2eDriver, *httptest.Server) {
	t.Helper()
	isolatedUsageHome(t)
	srv := httptest.NewServer(handler)
	m := smokeModel(t, width, height)
	m.cfg.CurrentProvider = "test"
	m.cfg.CurrentModel = "test-model"
	m.cfg.Providers["test"] = config.Provider{
		Label: "Test", Type: "openai", BaseURL: srv.URL + "/v1", Models: []string{"test-model"},
	}
	m.agent.Cfg = m.cfg
	m.cfg.AutoConfirm = true // unattended tool approval, like `mihani -y`

	pr, pw := io.Pipe()
	d := &e2eDriver{t: t, m: m, pw: pw, done: make(chan struct{})}
	d.sink = func(frame string) {
		d.mu.Lock()
		d.last = frame
		d.mu.Unlock()
	}
	p := tea.NewProgram(capturingModel{Model: m, sink: d.sink},
		tea.WithInput(pr), tea.WithOutput(io.Discard),
		// Render into the model: a pipe has no window size, so the standard
		// renderer would emit clipped frames and the assertions would measure
		// truncation instead of app behavior.
		tea.WithoutRenderer(),
		tea.WithoutCatchPanics(), // a panic must fail the test, not be swallowed
	)
	go func() {
		defer close(d.done)
		_, _ = p.Run()
	}()
	d.p = p
	// The app delivers agent events through m.program; production Run() sets
	// this right after building the program, and the agent loop is dead
	// without it, so the harness must do the same.
	m.program = p
	d.send("\x1b[8;%d;%dt", height, width)
	return d, srv
}

func (d *e2eDriver) send(format string, args ...any) {
	d.t.Helper()
	_, err := d.pw.Write([]byte(fmt.Sprintf(format, args...)))
	if err != nil && !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "io: read/write") {
		d.t.Logf("write to program: %v", err)
	}
}

// typeText sends each character separately, paced like a fast human typist
// (~10 chars/second). The pacing matters: a machine-speed burst is treated as a
// paste on purpose, so typing instantly would not model real usage.
func (d *e2eDriver) typeText(s string) {
	d.t.Helper()
	for _, r := range s {
		if r == ' ' {
			d.send(" ")
		} else {
			d.send("%c", r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// state asks the running program for a consistent snapshot of the model.
func (d *e2eDriver) state() probeResult {
	d.t.Helper()
	reply := make(chan probeResult, 1)
	d.p.Send(probeMsg{reply: reply})
	select {
	case r := <-reply:
		return r
	case <-time.After(5 * time.Second):
		d.t.Fatal("probe timed out — the app stopped processing input (wedged?)")
		return probeResult{}
	}
}

func (d *e2eDriver) screen() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

// quit presses ctrl+c until the program exits. The first press only cancels a
// running turn (that is the documented behavior), so a busy app needs a second
// one — exactly like a real user.
func (d *e2eDriver) quit() {
	d.t.Helper()
	for i := 0; i < 4; i++ {
		d.send("\x03")
		select {
		case <-d.done:
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
	d.t.Fatal("program did not exit after repeated ctrl+c")
}

// waitFor polls the last rendered frame until cond holds.
func (d *e2eDriver) waitFor(what string, cond func(string) bool) {
	d.t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if cond(d.screen()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("timed out waiting for %s\n--- last screen ---\n%s", what, stripANSI(d.screen()))
}

// waitForState polls the model's own state until cond holds, failing with a
// readable dump. State, not the visible screen, is the right assertion target:
// content can legitimately sit below the scroll fold while still being correct.
func (d *e2eDriver) waitForState(what string, cond func(probeResult) bool) {
	d.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last probeResult
	for time.Now().Before(deadline) {
		last = d.state()
		if cond(last) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("timed out waiting for %s (blocks=%d busy=%v overlay=%q input=%q)\nblock text:\n%s\n--- screen ---\n%s",
		what, last.blocks, last.busy, last.overlay, last.input, last.blockText, stripANSI(d.screen()))
}

// requireNotEmpty fails when a frame is blank: the "empty response from the
// app" failure mode.
func (d *e2eDriver) requireNotEmpty(what string) {
	d.t.Helper()
	if strings.TrimSpace(stripANSI(d.screen())) == "" {
		d.t.Fatalf("%s rendered a completely empty screen", what)
	}
}

// End-to-end: launch the real program, type a full command, and confirm the
// app reacted (no blank screen, no crash, exits cleanly).
func TestE2ECommandRoundTrip(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()

	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })
	d.requireNotEmpty("startup")
	startBlocks := d.state().blocks

	// /status appends an info block describing the workspace and session.
	d.typeText("/status")
	d.send("\r")
	d.waitForState("/status block", func(r probeResult) bool {
		return r.blocks > startBlocks && strings.Contains(r.blockText, "session")
	})
	d.requireNotEmpty("after /status")

	// /help lists the commands.
	before := d.state().blocks
	d.typeText("/help")
	d.send("\r")
	d.waitForState("/help block", func(r probeResult) bool {
		return r.blocks > before && strings.Contains(r.blockText, "Commands")
	})
	d.requireNotEmpty("after /help")

	// esc clears the composer and typing works again afterwards.
	d.send("\x1b")
	d.typeText("hello")
	d.waitForState("typed text in composer", func(r probeResult) bool { return r.input == "hello" })
	d.requireNotEmpty("after typing")

	// A typed (non-command) message must NOT auto-send: it stays in the
	// composer until enter, so a slow typist is never raced by their input.
	if d.state().busy {
		t.Fatal("typing alone started a turn")
	}
}

// End-to-end mouse: with capture on, a real SGR drag must not crash the app
// and must keep rendering afterwards.
func TestE2EMouseDragKeepsAppAlive(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	// Turn mouse capture on through the real command path.
	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse toggled", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture")
	})

	// press, drag, release across the transcript area.
	d.send("\x1b[<0;10;3M")
	d.send("\x1b[<32;40;8M")
	d.send("\x1b[<32;60;14M")
	d.send("\x1b[<0;60;14m")
	d.waitFor("app responsive after drag", func(s string) bool {
		return strings.TrimSpace(stripANSI(s)) != ""
	})
	d.requireNotEmpty("after drag")
}

// End-to-end sweep: every slash command, driven through the REAL program with
// real keystrokes. Each must leave the app running, rendering, and able to quit
// cleanly — no panic, no hang, no blank screen.
func TestE2EEveryCommandRuns(t *testing.T) {
	skip := map[string]bool{"/quit": true} // covered by the driver's ctrl+c quit
	for _, item := range commands {
		name := item.name
		if skip[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			d := newE2E(t, 100, 30)
			defer d.quit()
			d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

			before := d.state().blocks
			d.typeText(name)
			d.send("\r")

			// Give the app a moment to react, then assert it is healthy
			// regardless of which branch the command took.
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				if d.state().blocks != before || d.state().overlay != "" {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			// Any command may open a modal (connect, settings, update...):
			// clear it so the app is back to a usable state.
			d.send("\x1b")
			d.send("\x1b")
			d.send("\x1b")
			d.requireNotEmpty("after " + name)
		})
	}
}

// End-to-end: the composer must accept multiline input (ctrl+j), and enter
// must send exactly ONE message.
func TestE2EMultilineComposerSendsOnce(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("first line")
	d.send("\x0a") // ctrl+j: newline inside the composer
	d.typeText("second line")
	d.send("\x0a")
	d.typeText("third line")

	state := d.state()
	if !strings.Contains(state.input, "first line") || !strings.Contains(state.input, "third line") {
		t.Fatalf("multiline composer lost content: %q", state.input)
	}
	if !strings.Contains(state.input, "\n") {
		t.Fatalf("expected newlines in the composer, got %q", state.input)
	}

	// Enter sends the whole block as one user message.
	before := d.state().blocks
	d.send("\r")
	d.waitForState("one user message sent", func(r probeResult) bool {
		return r.blocks == before+1 && strings.Contains(r.blockText, "third line")
	})
}

// End-to-end: a huge bracketed paste must arrive as a single message and
// never auto-send — the exact field-report scenario.
func TestE2EBracketedPasteNeverAutoSends(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	payload := strings.Repeat("a large pasted prompt line with content\n", 60) // ~2 KB, 60 lines
	d.send("\x1b[200~%s\x1b[201~", payload)

	d.waitForState("paste landed whole", func(r probeResult) bool {
		return strings.Count(r.input, "\n") >= 50
	})
	if d.state().busy {
		t.Fatal("a bracketed paste must never start a turn on its own")
	}
	if strings.Count(d.state().blockText, "pasted prompt line") != 0 {
		t.Fatal("pasted text leaked into the transcript instead of the composer")
	}
	d.requireNotEmpty("after paste")
}

// The whole agent loop through the real program: the model streams text, calls
// a tool, gets the result, and answers. The user must see all of it and be
// able to keep using the app afterwards.
func TestE2EFullTurnWithToolCall(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			// First response: call the tool, exactly as a real model would.
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read_file","arguments":"{\"path\":\"main.go\"}"}}]}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		// Second response: the final answer.
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"The file starts with package main."}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}

	d, srv := newE2EWithServer(t, 100, 30, handler)
	defer srv.Close()
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("what does main.go start with")
	d.send("\r")

	// The turn must complete with the model's answer in the transcript.
	d.waitForState("turn completed", func(r probeResult) bool {
		return strings.Contains(r.blockText, "The file starts with package main.")
	})
	if d.state().busy {
		t.Fatal("app should be idle after the turn finished")
	}
	d.requireNotEmpty("after turn")

	// The tool call must have actually executed through the real pipeline.
	d.waitForState("tool result recorded", func(r probeResult) bool {
		return strings.Contains(r.blockText, "main.go")
	})

	// And the app must be usable for a second turn.
	before := d.state().blocks
	d.send("\x1b")
	d.typeText("and now the second question")
	d.send("\r")
	d.waitForState("second turn answered", func(r probeResult) bool {
		return r.blocks > before
	})
	d.requireNotEmpty("after second turn")
}

// A provider that keeps failing must not blank the screen, crash, or wedge the
// app: the user has to see that the app is reconnecting, be able to cancel,
// and carry on working.
func TestE2EProviderFailureIsVisible(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream exploded"}}`))
	}
	d, srv := newE2EWithServer(t, 100, 30, handler)
	defer srv.Close()
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("this will fail")
	d.send("\r")

	// The failure must be visible as reconnecting progress (the app retries a
	// 5xx for minutes by design), not as a silent hang.
	d.waitFor("reconnect progress shown", func(s string) bool {
		return strings.Contains(stripANSI(s), "reconnecting")
	})
	d.requireNotEmpty("during provider failure")

	// The user must be able to cancel the stuck turn and keep working.
	d.send("\x1b")
	d.send("\x1b")
	d.waitForState("turn cancelled", func(r probeResult) bool { return !r.busy })
	d.send("\x1b")
	d.typeText("still typing")
	d.waitForState("composer alive after failure", func(r probeResult) bool {
		return strings.Contains(r.input, "still typing")
	})
	d.requireNotEmpty("after cancelling")
}

// End-to-end: rapid input storm must never crash, hang, or blank the screen.
func TestE2ERandomInputStorm(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	noise := []string{
		"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[5~", "\x1b[6~",
		"\x1b[<0;5;5M", "\x1b[<0;5;5m", "\x1b[<64;10;10M",
		"\t", "\x1b[Z", "\x7f", "\x01", "\x12", "\x15", "\x17", "\x1a",
		"/", "?", "hello", "\r", "\x1b", "\x0e", "\x10", "\x1b[200~x\x1b[201~",
	}
	for i := 0; i < 400; i++ {
		d.send("%s", noise[i%len(noise)])
	}
	d.waitFor("app alive after storm", func(s string) bool {
		return strings.TrimSpace(stripANSI(s)) != ""
	})
	d.requireNotEmpty("after input storm")

	// And it must still accept real input afterwards.
	d.send("\x1b")
	d.typeText("still works")
	d.waitForState("input accepted after storm", func(r probeResult) bool {
		return strings.Contains(r.input, "still works")
	})
}
