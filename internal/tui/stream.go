package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lforato/gocards/internal/ai"
)

type streamChunkMsg struct{ text string }
type streamDoneMsg struct{ full string }
type streamErrMsg struct{ err error }

func pumpStream(ch <-chan ai.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamDoneMsg{}
		}
		if ev.Err != nil {
			return streamErrMsg{err: ev.Err}
		}
		if ev.Done {
			return streamDoneMsg{full: ev.Full}
		}
		return streamChunkMsg{text: ev.Chunk}
	}
}
