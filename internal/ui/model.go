// Package ui implements the interactive Bubble Tea list used to review and
// select outdated Homebrew packages before upgrading.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/garthdw/homebrew-changes/internal/changelog"
	"github.com/garthdw/homebrew-changes/internal/ghsource"
)

// Item is one outdated package in the list.
type Item struct {
	Name      string
	Kind      string // "formula" or "cask"
	Installed string
	Current   string
	Owner     string // GitHub owner, empty if no repo could be resolved
	Repo      string // GitHub repo name

	Expanded bool
	Selected bool
	Loading  bool
	Body     string // rendered changelog/releases content, empty until fetched
}

// Result is returned by Run: the packages the user checked, and whether
// they asked to proceed with the upgrade.
type Result struct {
	Selected []Item
	Upgrade  bool
}

var (
	styleCursor    = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	styleSelected  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleUnselect  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleVersion   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleArrow     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleHeader    = lipgloss.NewStyle().Bold(true)
	styleBodyFrame = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginLeft(2)
)

type model struct {
	items    []Item
	cursor   int
	spinner  spinner.Model
	quitting bool
	upgrade  bool
}

func newModel(items []Item) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleDim
	return model{items: items, spinner: s}
}

// Run launches the interactive list for the given packages and blocks until
// the user quits or confirms an upgrade.
func Run(items []Item) (Result, error) {
	m := newModel(items)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm := finalModel.(model)

	var selected []Item
	for _, it := range fm.items {
		if it.Selected {
			selected = append(selected, it)
		}
	}
	return Result{Selected: selected, Upgrade: fm.upgrade}, nil
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

type bodyFetchedMsg struct {
	index int
	body  string
}

func fetchBody(index int, it Item) tea.Cmd {
	return func() tea.Msg {
		if it.Owner == "" {
			return bodyFetchedMsg{index: index, body: "No GitHub repository could be resolved for this package; changelog unavailable."}
		}

		var raw string
		var sourceLabel string
		if filename, content, ok := ghsource.FetchChangelogFile(it.Owner, it.Repo); ok {
			sourceLabel = fmt.Sprintf("(from %s)", filename)
			raw = changelog.TrimToRange(content, it.Current, it.Installed)
		} else if releases, err := ghsource.FetchReleases(it.Owner, it.Repo, it.Installed); err == nil && len(releases) > 0 {
			sourceLabel = "(from GitHub releases)"
			var b strings.Builder
			for _, r := range releases {
				fmt.Fprintf(&b, "### %s (%s)\n\n%s\n\n", r.TagName, orUnknown(r.PublishedAt), orPlaceholder(r.Body))
			}
			raw = b.String()
		} else {
			return bodyFetchedMsg{index: index, body: "No changelog file or GitHub releases found for this package."}
		}

		rendered := renderMarkdown(raw)
		return bodyFetchedMsg{index: index, body: sourceLabel + "\n\n" + rendered}
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown date"
	}
	return s
}

func orPlaceholder(s string) string {
	if s == "" {
		return "_no release notes_"
	}
	return s
}

func renderMarkdown(raw string) string {
	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(96))
	if err != nil {
		return raw
	}
	out, err := r.Render(raw)
	if err != nil {
		return raw
	}
	return strings.TrimRight(out, "\n")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			it := &m.items[m.cursor]
			it.Expanded = !it.Expanded
			if it.Expanded && it.Body == "" && !it.Loading {
				it.Loading = true
				return m, fetchBody(m.cursor, *it)
			}

		case " ":
			if len(m.items) > 0 {
				m.items[m.cursor].Selected = !m.items[m.cursor].Selected
			}

		case "a":
			allSelected := true
			for _, it := range m.items {
				if !it.Selected {
					allSelected = false
					break
				}
			}
			for i := range m.items {
				m.items[i].Selected = !allSelected
			}

		case "u":
			m.upgrade = true
			return m, tea.Quit
		}

	case bodyFetchedMsg:
		if msg.index >= 0 && msg.index < len(m.items) {
			m.items[msg.index].Body = msg.body
			m.items[msg.index].Loading = false
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("%d outdated package(s)", len(m.items))))
	b.WriteString("\n")
	b.WriteString(styleDim.Render("↑/↓ navigate · enter expand/collapse · space select · a select all · u upgrade selected · q quit"))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = styleArrow.Render("> ")
		}

		checkbox := styleUnselect.Render("[ ]")
		if it.Selected {
			checkbox = styleSelected.Render("[x]")
		}

		sourceHint := styleDim.Render("[no changelog source]")
		if it.Owner != "" {
			sourceHint = styleDim.Render(fmt.Sprintf("[%s/%s]", it.Owner, it.Repo))
		}

		line := fmt.Sprintf("%s%s %s (%s)  %s -> %s  %s",
			cursor, checkbox, it.Name, it.Kind,
			styleVersion.Render(it.Installed), styleVersion.Render(it.Current),
			sourceHint)
		b.WriteString(line)
		b.WriteString("\n")

		if it.Expanded {
			var content string
			switch {
			case it.Loading:
				content = m.spinner.View() + " fetching changelog..."
			case it.Body != "":
				content = it.Body
			default:
				content = styleDim.Render("(empty)")
			}
			b.WriteString(styleBodyFrame.Render(content))
			b.WriteString("\n")
		}
	}

	return b.String()
}
