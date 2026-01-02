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
	"github.com/atotto/clipboard"
    "github.com/fsnotify/fsnotify"
    "os"
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

	// Selection Style
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#555555")).Foreground(lipgloss.Color("#ffffff"))
)

type Point struct {
	X int
	Y int
}


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
    xOffset         int
    screenWidth     int
    wrap            bool
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

	// Selection
	rawContent     string // Content without ANSI codes for copying
	selecting      bool
	selectionStart *Point
	selectionEnd   *Point


	// Text Filter Storage
	filterText string

	// Date Filters
	startDate *time.Time
	endDate   *time.Time

    // Live Tailing
    following bool
    fileSize  int64
    watcher   *fsnotify.Watcher

    // Folding
    foldStackTraces bool
}

func InitialModel(filename, content string) Model {
	ti := textinput.New()
	ti.Placeholder = "Filter logs..."
	ti.CharLimit = 156
	ti.Width = 20

	// Highlighting will be applied lazily in View()
	// highlighted := highlightLog(content)

    // Get initial file size for watcher
    var fileSize int64
    f, err := os.Stat(filename)
    if err == nil {
        fileSize = f.Size()
    }
    
    // Initialize Watcher
    watcher, _ := fsnotify.NewWatcher()
    if watcher != nil {
        watcher.Add(filename)
    }

	return Model{
		filename:        filename,
		rawContent:      content,
		originalContent: content, // No longer highlighted initially
		content:         content, // No longer highlighted initially
		headerHeight:    3,
		footerHeight:    3,
		textInput:       ti,
		inputMode:       ModeNormal,
		showError:       true,
		showWarn:        true,
		showInfo:        true,
		showDebug:       true,
		regexMode:       false,

		selectionStart:  nil,
		selectionEnd:    nil,
        xOffset:         0,
        screenWidth:     0,
        wrap:            false,
        following:       false, // Start with follow mode off by default? Or detection?
        fileSize:        fileSize,
        watcher:         watcher,
        foldStackTraces: false,
	}
}


