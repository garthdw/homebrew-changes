// Package ui implements the interactive Bubble Tea list used to review and
// select outdated Homebrew packages before upgrading.
package ui

import (
	"cmp"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/garthdw/homebrew-changes/internal/changelog"
	"github.com/garthdw/homebrew-changes/internal/ghsource"
	"github.com/garthdw/homebrew-changes/internal/homebrew"
)

// Item is one outdated package in the list.
type Item struct {
	Name      string
	Kind      string // "formula" or "cask"
	Installed string
	Current   string
	Homepage  string // package homepage, empty if unknown
	Owner     string // GitHub owner, empty if no repo could be resolved
	Repo      string // GitHub repo name

	Expanded bool
	Loading  bool
	Body     string // rendered changelog/releases content, empty until fetched

	// Upgrading, Upgraded, and UpgradeErr track an in-place single-package
	// upgrade triggered by the "u" key; unlike ActionUpgradeAll, this
	// happens without quitting the list so the changelog stays browsable.
	Upgrading  bool
	Upgraded   bool
	UpgradeErr string
}

// Action is what the user asked to do when they quit the list.
type Action int

const (
	// ActionNone means the user quit without upgrading anything.
	ActionNone Action = iota
	// ActionUpgradeAll means every outdated package should be upgraded.
	ActionUpgradeAll
)

// Result is returned by Run: what the user asked to do when they quit, and
// the final state of every item (reflecting any in-place single-package
// upgrades performed via the "u" key before quitting).
type Result struct {
	Action Action
	Items  []Item
}

var (
	styleVersion   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleArrow     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleHeader    = lipgloss.NewStyle().Bold(true)
	styleSelected  = lipgloss.NewStyle().Bold(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleBodyFrame = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginLeft(2)
)

const (
	headerLines = 2 // "Outdated packages:" + blank line
	footerLines = 2 // blank line + keybinding hint
)

type model struct {
	items     []Item
	cursor    int
	spinner   spinner.Model
	viewport  viewport.Model
	ready     bool
	quitting  bool
	action    Action
	itemLines []int // line offset of each item's header within the rendered content
}

func newModel(items []Item) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleDim
	return model{items: items, spinner: s}
}

// Run launches the interactive, fullscreen list for the given packages and
// blocks until the user quits or confirms an upgrade.
func Run(items []Item) (Result, error) {
	m := newModel(items)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm := finalModel.(model)
	return Result{Action: fm.action, Items: fm.items}, nil
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
				fmt.Fprintf(&b, "### %s (%s)\n\n%s\n\n", r.TagName, cmp.Or(r.PublishedAt, "unknown date"), cmp.Or(r.Body, "_no release notes_"))
			}
			raw = b.String()
		} else {
			return bodyFetchedMsg{index: index, body: "No changelog file or GitHub releases found for this package."}
		}

		rendered := renderMarkdown(raw)
		return bodyFetchedMsg{index: index, body: sourceLabel + "\n\n" + rendered}
	}
}

type upgradeDoneMsg struct {
	index int
	err   error
}

func upgradePackage(index int, it Item) tea.Cmd {
	return func() tea.Msg {
		var err error
		if it.Kind == "cask" {
			err = homebrew.UpgradeQuiet(nil, []string{it.Name})
		} else {
			err = homebrew.UpgradeQuiet([]string{it.Name}, nil)
		}
		return upgradeDoneMsg{index: index, err: err}
	}
}

// openURL opens url in the user's default browser via macOS's `open`.
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Start()
		return nil
	}
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

func (m *model) refreshViewport() {
	content, itemLines := m.renderList()
	m.itemLines = itemLines
	m.viewport.SetContent(content)
}

