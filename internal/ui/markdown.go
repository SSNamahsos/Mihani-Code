package ui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

var (
	mdMu       sync.Mutex
	mdRenderer = map[int]*glamour.TermRenderer{}
)

// markdownWidth caps rendered markdown for readability on very wide terminals.
func markdownWidth(width int) int {
	if width < 20 {
		width = 20
	}
	if width > 100 {
		width = 100
	}
	return width - 2
}

// renderMarkdown converts markdown to styled terminal output at the given width.
func renderMarkdown(src string, width int) string {
	w := markdownWidth(width)
	mdMu.Lock()
	defer mdMu.Unlock()
	r, ok := mdRenderer[w]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(w),
		)
		if err != nil {
			return src
		}
		mdRenderer[w] = r
	}
	out, err := r.Render(src)
	if err != nil {
		return src
	}
	return strings.Trim(out, "\n")
}
