package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func BenchmarkRefreshViewLongTranscript(b *testing.B) {
	m := newTestModel(120, 40)
	long := ""
	for i := 0; i < 40; i++ {
		long += fmt.Sprintf("Paragraph %d with **bold**, `code`, and a list:\n\n- item one\n- item two\n\n", i)
	}
	for i := 0; i < 60; i++ {
		m.blocks = append(m.blocks, &block{kind: blockAssistant, content: long, finalized: true})
	}
	m.relayout()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.refreshView()
	}
}

func BenchmarkMouseMoveFullRepaint(b *testing.B) {
	m := newTestModel(120, 40)
	long := ""
	for i := 0; i < 40; i++ {
		long += fmt.Sprintf("Paragraph %d with **bold**, `code`, and a list:\n\n- item one\n- item two\n\n", i)
	}
	for i := 0; i < 60; i++ {
		m.blocks = append(m.blocks, &block{kind: blockAssistant, content: long, finalized: true})
	}
	m.relayout()
	m.refreshView()
	m.selOn = true
	m.selDrag = true
	m.selA = selPos{row: 5, col: 1}
	m.selH = selPos{row: 5, col: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.mouseMove(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 5, Y: 6})
	}
}