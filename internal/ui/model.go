package ui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.BorderStyle(b)
	}()

	// Log Level Styles
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Bold(true)
	infoStyleLog = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	debugStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Bold(true)

	// JSON Styles
	jsonKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd"))
)

type InputMode int

const (
	ModeNormal InputMode = iota
	ModeFilter
	ModeSetStartDate
	ModeSetEndDate
)

type Model struct {
	viewport        viewport.Model
	textInput       textinput.Model
	originalContent string
	content         string
	filename        string
	ready           bool
	headerHeight    int
	footerHeight    int
	inputMode       InputMode

	// Advanced Filters
	showError bool
	showWarn  bool
	showInfo  bool
	showDebug bool
	regexMode bool

	// Text Filter Storage
	filterText string

	// Date Filters
	startDate *time.Time
	endDate   *time.Time
}

func InitialModel(filename, content string) Model {
	ti := textinput.New()
	ti.Placeholder = "Filter logs..."
	ti.CharLimit = 156
	ti.Width = 20

	// Apply highlighting initially
	highlighted := highlightLog(content)

	return Model{
		filename:        filename,
		originalContent: highlighted,
		content:         highlighted,
		headerHeight:    3,
		footerHeight:    3,
		textInput:       ti,
		inputMode:       ModeNormal,
		showError:       true,
		showWarn:        true,
		showInfo:        true,
		showDebug:       true,
		regexMode:       false,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Handle resize independently
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		verticalMarginHeight := m.headerHeight + m.footerHeight
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = m.headerHeight
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
	}

	// Handle text input if in any input mode
	if m.inputMode != ModeNormal {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				val := m.textInput.Value()

				if m.inputMode == ModeFilter {
					m.filterText = val
				} else if m.inputMode == ModeSetStartDate {
					if val == "" {
						m.startDate = nil
					} else {
						t, err := parseDate(val)
						if err == nil {
							m.startDate = &t
						}
					}
				} else if m.inputMode == ModeSetEndDate {
					if val == "" {
						m.endDate = nil
					} else {
						t, err := parseDate(val)
						if err == nil {
							m.endDate = &t
						}
					}
				}

				m.inputMode = ModeNormal
				m.applyFilters()
				m.textInput.Blur()
				return m, nil
			case "esc":
				m.inputMode = ModeNormal
				m.textInput.Blur()
				// Re-apply filters to restore content if we were halfway typing
				m.applyFilters()
				return m, nil
			}
		}
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.inputMode = ModeFilter
			m.textInput.Placeholder = "Filter logs..."
			m.textInput.SetValue(m.filterText)
			m.textInput.Focus()
			return m, textinput.Blink
		case "[":
			m.inputMode = ModeSetStartDate
			m.textInput.Placeholder = "YYYY-MM-DD HH:MM:SS"
			m.textInput.SetValue("") // Always clear for new date input? Or show existing?
			// Show existing if set
			if m.startDate != nil {
				m.textInput.SetValue(m.startDate.Format("2006-01-02 15:04:05"))
			}
			m.textInput.Focus()
			return m, textinput.Blink
		case "]":
			m.inputMode = ModeSetEndDate
			m.textInput.Placeholder = "YYYY-MM-DD HH:MM:SS"
			if m.endDate != nil {
				m.textInput.SetValue(m.endDate.Format("2006-01-02 15:04:05"))
			} else {
				m.textInput.SetValue("")
			}
			m.textInput.Focus()
			return m, textinput.Blink
		case "esc":
			// clear all filters
			m.filterText = ""
			m.startDate = nil
			m.endDate = nil
			m.applyFilters()

		// Advanced Toggles
		case "1":
			m.showError = !m.showError
			m.applyFilters()
		case "2":
			m.showWarn = !m.showWarn
			m.applyFilters()
		case "3":
			m.showInfo = !m.showInfo
			m.applyFilters()
		case "4":
			m.showDebug = !m.showDebug
			m.applyFilters()
		case "ctrl+r":
			m.regexMode = !m.regexMode
			m.applyFilters() // Re-apply to update regex usage
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) applyFilters() {
	var filtered []string
	lines := strings.Split(m.originalContent, "\n")

	// Pre-compile regex if in regex mode
	var regex *regexp.Regexp
	var err error
	if m.regexMode && m.filterText != "" {
		regex, err = regexp.Compile(m.filterText)
		if err != nil {
			// Invalid regex
		}
	}

	for _, line := range lines {
		// 1. Level Filtering
		if strings.Contains(line, "ERROR") && !m.showError {
			continue
		}
		if strings.Contains(line, "WARN") && !m.showWarn {
			continue
		}
		if strings.Contains(line, "INFO") && !m.showInfo {
			continue
		}
		if strings.Contains(line, "DEBUG") && !m.showDebug {
			continue
		}

		// 2. Date Filtering
		if m.startDate != nil || m.endDate != nil {
			t, ok := extractDate(line)
			if ok {
				if m.startDate != nil && t.Before(*m.startDate) {
					continue
				}
				if m.endDate != nil && t.After(*m.endDate) {
					continue
				}
			}
			// If no date found in line, do we keep or drop?
			// Usually keep if we are not strict, or drop if we are strict.
			// Let's default to DROP if we have active date filters and can't find a date?
			// Or KEEP to be safe?
			// Let's KEEP lines without dates (like stack traces) usually attached to previous lines.
			// But since we are processing line-by-line, we don't know context.
			// Simple approach: if date found, filter. If not found, INCLUDE (assume it's part of context).
		}

		// 3. Text/Regex Filtering
		if m.filterText != "" {
			if m.regexMode && regex != nil {
				if !regex.MatchString(line) {
					continue
				}
			} else if !m.regexMode {
				if !strings.Contains(strings.ToLower(line), strings.ToLower(m.filterText)) {
					continue
				}
			}
		}

		filtered = append(filtered, line)
	}

	m.content = strings.Join(filtered, "\n")
	m.viewport.SetContent(m.content)
	m.viewport.YOffset = 0
}

func parseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown format")
}

func extractDate(line string) (time.Time, bool) {
	// Simple heuristic: look for the first occurrence of something looking like a date
	// 2023-01-01 or 2023-01-01T...
	// Regex for YYYY-MM-DD
	// We matched the YYYY-MM-DD part, but we want to capture time if present too.
	// But `parseDate` handles the formats. We just need to find the substring that LOOKS like a date start.
	re := regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]?(\d{2}:\d{2}:\d{2})?`)
	loc := re.FindStringIndex(line)
	if loc != nil {
		s := line[loc[0]:loc[1]]
		t, err := parseDate(s)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func highlightLog(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// JSON Pretty Print
		if strings.HasPrefix(strings.TrimSpace(line), "{") && strings.HasSuffix(strings.TrimSpace(line), "}") {
			var js map[string]interface{}
			if json.Unmarshal([]byte(line), &js) == nil {
				line = colorizeJSON(line)
			}
		}

		if strings.Contains(line, "ERROR") {
			lines[i] = strings.Replace(line, "ERROR", errorStyle.Render("ERROR"), 1)
		} else if strings.Contains(line, "WARN") {
			lines[i] = strings.Replace(line, "WARN", warnStyle.Render("WARN"), 1)
		} else if strings.Contains(line, "INFO") {
			lines[i] = strings.Replace(line, "INFO", infoStyleLog.Render("INFO"), 1)
		} else if strings.Contains(line, "DEBUG") {
			lines[i] = strings.Replace(line, "DEBUG", debugStyle.Render("DEBUG"), 1)
		} else {
			lines[i] = line
		}
	}
	return strings.Join(lines, "\n")
}

func colorizeJSON(s string) string {
	re := regexp.MustCompile(`"([^"]+)":`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		return jsonKeyStyle.Render(match)
	})
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

func (m Model) headerView() string {
	title := titleStyle.Render(m.filename)
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m Model) footerView() string {
	if m.inputMode != ModeNormal {
		// Show what we are inputting
		prefix := ""
		switch m.inputMode {
		case ModeFilter:
			prefix = "/"
		case ModeSetStartDate:
			prefix = "[Start]: "
		case ModeSetEndDate:
			prefix = "[End]: "
		}
		return prefix + m.textInput.View()
	}
	
	// Show active date filters in footer if present
	status := fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)
	if m.startDate != nil {
		status += fmt.Sprintf(" [Start:%s]", m.startDate.Format("01-02 15:04"))
	}
	if m.endDate != nil {
		status += fmt.Sprintf(" [End:%s]", m.endDate.Format("01-02 15:04"))
	}
	
	info := infoStyle.Render(status)
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