func (m Model) Init() tea.Cmd {
    // Start Input Blink AND File Watcher
    cmds := []tea.Cmd{textinput.Blink}
    if m.watcher != nil {
        cmds = append(cmds, WaitForFileChange(m.watcher, m.filename, m.fileSize))
    }
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
    
    // Handle File Changes
    if msg, ok := msg.(FileChangeMsg); ok {
        if msg.Error != nil {
            // Handle error?
        } else if msg.NewContent != "" {
            // Append new content
            m.originalContent += msg.NewContent
            // Re-apply filters to update m.content
            // Note: This might be expensive for huge files. 
            // Optimization: append to m.content if no filters active?
            m.applyFilters()
            
            m.fileSize = msg.NewOffset
            
            // Auto-scroll if following
            if m.following {
                m.viewport.GotoBottom()
            }
        }
        // Continue watching
        if m.watcher != nil {
             cmds = append(cmds, WaitForFileChange(m.watcher, m.filename, m.fileSize))
        }
    }

	// Handle resize independently
	// Handle resize independently
	// Handle resize independently
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		verticalMarginHeight := m.headerHeight + m.footerHeight
        m.screenWidth = msg.Width

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = m.headerHeight
            m.viewport.Width = 20000 // Virtual width to avoid clipping
            // Only set content AFTER setting width to avoid initial wrapping? 
            // Actually New() sets width. We overwrite it.
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = 20000 // Keep it wide
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
        
        // Return early to avoid m.viewport.Update(msg) resetting Width to msg.Width
        return m, nil 
	}

	// Handle Mouse Events for Selection
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Y >= m.headerHeight && msg.Y < m.viewport.Height+m.headerHeight {
			lineIndex := msg.Y - m.headerHeight + m.viewport.YOffset
            logicalX := msg.X + m.xOffset
			totalLines := strings.Count(m.content, "\n") + 1
			if lineIndex >= 0 && lineIndex < totalLines {
				if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
					m.selecting = true
					m.selectionStart = &Point{X: logicalX, Y: lineIndex}
					m.selectionEnd = &Point{X: logicalX, Y: lineIndex}
					// View update happens automatically on re-render
				} else if msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft && m.selecting {
					m.selectionEnd = &Point{X: logicalX, Y: lineIndex}
					// View update happens automatically on re-render
				} else if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
					m.selecting = false
				}
			}
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
		case "y":

			if m.selectionStart != nil && m.selectionEnd != nil {
				start, end := *m.selectionStart, *m.selectionEnd
				if start.Y > end.Y || (start.Y == end.Y && start.X > end.X) {
					start, end = end, start
				}

				lines := strings.Split(m.content, "\n")
				var selectedLines []string

				for i := start.Y; i <= end.Y && i < len(lines); i++ {
					line := stripAnsi(lines[i]) // Strip ANSI first
					runes := []rune(line)
					
					startCol := 0
					if i == start.Y {
						startCol = start.X
					}
					
					endCol := len(runes)
					if i == end.Y {
						endCol = end.X + 1 // Inclusive
					}
					
					// Clamp
					if startCol < 0 { startCol = 0 }
					if startCol > len(runes) { startCol = len(runes) }
					if endCol < 0 { endCol = 0 }
					if endCol > len(runes) { endCol = len(runes) }
					
					if startCol < endCol {
						selectedLines = append(selectedLines, string(runes[startCol:endCol]))
					} else {
                        // Empty line or invalid range
                        selectedLines = append(selectedLines, "")
                    }
				}

				text := strings.Join(selectedLines, "\n")
				clipboard.WriteAll(text)

				m.selectionStart = nil
				m.selectionEnd = nil
			}
			return m, nil


		case "esc":
			if m.selectionStart != nil {
				m.selectionStart = nil
				m.selectionEnd = nil
				return m, nil
			}
			// clear all filters
			m.filterText = ""
			m.startDate = nil
			m.endDate = nil
			m.applyFilters()


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
        
        // Horizontal Scrolling
        case "right", "l":
            m.xOffset += 5
        case "left", "h":
            m.xOffset -= 5
            if m.xOffset < 0 {
                m.xOffset = 0
            }
        
        // Toggle Word Wrap
        case "w":
            m.wrap = !m.wrap
            
        // Toggle Follow Mode
        case "f":
            m.following = !m.following
            if m.following {
                m.viewport.GotoBottom()
            }
        
        // Toggle Stack Trace Folding
        case "z":
            m.foldStackTraces = !m.foldStackTraces
            m.applyFilters()
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
    
    // Stack Trace Folding Logic
    // If not folding, we just proceed.
    // If folding, we process the 'filtered' list again (or ideally during initial pass, but separation is cleaner for MVP).
    if m.foldStackTraces {
        var folded []string
        var traceBuffer []string
        
        flushTrace := func() {
            if len(traceBuffer) > 0 {
                // Heuristic: If just 1 line, don't fold.
                if len(traceBuffer) == 1 {
                    folded = append(folded, traceBuffer...)
                } else {
                    // Fold!
                    summary := fmt.Sprintf("  [+] %d lines folded (stack trace/indented block)...", len(traceBuffer))
                    // Style it?
                    summary = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render(summary)
                    folded = append(folded, summary)
                }
                traceBuffer = nil
            }
        }
        
        for _, line := range filtered {
            // Check for indentation (heuristic for stack trace)
            // TAB or at least 2 spaces
            isIndented := strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ")
            
            if isIndented {
                traceBuffer = append(traceBuffer, line)
            } else {
                flushTrace()
                folded = append(folded, line)
            }
        }
        flushTrace()
        m.content = strings.Join(folded, "\n")
    } else {
        m.content = strings.Join(filtered, "\n")
    }

    // Clear selection on filter change
    m.selectionStart = nil
    m.selectionEnd = nil
	m.viewport.SetContent(m.content)
	m.viewport.YOffset = 0
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
    
    // Lazy Rendering Logic
    // 1. Get visible raw text from viewport
    visibleText := m.viewport.View()
    visibleLines := strings.Split(visibleText, "\n")
    
    // 2. Iterate and apply highlighting/selection
    var renderedLines []string
    for i, line := range visibleLines {
        // Calculate real line index in the full content
        realLineIndex := m.viewport.YOffset + i
        
        // 1. Syntax Highlighting (Lazy)
        // We use a helper that processes just one line
        line = highlightLine(line)
        
        // 2. Wrap vs Horizontal Scroll
        if m.wrap {
             // WRAP MODE
             // We render the FULL highlighted line, wrapped to screen width.
             // Selection could be applied here?
             
             // Complex Selection in Unwrap is hard because logical X jumps.
             // For MVP, applying selection logic logically to the unwrapped string works best.
             // lipgloss/wordwrap usually preserves background color?
             
             // 1. Apply Selection to full line if applicable
             if m.selectionStart != nil && m.selectionEnd != nil {
                 start, end := *m.selectionStart, *m.selectionEnd
                 if start.Y > end.Y || (start.Y == end.Y && start.X > end.X) {
                     start, end = end, start
                 }
                  if realLineIndex >= start.Y && realLineIndex <= end.Y {
                      cleanLine := stripAnsi(line)
                      runes := []rune(cleanLine)
                      
                      startCol := 0
                      if realLineIndex == start.Y {
                          startCol = start.X
                      }
                      
                      endCol := len(runes)
                      if realLineIndex == end.Y {
                          endCol = end.X + 1
                      }
                      
                      if startCol < 0 { startCol = 0 }
                      if startCol > len(runes) { startCol = len(runes) }
                      if endCol < 0 { endCol = 0 }
                      if endCol > len(runes) { endCol = len(runes) }
                      
                      if startCol < endCol {
                          pre := string(runes[:startCol])
                          sel := selectedStyle.Render(string(runes[startCol:endCol]))
                          post := string(runes[endCol:])
                          line = pre + sel + post
                      }
                  }
             }

             // 2. Wrap the fully styled string
             // Note: using m.screenWidth - constant for margin/padding if any.
             // Viewport width is usually correct for inner content if set right.
             // We use m.screenWidth as a fallback or m.viewport.Width if checking raw terminal size. 
             // But m.viewport.Width is 20000. So we must use m.screenWidth!
             
             width := m.screenWidth
             if width <= 0 { width = 80 } // safety
             
             // lipgloss's Style.Width() with Render() handles wrapping and ANSI codes.
             wrapped := lipgloss.NewStyle().Width(width).Render(line)
             renderedLines = append(renderedLines, wrapped)

        } else {
                // NO WRAP / HORIZONTAL SCROLL MODE
        
                // Convert to runes for safe slicing
                rawLine := visibleLines[i]
                rawRunes := []rune(rawLine)

                if m.xOffset < len(rawRunes) {
                     end := m.xOffset + m.screenWidth
                     if end > len(rawRunes) {
                         end = len(rawRunes)
                     }
                     // Store the visible slice
                     visiblePart := string(rawRunes[m.xOffset : end])

                     // Highlight visible part
                     line = highlightLine(visiblePart)

                     // 2. Selection Highlighting (Lazy)
                     if m.selectionStart != nil && m.selectionEnd != nil {
                         start, end := *m.selectionStart, *m.selectionEnd
                         if start.Y > end.Y || (start.Y == end.Y && start.X > end.X) {
                             start, end = end, start
                         }

                         // Check intersection
                         if realLineIndex >= start.Y && realLineIndex <= end.Y {
                             
                             // Adjust X to visual
                              visStart := 0
                              if realLineIndex == start.Y {
                                  visStart = start.X - m.xOffset
                              }
                              visEnd := len([]rune(visiblePart)) // default end of line
                              if realLineIndex == end.Y {
                                  visEnd = end.X + 1 - m.xOffset
                              }
                              
                              // Clamp to 0..len
                              if visStart < 0 { visStart = 0 }
                              if visStart > len([]rune(visiblePart)) { visStart = len([]rune(visiblePart)) }
                              if visEnd < 0 { visEnd = 0 }
                              if visEnd > len([]rune(visiblePart)) { visEnd = len([]rune(visiblePart)) }
                              
                              if visStart < visEnd {
                                  vpRunes := []rune(stripAnsi(line)) // Remove syntax colors to apply selection cleanly
                                  pre := string(vpRunes[:visStart])
                                  sel := selectedStyle.Render(string(vpRunes[visStart:visEnd]))
                                  post := string(vpRunes[visEnd:])
                                  line = pre + sel + post
                              }
                         }
                     }
                } else {
                     line = "" // Scrolled past end
                }
        
                renderedLines = append(renderedLines, line)
        }

    }
    
    // Join rendered lines
    finalContent := strings.Join(renderedLines, "\n")

	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), finalContent, m.footerView())
}


