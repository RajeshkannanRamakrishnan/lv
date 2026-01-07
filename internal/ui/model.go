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
    "sort"
    "math"
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

	// Match Style (Search Matches)
	matchStyle = lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00")).Foreground(lipgloss.Color("#000000"))
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
    ModeJumpTime
)

type Model struct {
	viewport        viewport.Model
	textInput       textinput.Model
	originalLines   []string

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

	// Virtualization
	filteredLines   []string // Replaces content/originalLines for display (this is the SOURCE of truth for viewport)
	yOffset         int
	viewportHeight  int
	
	// Live Tailing
	following bool
	fileSize  int64
	watcher   *fsnotify.Watcher

	// Folding
	foldStackTraces bool

	// Timeline
	showTimeline     bool
	timelineViewport viewport.Model

	// Bookmarks
	bookmarks map[int]struct{}
}

func InitialModel(filename string, lines []string) Model {
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
		originalLines:   lines,
		filteredLines:   lines, // Initially all lines
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
		yOffset:         0,
		screenWidth:     0,
		wrap:            false,
		following:       false, 
		fileSize:        fileSize,
		watcher:         watcher,
		foldStackTraces: false,
		showTimeline:    false,
		bookmarks:       make(map[int]struct{}),
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
            newLines := strings.Split(msg.NewContent, "\n")
            // Handle edge case where last line was incomplete? 
            // For simplicity, just append. Ideally we handle partial lines.
            
			m.originalLines = append(m.originalLines, newLines...)
			// Re-apply filters to update m.content
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
			m.viewport.SetContent("") // Virtualized: View() handles content
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
			lineIndex := msg.Y - m.headerHeight + m.yOffset
			
			// Adjust for gutter offset in No-Wrap mode (default view)
			// We add 3 spaces of padding in View() for no-wrap mode: line = "   " + line
			// So we need to subtract 3 from visual X to get logical X.
			gutterOffset := 0
			if !m.wrap {
				gutterOffset = 3
			}
			
            logicalX := msg.X + m.xOffset - gutterOffset
            if logicalX < 0 {
                logicalX = 0
            }

			totalLines := len(m.filteredLines)
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
						t, err := parseDate(val)
						if err == nil {
							m.endDate = &t
						}
					}
                } else if m.inputMode == ModeJumpTime {
                    // Jump to Time Logic
                    if val != "" {
                        target, err := parseDate(val)
                        // Heuristic: If parsing fails or assumes year 0, try combining with first log line date
                        if err != nil || target.Year() == 0 {
                             // Try to interpret as HH:MM or HH:MM:SS relative to first log line
                             // Get base date
                             if len(m.filteredLines) > 0 {
                                 // Simple: split first line
                                 firstLine := m.filteredLines[0]
                                 if base, ok := extractDate(firstLine); ok {
                                     // Try to parse val as HH:MM:SS
                                     // We can use a custom parser or try strict formats
                                     // Simple approach: Replace timestamp in base with val?
                                     // Or parse val as time.Time (0000-01-01 HH:MM partial) and join.
                                     
                                     // Let's rely on time.Parse for just time formats
                                     timeFormats := []string{"15:04", "15:04:05", "3:04PM"}
                                     var timeComponent time.Time
                                     parsedTime := false
                                     for _, tf := range timeFormats {
                                         if tc, err := time.Parse(tf, val); err == nil {
                                             timeComponent = tc
                                             parsedTime = true
                                             break
                                         }
                                     }
                                     
                                     if parsedTime {
                                         // Combine base YYYY-MM-DD with timeComponent HH:MM:SS
                                         year, month, day := base.Date()
                                         hour, min, sec := timeComponent.Clock()
                                         target = time.Date(year, month, day, hour, min, sec, 0, base.Location())
                                         err = nil // Success
                                     }
                                 }
                             }
                        }
                        
                        if err == nil {
                            // Search for first line >= target
                            lines := m.filteredLines
                            for i, line := range lines {
                                if t, ok := extractDate(line); ok {
                                    if !t.Before(target) {
                                        m.viewport.YOffset = i
                                        break
                                    }
                                }
                            }
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

				lines := m.filteredLines
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
            
        // Toggle Timeline
        case "t":
            m.showTimeline = !m.showTimeline
            if m.showTimeline {
                m.generateTimeline()
                // Initialize timeline viewport if not ready
                if m.timelineViewport.Height == 0 {
                    m.timelineViewport = viewport.New(m.screenWidth, m.viewport.Height)
                    m.timelineViewport.YPosition = m.headerHeight
                }
                m.timelineViewport.Width = m.screenWidth
                m.timelineViewport.Height = m.viewport.Height // Overlay same size
            }
        
        // Time Travel
        case "J":
            m.inputMode = ModeJumpTime
            m.textInput.Placeholder = "14:30 or YYYY-MM-DD..."
            m.textInput.SetValue("")
            m.textInput.Focus()
            return m, textinput.Blink
        
        // Bookmarks
        case "m":
             // Toggle bookmark at current YOffset (top visible line)
             // Or selection? Usually "mark current line".
             // We use m.viewport.YOffset.
             row := m.viewport.YOffset
             if _, exists := m.bookmarks[row]; exists {
                 delete(m.bookmarks, row)
             } else {
                 m.bookmarks[row] = struct{}{}
             }
             
        case "n":
             // Jump to next bookmark > current YOffset
             start := m.viewport.YOffset + 1
             next := -1
             minDist := int(^uint(0) >> 1)
             
             for row := range m.bookmarks {
                 if row >= start {
                     dist := row - start
                     if dist < minDist {
                         minDist = dist
                         next = row
                     }
                 }
             }
             
             if next != -1 {
                 m.viewport.YOffset = next
             }
             
        case "N":
             // Jump to prev bookmark < current YOffset
             start := m.viewport.YOffset - 1
             prev := -1
             minDist := int(^uint(0) >> 1)
             
             for row := range m.bookmarks {
                 if row <= start {
                     dist := start - row
                     if dist < minDist {
                         minDist = dist
                         prev = row
                     }
                 }
             }
             
             if prev != -1 {
                 m.viewport.YOffset = prev
             }
		}

	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
        // Virtualized Scrolling
        case "up", "k":
            m.yOffset--
        case "down", "j":
            m.yOffset++
        case "pgup", "ctrl+b":
            m.yOffset -= m.viewport.Height
        case "pgdown", "ctrl+f", "space":
            m.yOffset += m.viewport.Height
        case "home", "g":
            m.yOffset = 0
        case "end", "G":
            m.yOffset = len(m.filteredLines) - m.viewport.Height
        }
    case tea.MouseMsg:
        switch msg.Type {
        case tea.MouseWheelUp:
            m.yOffset--
        case tea.MouseWheelDown:
            m.yOffset++
        }
    }
    
    // Clamp yOffset
    if m.yOffset < 0 {
        m.yOffset = 0
    }
    maxOffset := len(m.filteredLines) - m.viewport.Height
    if maxOffset < 0 {
        maxOffset = 0
    }
    if m.yOffset > maxOffset {
        m.yOffset = maxOffset
    }
    
    // Disable follow mode if user scrolls up manually
    // (Simple heuristic: if not at bottom)
    if m.yOffset < maxOffset {
        m.following = false
    }

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
    
    // Timeline viewport update
    if m.showTimeline {
        m.timelineViewport, cmd = m.timelineViewport.Update(msg)
        cmds = append(cmds, cmd)
    }

	return m, tea.Batch(cmds...)
}

