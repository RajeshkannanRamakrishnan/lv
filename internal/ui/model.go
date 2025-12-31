package ui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Bold(true)
	infoStyleLog = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	debugStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Bold(true)

    // JSON Styles
    jsonKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd"))
    jsonValStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c"))
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
	filtering       bool
    
    // Advanced Filters
    showError       bool
    showWarn        bool
    showInfo        bool
    showDebug       bool
    regexMode       bool
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

	if m.filtering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.filtering = false
				m.applyFilters()
				m.textInput.Blur()
				return m, nil
			case "esc":
				m.filtering = false
				m.textInput.Blur()
                // Do not reset text input here, just cancel focus
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
			m.filtering = true
			m.textInput.Focus()
			return m, textinput.Blink
		case "esc":
			// clear filter text
			m.textInput.Reset()
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
            if m.regexMode {
                m.textInput.Placeholder = "Regex Filter..."
            } else {
                m.textInput.Placeholder = "Filter logs..."
            }
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) applyFilters() {
    query := m.textInput.Value()
    
    var filtered []string
    lines := strings.Split(m.originalContent, "\n")
    
    // Pre-compile regex if in regex mode
    var regex *regexp.Regexp
    var err error
    if m.regexMode && query != "" {
        regex, err = regexp.Compile(query)
        if err != nil {
            // Invalid regex, treat as match failure for now
            // In a better UI we would show error
        }
    }
    
    for _, line := range lines {
        // 1. Level Filtering
        if strings.Contains(line, "ERROR") && !m.showError { continue }
        if strings.Contains(line, "WARN") && !m.showWarn { continue }
        if strings.Contains(line, "INFO") && !m.showInfo { continue }
        if strings.Contains(line, "DEBUG") && !m.showDebug { continue }
        
        // 2. Search/Regex Filtering
        if query != "" {
            if m.regexMode && regex != nil {
                 if !regex.MatchString(line) { continue }
            } else if !m.regexMode {
                 if !strings.Contains(strings.ToLower(line), strings.ToLower(query)) { continue }
            } else {
                // Regex invalid, maybe just skip or show? skipping
                continue
            }
        }
        
        filtered = append(filtered, line)
    }
    
    m.content = strings.Join(filtered, "\n")
	m.viewport.SetContent(m.content)
    m.viewport.YOffset = 0
}

func highlightLog(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
        // JSON Pretty Print
        if strings.HasPrefix(strings.TrimSpace(line), "{") && strings.HasSuffix(strings.TrimSpace(line), "}") {
            var js map[string]interface{}
            if json.Unmarshal([]byte(line), &js) == nil {
                // Valid JSON, let's pretty print it or just colorize keys
                // For simplicity in TUI line-based, let's just colorize keys in the single line
                // Re-serializing might disrupt standard log format if it was compact. 
                // Let's iterate keys and colorize them.
                // A full syntax highlighter is complex, implementing a basic heuristic here.
                
                // Helper to colorize keys in string
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
             // If JSON was processed, update line
             lines[i] = line
        }
	}
	return strings.Join(lines, "\n")
}

func colorizeJSON(s string) string {
    // Basic regex to find keys: "key":
    re := regexp.MustCompile(`"([^"]+)":`)
    return re.ReplaceAllStringFunc(s, func(match string) string {
        // match is "key":
        // simple replace
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
	if m.filtering {
		return m.textInput.View()
	}
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
