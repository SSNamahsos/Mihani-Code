package ui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuf is a thread-safe sink for the program's raw output bytes, so a test
// can assert on the DECSET/DECRST sequences the app asked the terminal to
// switch on.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

var _ io.Writer = (*syncBuf)(nil)

// newE2ECapture drives the real program with the real renderer so the raw
// terminal control sequences it emits are observable.
func newE2ECapture(t *testing.T, width, height int) (*e2eDriver, *syncBuf) {
	t.Helper()
	out := &syncBuf{}
	d, _ := newE2EWithServerOutput(t, width, height, out, true, func(w http.ResponseWriter, r *http.Request) {})
	return d, out
}

// sgrMouse is the report a terminal sends when SGR mouse encoding (1006) is
// active — plain ASCII, self-delimiting, safe to hand to a stream parser.
func sgrMouse(button, col, row int, release bool) string {
	final := 'M'
	if release {
		final = 'm'
	}
	return fmt.Sprintf("\x1b[<%d;%d;%d%c", button, col, row, final)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// Turning mouse capture on at runtime must switch the terminal to SGR mouse
// encoding (1006) as well as button-event tracking (1002), exactly like the
// startup path does.
//
// Without 1006 the terminal falls back to the legacy X10 encoding, "ESC [ M"
// plus three raw bytes that are each a coordinate offset by 32. Those bytes are
// printable, and "ESC [ M" is a complete CSI on its own, so any report that
// arrives split across two reads is mis-parsed and the payload lands in the
// composer as typed junk — the reported "####@@!".
func TestMouseToggleEnablesSGR(t *testing.T) {
	d, out := newE2ECapture(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	mark := len(out.String())
	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse toggled", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture")
	})
	time.Sleep(400 * time.Millisecond)

	after := out.String()[mark:]
	if !strings.Contains(after, "\x1b[?1002h") {
		t.Errorf("/mouse did not enable button-event mouse tracking (1002)\nemitted:\n%q", tail(after, 800))
	}
	if !strings.Contains(after, "\x1b[?1006h") {
		t.Errorf("/mouse did not enable SGR mouse encoding (1006), so the terminal reports in X10 mode and its raw payload bytes reach the composer as junk\nemitted:\n%q", tail(after, 800))
	}
}

// Toggling capture back off must hand the terminal's native text selection
// back to the user: every mouse mode the app may have turned on has to be
// reset, not just the one bubbletea remembers.
func TestMouseToggleOffReleasesEveryMode(t *testing.T) {
	d, out := newE2ECapture(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse capture on", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture: on")
	})

	mark := len(out.String())
	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse capture off", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture: off")
	})
	time.Sleep(400 * time.Millisecond)

	after := out.String()[mark:]
	for _, seq := range []string{"\x1b[?1002l", "\x1b[?1003l", "\x1b[?1006l", "\x1b[?1015l"} {
		if !strings.Contains(after, seq) {
			t.Errorf("turning capture off did not emit %q, so the terminal stays in that mouse mode and its native text selection stays hijacked\nemitted:\n%q", seq, tail(after, 800))
		}
	}
}

// The reported field bug, end to end: with capture on, clicking a message opens
// its action menu and leaves the composer untouched — no mouse report bytes
// leaking into the chat box.
func TestMouseClickOpensMenuWithoutTypingJunk(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse toggled", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture")
	})

	// A real click: press, a little pointer motion while held, release.
	d.send("%s", sgrMouse(0, 20, 5, false))
	time.Sleep(40 * time.Millisecond)
	d.send("%s", sgrMouse(32, 20, 5, false))
	time.Sleep(40 * time.Millisecond)
	d.send("%s", sgrMouse(32, 21, 5, false))
	time.Sleep(40 * time.Millisecond)
	d.send("%s", sgrMouse(0, 21, 5, true))

	d.waitForState("message menu opened by click", func(r probeResult) bool {
		return r.overlay != ""
	})
	if v := d.state().input; v != "" {
		t.Fatalf("mouse traffic leaked into the composer: %q", v)
	}
	d.requireNotEmpty("after click")
}

// With a modal open the app must drop mouse traffic instead of letting it fall
// through to the composer.
func TestMouseEventsNeverReachComposerWhileModalOpen(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse toggled", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture")
	})

	// Open a modal that owns the screen, then click around inside it.
	d.typeText("/export")
	d.send("\r")
	d.waitForState("export menu open", func(r probeResult) bool { return r.overlay != "" })

	for _, y := range []int{3, 6, 9, 12} {
		d.send("%s", sgrMouse(0, 20, y, false))
		time.Sleep(30 * time.Millisecond)
		d.send("%s", sgrMouse(0, 20, y, true))
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)

	if v := d.state().input; v != "" {
		t.Fatalf("mouse traffic leaked into the composer while a modal was open: %q", v)
	}
}

// A drag used to re-render the entire transcript on every pointer motion
// event, twice over (the auto-scroll helper repainted on its own as well).
// Button-event tracking streams one motion message per cell the pointer
// crosses, so a single drag queued hundreds of full re-renders and the UI
// locked up — the "the terminal crashes when I run /mouse" report.
func TestMouseMotionStormStaysResponsive(t *testing.T) {
	d := newE2E(t, 100, 30)
	defer d.quit()
	d.waitFor("first frame", func(s string) bool { return strings.TrimSpace(stripANSI(s)) != "" })

	d.typeText("/mouse")
	d.send("\r")
	d.waitForState("mouse toggled", func(r probeResult) bool {
		return strings.Contains(r.blockText, "mouse capture")
	})

	// Arm a drag, then flood the app with motion events the way a terminal
	// streams them while a button is held.
	d.send("%s", sgrMouse(0, 10, 5, false))
	start := time.Now()
	for i := 0; i < 2000; i++ {
		d.send("%s", sgrMouse(32, 10+i%80, 5+(i/80)%20, false))
	}
	d.send("%s", sgrMouse(0, 10, 5, true))
	elapsed := time.Since(start)

	// The app must still be answering within the driver's normal budget; if
	// each motion event triggered a full transcript re-render, the probe below
	// would time out and the app would look frozen.
	d.waitForState("app responsive after motion storm", func(r probeResult) bool { return r.blocks > 0 })
	t.Logf("2000 motion events processed in %s", elapsed)

	// And the drag must still have selected and copied something.
	d.waitFor("selection notice after storm", func(s string) bool {
		return strings.Contains(stripANSI(s), "Copied")
	})
	d.requireNotEmpty("after motion storm")
}