func (m *Model) applyFilters() {
	var filtered []string
    // Directly iterate over originalLines
	lines := m.originalLines

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
        m.filteredLines = folded
    } else {
        m.filteredLines = filtered
    }

    // Clear selection on filter change
    m.selectionStart = nil
    m.selectionEnd = nil
    // Clear bookmarks on filter change? indices are invalid.
    m.bookmarks = make(map[int]struct{}) 
    
    // Virtualization reset
	m.yOffset = 0
    m.viewport.SetContent("") // Clear viewport content to force refresh? Actually View() constructs it.
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
    
    // Virtualization:
    // 1. Determine visible slice from m.filteredLines based on m.yOffset
    start := m.yOffset
    end := start + m.viewport.Height
    if start >= len(m.filteredLines) {
        start = len(m.filteredLines)
    }
    if end > len(m.filteredLines) {
        end = len(m.filteredLines)
    }
    
    visibleLines := m.filteredLines[start:end]
    
    // 2. Iterate and apply highlighting/selection to only these lines
    var renderedLines []string
    for i, line := range visibleLines {
        // Calculate real line index
        realLineIndex := start + i
        
        isBookmarked := false
        if _, ok := m.bookmarks[realLineIndex]; ok {
            isBookmarked = true
        }

        // 2. Wrap vs Horizontal Scroll
        if m.wrap {
             // WRAP MODE
			 // 0. Highlight Matches (Priority)
			 line = highlightMatches(line, m.filterText, m.regexMode)
             line = highlightLine(line)
             if isBookmarked {
                 line = "🔖 " + line
             }
             
             // 1. Apply Selection
             if m.selectionStart != nil && m.selectionEnd != nil {
                 startSel, endSel := *m.selectionStart, *m.selectionEnd
                 if startSel.Y > endSel.Y || (startSel.Y == endSel.Y && startSel.X > endSel.X) {
                     startSel, endSel = endSel, startSel
                 }
                  if realLineIndex >= startSel.Y && realLineIndex <= endSel.Y {
                      cleanLine := stripAnsi(line)
                      
                      runes := []rune(cleanLine)
                      
                      startCol := 0
                      if realLineIndex == startSel.Y {
                          startCol = startSel.X
                      }
                      
                      endCol := len(runes)
                      if realLineIndex == endSel.Y {
                          endCol = endSel.X + 1
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
             

             
             width := m.screenWidth
             if width <= 0 { width = 80 }
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
					 visiblePart = highlightMatches(visiblePart, m.filterText, m.regexMode)
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
                                  vpRunes := []rune(stripAnsi(line)) 
                                  pre := string(vpRunes[:visStart])
                                  sel := selectedStyle.Render(string(vpRunes[visStart:visEnd]))
                                  post := string(vpRunes[visEnd:])
                                  line = pre + sel + post
                              }
                         }
                     }
                     
                     // 3. Apply Bookmark (Visual Only, after highlighting/selection)
                     if isBookmarked {
                         line = "🔖 " + line
                     } else {
                         line = "   " + line // Maintain alignment
                     }
                     
                } else {
                     line = "" // Scrolled past end
                }
        
                renderedLines = append(renderedLines, line)
        }

    }
    
	// Join rendered lines
    finalContent := strings.Join(renderedLines, "\n")
    
    // IMPORTANT: Feed the rendered (and potentially wrapped) content to the viewport
    // This handling clipping (ensure we don't exceed height) and padding if strictly needed.
    m.viewport.SetContent(finalContent)
    m.viewport.YOffset = 0
    
    currentView := m.viewport.View()
    if m.showTimeline {
        currentView = m.timelineViewport.View()
    }

	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), currentView, m.footerView())
}

