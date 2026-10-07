package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type previewEntry struct {
	edit     *agent.Edit
	activity *agent.Activity
	err      error
	loaded   bool
	checked  time.Time
}

type previewResult struct {
	key      sessionIdentity
	revision int
	edit     *agent.Edit
	activity *agent.Activity
	err      error
}

type previewTick struct{}

const previewInterval = time.Second

// WithPreviewSources enables optional, selected-session-only detail loading.
func WithPreviewSources(adapters ...agent.Adapter) ModelOption {
	return func(model *Model) {
		model.previewSources = make(map[string]agent.PreviewSource)
		for _, adapter := range adapters {
			if source, ok := adapter.(agent.PreviewSource); ok {
				model.previewSources[adapter.Provider()] = source
			}
		}
	}
}

func previewTimer() tea.Cmd {
	return tea.Tick(previewInterval, func(time.Time) tea.Msg { return previewTick{} })
}

// Update synchronizes previews after every selection-changing event, including
// sorting, sidebar filtering, and provider updates that remove a session.
func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if model.previewCache == nil {
		model.previewCache = make(map[sessionIdentity]previewEntry)
	}
	var command tea.Cmd
	switch message := message.(type) {
	case previewResult:
		if message.key != model.previewKey || message.revision != model.previewRevision {
			return model, nil
		}
		model.previewCancel = nil
		cached := model.previewCache[message.key]
		cached.loaded = true
		cached.err = message.err
		cached.checked = time.Now()
		if message.err == nil {
			// The reader retains previous records; nil now means the transcript reset.
			cached.edit = message.edit
			cached.activity = message.activity
		}
		model.previewCache[message.key] = cached
	case previewTick:
		command = previewTimer()
	case tea.KeyMsg:
		// Search owns text input, including the preview shortcut.
		if model.projectSearching {
			next, cmd := model.update(message)
			model = next.(Model)
			command = cmd
			break
		}
		if message.String() == "v" {
			model.previewExpanded = true
			model.previewScroll = 0
			command := model.syncPreview()
			return model, command
		}
		if model.previewExpanded {
			switch message.String() {
			case "esc":
				model.previewExpanded = false
				return model, nil
			case "j", "down":
				model.previewScroll++
				return model, nil
			case "k", "up":
				model.previewScroll = max(0, model.previewScroll-1)
				return model, nil
			case "q", "ctrl+c":
			default:
				return model, nil
			}
		}
		if message.String() == "q" || message.String() == "ctrl+c" {
			if model.previewCancel != nil {
				model.previewCancel()
			}
			return model.update(message)
		}
		next, cmd := model.update(message)
		model = next.(Model)
		command = cmd
	default:
		next, cmd := model.update(message)
		model = next.(Model)
		command = cmd
	}
	previewCommand := model.syncPreview()
	return model, tea.Batch(command, previewCommand)
}

func (model *Model) syncPreview() tea.Cmd {
	session, ok := model.selectedSession()
	key := sessionIdentity{provider: session.Provider, id: session.ID}
	if key != model.previewKey {
		if model.previewCancel != nil {
			model.previewCancel()
			model.previewCancel = nil
		}
		model.previewKey = key
		model.previewRevision++
		model.previewScroll = 0
	}
	if !ok || model.previewCancel != nil {
		return nil
	}
	source := model.previewSources[session.Provider]
	if source == nil {
		return nil
	}
	cached := model.previewCache[key]
	if !cached.checked.IsZero() && time.Since(cached.checked) < previewInterval {
		return nil
	}
	ctx, cancel := context.WithTimeout(model.appContext, requestTimeout)
	model.previewCancel = cancel
	model.previewRevision++
	revision := model.previewRevision
	return func() tea.Msg {
		defer cancel()
		preview, err := source.LatestPreview(ctx, session)
		return previewResult{key: key, revision: revision, edit: preview.Edit, activity: preview.Activity, err: err}
	}
}

