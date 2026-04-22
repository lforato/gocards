package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

const previewMaxLines = 10

func previewCard(in store.CardInput, width int) string {
	rows := []string{
		tui.StyleMuted.Render("type") + "  " + typeBadge(in.Type, true) + "   " +
			tui.StyleMuted.Render("lang") + "  " + in.Language,
		"",
		tui.StyleMuted.Render("prompt"),
		previewBlock(in.Prompt, "(empty)", width),
	}
	rows = append(rows, cardSpecificPreview(in, width)...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func cardSpecificPreview(in store.CardInput, width int) []string {
	switch in.Type {
	case models.CardMCQ:
		return previewMCQChoices(in.Choices)
	case models.CardFill:
		return previewFillTemplate(in.BlanksData, width)
	case models.CardCode, models.CardExp:
		return previewExpected(in.ExpectedAnswer, width)
	}
	return nil
}

func previewMCQChoices(choices []models.Choice) []string {
	rows := []string{"", tui.StyleMuted.Render("choices")}
	for _, ch := range choices {
		mark := "[ ]"
		if ch.IsCorrect {
			mark = tui.StyleSuccess.Render("[x]")
		}
		rows = append(rows, fmt.Sprintf("  %s %s. %s", mark, ch.ID, ch.Text))
	}
	return rows
}

func previewFillTemplate(blanks *models.BlankData, width int) []string {
	if blanks == nil {
		return nil
	}
	return []string{
		"", tui.StyleMuted.Render("template"),
		previewBlock(blanks.Template, "(empty)", width),
		"", tui.StyleMuted.Render("blanks: " + strings.Join(blanks.Blanks, ", ")),
	}
}

func previewExpected(expected string, width int) []string {
	if expected == "" {
		return nil
	}
	return []string{"", tui.StyleMuted.Render("expected answer"), previewBlock(expected, "", width)}
}

// previewBlock wraps content in a rounded border constrained to totalWidth.
// Content past previewMaxLines is clipped with an ellipsis. The explicit
// width is what keeps the border from running past the terminal edge.
func previewBlock(content, placeholder string, totalWidth int) string {
	if totalWidth < 10 {
		totalWidth = 10
	}
	innerW := max(4, totalWidth-4)

	raw := strings.TrimSpace(content)
	if raw == "" {
		raw = placeholder
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > previewMaxLines {
		lines = append(lines[:previewMaxLines], "…")
	}
	body := lipgloss.NewStyle().Width(innerW).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Width(innerW + 2).
		Render(body)
}