func (m *Model) generateTimeline() {
    // 1. Extract timestamps
    var timestamps []time.Time
    lines := m.filteredLines
    for _, line := range lines {
        if t, ok := extractDate(line); ok {
            timestamps = append(timestamps, t)
        }
    }
    
    if len(timestamps) == 0 {
        m.timelineViewport.SetContent("\n  No timestamps found in current view.")
        return
    }
    
    sort.Slice(timestamps, func(i, j int) bool {
        return timestamps[i].Before(timestamps[j])
    })
    
    minTime := timestamps[0]
    maxTime := timestamps[len(timestamps)-1]
    
    duration := maxTime.Sub(minTime)
    
    // Determine interval
    var interval time.Duration
    var format string
    
    if duration < time.Hour {
        interval = time.Minute
        format = "15:04"
    } else if duration < 24*time.Hour {
        interval = 15 * time.Minute // 15 mins
        format = "15:04"
    } else {
        interval = time.Hour
        format = "02 Jan 15:04"
    }
    
    // Create buckets
    // Map bucket start time -> count
    buckets := make(map[int64]int)
    var maxCount int
    
    for _, t := range timestamps {
        bucket := t.Truncate(interval).Unix()
        buckets[bucket]++
        if buckets[bucket] > maxCount {
            maxCount = buckets[bucket]
        }
    }
    
    // Render Bars
    var out strings.Builder
    out.WriteString(fmt.Sprintf("\n  Log Volume Analysis (%s - %s)\n", minTime.Format(format), maxTime.Format(format)))
    out.WriteString(fmt.Sprintf("  Total Logs: %d | Interval: %s\n\n", len(timestamps), interval))
    
    // Iterate from start to end by interval
    // Limit to ~50-100 bars to prevent massive output
    // Actually, viewport handles unlimited height.
    
    startUnix := minTime.Truncate(interval).Unix()
    endUnix := maxTime.Truncate(interval).Unix()
    
    // Safe guard against infinite loop if interval is 0 (shouldn't happen)
    if interval == 0 { interval = time.Minute }
    
    barWidth := 50
    
    for t := startUnix; t <= endUnix; t += int64(interval.Seconds()) {
        count := buckets[t]
        
        // Normalize bar length
        barLen := 0
        if maxCount > 0 {
            barLen = int(math.Ceil(float64(count) / float64(maxCount) * float64(barWidth)))
        }
        
        bar := strings.Repeat("█", barLen) 
        // Pad with spaces
        // bar += strings.Repeat(" ", barWidth - barLen)
        
        timeLabel := time.Unix(t, 0).Format(format)
        out.WriteString(fmt.Sprintf("  %s │ %s (%d)\n", timeLabel, bar, count))
    }
    
    m.timelineViewport.SetContent(out.String())
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

func highlightMatches(line, pattern string, isRegex bool) string {
	if pattern == "" {
		return line
	}

	var re *regexp.Regexp
	var err error

	if isRegex {
		re, err = regexp.Compile(pattern)
	} else {
		// Case insensitive literal match
		re, err = regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
	}

	if err != nil {
		return line // Return regex errors as is (or handle better?)
	}

	return re.ReplaceAllStringFunc(line, func(match string) string {
		return matchStyle.Render(match)
	})
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
        case ModeJumpTime:
            prefix = "[Jump To]: "
		}
		return prefix + m.textInput.View()
	}
	
	// Show active date filters in footer if present
    // Calculate scroll percent manually
    var percent float64
    if len(m.filteredLines) > 0 {
        percent = float64(m.yOffset) / float64(len(m.filteredLines)-m.viewport.Height)
        if percent < 0 { percent = 0 }
        if percent > 1 { percent = 1 }
    }
	status := fmt.Sprintf("%3.f%%", percent*100)
    
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