func (model Model) previewView(width, height int, expanded bool) string {
	width = max(1, width)
	height = max(1, height)
	session, ok := model.selectedSession()
	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
	if model.showActivity(session, cached) {
		return model.activityView(session, cached, width, height, expanded)
	}
	if cached.edit == nil {
		return model.emptyPreviewView(model.previewStatus(session, ok, cached), width, height, expanded)
	}
	title := "LAST EDIT"
	if expanded {
		title = "EDIT PREVIEW · Esc: return · j/k: scroll"
	}
	lines := []string{accentStyle.Render(ansi.Truncate(title, width, "…"))}
	edit := cached.edit
	name := safeDisplayText(edit.Filename)
	if expanded {
		lines[0] += mutedStyle.Render(ansi.Truncate(" · checked "+cached.checked.Format("15:04:05"), max(0, width-lipgloss.Width(lines[0])), "…"))
	}
	age := formatLastActiveAt(edit.CompletedAt, time.Now())
	if cached.err != nil || session.Stale || time.Since(cached.checked) > 5*time.Second {
		age += " · stale"
	}
	if !expanded {
		lines = []string{alignedLine(accentStyle.Render("LAST EDIT"), mutedStyle.Render(age), width)}
	}
	lines = append(lines, mutedStyle.Render(ansi.Truncate(name, width, "…")))
	if expanded {
		lines = append(lines, mutedStyle.Render(ansi.Truncate(age, width, "…")))
	}
	diff := strings.Split(strings.TrimSuffix(edit.Diff, "\n"), "\n")
	if edit.Diff == "" {
		diff = []string{"Preview unavailable"}
	}
	truncated := edit.Truncated
	if !expanded {
		for i := 1; i < len(diff); i++ {
			if strings.HasPrefix(diff[i], "@@") {
				diff = diff[:i]
				truncated = true
				break
			}
		}
		if len(diff) > 0 && strings.HasPrefix(diff[0], "@@") {
			lines = append(lines, accentStyle.Render(ansi.Truncate(diff[0], width, "…")))
			diff = diff[1:]
		}
		// Anchor the small box at the first changed line, keeping one leading
		// context line so a long hunk header does not hide the actual edit.
		for i, line := range diff {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				start := max(0, i-1)
				truncated = truncated || start > 0
				diff = diff[start:]
				break
			}
		}
	}
	offset := 0
	if expanded {
		offset = min(model.previewScroll, max(0, len(diff)-max(1, height-4)))
	}
	capacity := max(0, height-len(lines)-1)
	end := min(len(diff), offset+capacity)
	truncated = truncated || end < len(diff) || offset > 0
	for _, line := range diff[offset:end] {
		line = safeDisplayText(strings.ReplaceAll(line, "\t", "  "))
		if ansi.StringWidth(line) > width {
			truncated = true
		}
		style := mainTextStyle
		if strings.HasPrefix(line, "+") {
			style = runningStyle
		}
		if strings.HasPrefix(line, "-") {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorError))
		}
		lines = append(lines, style.Render(ansi.Truncate(line, width, "…")))
	}
	if !expanded {
		added, removed := 0, 0
		for _, line := range strings.Split(edit.Diff, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ ") {
				added++
			}
			if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- ") {
				removed++
			}
		}
		for len(lines) < height-1 {
			lines = append(lines, "")
		}
		label := fmt.Sprintf("+%d −%d", added, removed)
		if truncated {
			label += " … truncated"
		}
		lines = append(lines, alignedLine(mutedStyle.Render(label), shortcutKeyStyle.Render("v")+mutedStyle.Render(" expand"), width))
	} else if truncated {
		label := "… truncated · v: expand"
		if expanded {
			label = fmt.Sprintf("… truncated · lines %d–%d/%d", offset+1, end, len(diff))
		}
		lines = append(lines, mutedStyle.Render(ansi.Truncate(label, width, "…")))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func clipLines(text string, width int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, width), "…")
	}
	return strings.Join(lines, "\n")
}

func (model Model) detailWithPreview() string {
	// The preview border ends at the same column as the panel's separator.
	width := max(1, model.table.Width()-detailStyle.PaddingRight(0).GetHorizontalFrameSize())
	// Below this width metadata and a readable hunk cannot share a row. Keep a
	// one-line preview status, with the same expansion shortcut, under metadata.
	if width < 64 {
		session, selected := model.selectedSession()
		cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
		label := model.previewStatus(session, selected, cached)
		if cached.edit != nil {
			status := formatLastActiveAt(cached.edit.CompletedAt, time.Now())
			if cached.err != nil || session.Stale || time.Since(cached.checked) > 5*time.Second {
				status = "stale"
			}
			nameWidth := max(1, width-ansi.StringWidth(status)-16)
			label = "v: expand · " + status + " · " + ansi.Truncate(safeDisplayText(filepath.Base(cached.edit.Filename)), nameWidth, "…")
		}
		if model.showActivity(session, cached) {
			label = "v · " + cached.activity.Label + ": " + safeDisplayText(cached.activity.Text)
		}
		return clipLines(model.detailView(), width) + "\n" + mutedStyle.Render(ansi.Truncate(safeDisplayText(label), width, "…"))
	}
	previewWidth := min(42, width*3/8)
	metadataWidth := width - previewWidth - 1
	metadataLines := strings.Split(model.detailView(), "\n")
	metadataLines[0] = model.detailHeadingAtWidth(metadataWidth)
	// The preview's top border occupies a row. Inset the metadata by that
	// row so the two heading texts share a baseline.
	metadata := lipgloss.NewStyle().
		Width(metadataWidth).
		Render("\n" + clipLines(strings.Join(metadataLines, "\n"), metadataWidth))
	previewStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorDivider))
	preview := previewStyle.Render(model.previewView(previewWidth-previewStyle.GetHorizontalFrameSize(), model.inlinePreviewHeight(), false))
	return lipgloss.JoinHorizontal(lipgloss.Top, metadata, " ", preview)
}