// scrollToCursor adjusts the viewport offset so the highlighted item's
// header line is visible, scrolling as little as possible.
func (m *model) scrollToCursor() {
	if m.cursor < 0 || m.cursor >= len(m.itemLines) {
		return
	}
	target := m.itemLines[m.cursor]
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target > m.viewport.YOffset+m.viewport.Height-1 {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		vpHeight := msg.Height - headerLines - footerLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		m.refreshViewport()
		m.scrollToCursor()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "up":
			m.moveCursor(-1)
			m.refreshViewport()

		case "down":
			m.moveCursor(1)
			m.refreshViewport()

		case "K":
			m.moveCursor(-1)
			m.refreshViewport()

		case "J":
			m.moveCursor(1)
			m.refreshViewport()

		case "k":
			if len(m.items) > 0 && m.items[m.cursor].Expanded {
				m.viewport.ScrollUp(1)
			} else {
				m.moveCursor(-1)
				m.refreshViewport()
			}

		case "j":
			if len(m.items) > 0 && m.items[m.cursor].Expanded {
				m.viewport.ScrollDown(1)
			} else {
				m.moveCursor(1)
				m.refreshViewport()
			}

		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			it := &m.items[m.cursor]
			it.Expanded = !it.Expanded
			if it.Expanded && it.Body == "" && !it.Loading {
				it.Loading = true
				m.refreshViewport()
				m.scrollToCursor()
				return m, fetchBody(m.cursor, *it)
			}
			m.refreshViewport()
			m.scrollToCursor()

		case "o":
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			url := it.Homepage
			if url == "" && it.Owner != "" {
				url = fmt.Sprintf("https://github.com/%s/%s", it.Owner, it.Repo)
			}
			if url == "" {
				return m, nil
			}
			return m, openURL(url)

		case "a":
			m.action = ActionUpgradeAll
			return m, tea.Quit

		case "u":
			if len(m.items) == 0 {
				return m, nil
			}
			it := &m.items[m.cursor]
			if it.Upgraded || it.Upgrading {
				return m, nil
			}
			it.Upgrading = true
			it.UpgradeErr = ""
			m.refreshViewport()
			m.scrollToCursor()
			return m, upgradePackage(m.cursor, *it)
		}

	case bodyFetchedMsg:
		if msg.index >= 0 && msg.index < len(m.items) {
			m.items[msg.index].Body = msg.body
			m.items[msg.index].Loading = false
			m.refreshViewport()
		}

	case upgradeDoneMsg:
		if msg.index >= 0 && msg.index < len(m.items) {
			it := &m.items[msg.index]
			it.Upgrading = false
			if msg.err != nil {
				it.UpgradeErr = msg.err.Error()
			} else {
				it.Upgraded = true
			}
			m.refreshViewport()
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		for _, it := range m.items {
			if it.Loading || it.Upgrading {
				m.refreshViewport()
				break
			}
		}
		return m, cmd
	}

	return m, nil
}

// moveCursor moves the highlighted item by delta and scrolls it into view.
func (m *model) moveCursor(delta int) {
	if len(m.items) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.items)-1 {
		m.cursor = len(m.items) - 1
	}
	m.scrollToCursor()
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder
	b.WriteString(styleHeader.Render("Outdated packages:"))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("[↑/↓, j/k, J/K] move  [enter] expand/collapse [o] open homepage  [a] upgrade all  [u] upgrade current  [q] quit"))

	return b.String()
}

// renderList builds the scrollable document: one line per package, with the
// expanded ones' changelog bodies inlined beneath them. Returns the content
// and the line offset of each item's header, used to scroll a given item
// into view.
func (m model) renderList() (string, []int) {
	var b strings.Builder
	itemLines := make([]int, len(m.items))
	line := 0

	writeLine := func(s string) {
		b.WriteString(s)
		b.WriteString("\n")
		line += strings.Count(s, "\n") + 1
	}

	for i, it := range m.items {
		itemLines[i] = line

		// Styled per-segment (rather than rendering the whole line and
		// wrapping it in an outer style) because lipgloss.Render emits a
		// full SGR reset at the end of each call; nesting one Render's
		// output inside another clips the outer style at that reset.
		selected := i == m.cursor && !it.Expanded && !it.Upgraded

		cursorStyle, versionStyle, hintStyle := styleArrow, styleVersion, styleDim
		switch {
		case it.Upgraded:
			cursorStyle, versionStyle = styleDim, styleDim
		case selected:
			cursorStyle = cursorStyle.Bold(true)
			versionStyle = versionStyle.Bold(true)
			hintStyle = hintStyle.Bold(true)
		}

		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}

		sourceHint := hintStyle.Render("[no changelog source]")
		if it.Owner != "" {
			sourceHint = hintStyle.Render(fmt.Sprintf("[%s/%s]", it.Owner, it.Repo))
		}

		var statusTag string
		switch {
		case it.Upgraded:
			statusTag = "  " + styleDim.Render("[upgraded]")
		case it.Upgrading:
			statusTag = "  " + m.spinner.View() + " upgrading..."
		case it.UpgradeErr != "":
			statusTag = "  " + styleError.Render("[upgrade failed]")
		}

		nameKind := fmt.Sprintf("%s (%s)", it.Name, it.Kind)
		switch {
		case it.Upgraded:
			nameKind = styleDim.Render(nameKind)
		case selected:
			nameKind = styleSelected.Render(nameKind)
		}

		line := fmt.Sprintf("%s%s  %s -> %s  %s%s",
			cursor, nameKind,
			versionStyle.Render(it.Installed), versionStyle.Render(it.Current),
			sourceHint, statusTag)
		writeLine(line)

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
			writeLine(styleBodyFrame.Render(content))
		}
	}

	return b.String(), itemLines
}