// Replaces highlightLog (single line version)
func highlightLine(line string) string {
    // JSON Pretty Print Check
    if strings.HasPrefix(strings.TrimSpace(line), "{") && strings.HasSuffix(strings.TrimSpace(line), "}") {
        var js map[string]interface{}
        if json.Unmarshal([]byte(line), &js) == nil {
            return colorizeJSON(line)
        }
    }

    if strings.Contains(line, "ERROR") {
        return strings.Replace(line, "ERROR", errorStyle.Render("ERROR"), 1)
    } else if strings.Contains(line, "WARN") {
        return strings.Replace(line, "WARN", warnStyle.Render("WARN"), 1)
    } else if strings.Contains(line, "INFO") {
        return strings.Replace(line, "INFO", infoStyleLog.Render("INFO"), 1)
    } else if strings.Contains(line, "DEBUG") {
        return strings.Replace(line, "DEBUG", debugStyle.Render("DEBUG"), 1)
    }
    return line
}



func stripAnsi(str string) string {
	re := regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\x07)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
	return re.ReplaceAllString(str, "")
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



func colorizeJSON(s string) string {
	re := regexp.MustCompile(`"([^"]+)":`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		return jsonKeyStyle.Render(match)
	})
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
    
    if m.following {
        status += infoStyle.Render(" [FOLLOWING]")
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
