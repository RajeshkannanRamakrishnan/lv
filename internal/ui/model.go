package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-runewidth"
	"math"
	"os"
	"sort"
	"unicode/utf8"
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
	viewport      viewport.Model
	textInput     textinput.Model
	originalLines []string

	filename     string
	xOffset      int
	screenWidth  int
	wrap         bool
	ready        bool
	headerHeight int
	footerHeight int
	inputMode    InputMode

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
	regex      *regexp.Regexp

	// Date Filters
	startDate *time.Time
	endDate   *time.Time

	// Virtualization
	filteredLines  []string // Replaces content/originalLines for display (this is the SOURCE of truth for viewport)
	yOffset        int
	viewportHeight int

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

	// Help
	showHelp bool

	// Streamer
	streamer *Streamer

	// Cache
	layoutCache map[int][]string
}

func InitialModel(filename string, lines []string, reader io.Reader) Model {
	var streamer *Streamer
	if reader != nil {
		cfg := StreamerConfig{
			BatchLines: 200,
			FlushEvery: 50 * time.Millisecond,
		}
		// File startup backfill should favor throughput over ultra-low latency.
		if filename != "Stdin" {
			cfg.BatchLines = 5000
			cfg.FlushEvery = 100 * time.Millisecond
		}
		streamer = NewStreamerWithConfig(reader, cfg)
	}

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

	m := Model{
		filename:      filename,
		originalLines: lines,
		filteredLines: lines, // Initially all lines
		headerHeight:  3,
		footerHeight:  3,
		textInput:     ti,
		inputMode:     ModeNormal,
		showError:     true,
		showWarn:      true,
		showInfo:      true,
		showDebug:     true,
		regexMode:     false,

		selectionStart:  nil,
		selectionEnd:    nil,
		xOffset:         0,
		yOffset:         0,
		screenWidth:     0,
		wrap:            false,
		following:       streamer != nil && filename == "Stdin", // Auto-follow only for stdin streams
		fileSize:        fileSize,
		watcher:         watcher,
		foldStackTraces: false,
		showTimeline:    false,
		bookmarks:       make(map[int]struct{}),
		showHelp:        false,
		streamer:        streamer,
		layoutCache:     make(map[int][]string),
	}
	m.applyFilters(true)
	return m
}

func (m Model) Init() tea.Cmd {
	// Start Input Blink AND File Watcher
	cmds := []tea.Cmd{textinput.Blink}
	if m.watcher != nil {
		cmds = append(cmds, WaitForFileChange(m.watcher, m.filename, m.fileSize))
	}
	if m.streamer != nil {
		cmds = append(cmds, WaitForStream(m.streamer))
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
			newLines := splitIncomingContent(msg.NewContent)
			m.appendIncomingLines(newLines)

			m.fileSize = msg.NewOffset

			// Auto-scroll if following
			if m.following {
				// Virtualized goto bottom
				m.yOffset = len(m.filteredLines) - m.viewport.Height
				if m.yOffset < 0 {
					m.yOffset = 0
				}
			}
		}
		// Continue watching
		if m.watcher != nil {
			cmds = append(cmds, WaitForFileChange(m.watcher, m.filename, m.fileSize))
		}
	}

	// Handle Log Chunks (Streaming)
	if msg, ok := msg.(LogChunkMsg); ok {
		if msg.Err != nil {
			// EOF or error?
		} else if len(msg.Lines) > 0 {
			m.appendIncomingLines(msg.Lines)

			if m.following {
				m.yOffset = len(m.filteredLines) - m.viewport.Height
				if m.yOffset < 0 {
					m.yOffset = 0
				}
			}
		}
		// Continue stream loop
		if m.streamer != nil {
			cmds = append(cmds, WaitForStream(m.streamer))
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

		// Invalidate cache on resize
		m.layoutCache = make(map[int][]string)

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
					targetLine, targetX := m.resolvePos(msg.X, msg.Y-m.headerHeight)
					if targetLine >= 0 && targetLine < totalLines {
						m.selecting = true
						m.selectionStart = &Point{X: targetX, Y: targetLine}
						m.selectionEnd = &Point{X: targetX, Y: targetLine}
					}
				} else if msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft && m.selecting {
					targetLine, targetX := m.resolvePos(msg.X, msg.Y-m.headerHeight)
					if targetLine >= 0 && targetLine < totalLines {
						m.selectionEnd = &Point{X: targetX, Y: targetLine}
					}
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
					m.applyFilters(true)
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
				m.applyFilters(true)
				m.textInput.Blur()
				return m, nil
			case "esc":
				m.inputMode = ModeNormal
				m.textInput.Blur()
				// Re-apply filters to restore content if we were halfway typing
				m.applyFilters(true)
				return m, nil
			case "ctrl+y":
				m.copySelection()
				return m, nil
			}
		}
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showHelp {
			if msg.String() == "esc" || msg.String() == "?" || msg.String() == "q" {
				m.showHelp = false
			}
			return m, nil
		}

		switch msg.String() {
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		case "y":
			m.copySelection()
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
			m.applyFilters(true)

		case "/":
			m.inputMode = ModeFilter
			m.textInput.Placeholder = "Filter logs..."
			m.textInput.SetValue(m.filterText)
			m.textInput.SetCursor(len(m.filterText))
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
			m.applyFilters(true)
		case "2":
			m.showWarn = !m.showWarn
			m.applyFilters(true)
		case "3":
			m.showInfo = !m.showInfo
			m.applyFilters(true)
		case "4":
			m.showDebug = !m.showDebug
			m.applyFilters(true)
		case "R":
			m.regexMode = !m.regexMode
			m.applyFilters(true) // Re-apply to update regex usage

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

		// Clear all filters
		case "c":
			m.startDate = nil
			m.endDate = nil
			m.filterText = ""
			m.regexMode = false
			m.applyFilters(true)

		// Toggle Follow Mode
		case "f":
			m.following = !m.following
			if m.following {
				m.yOffset = len(m.filteredLines) - m.viewport.Height
				if m.yOffset < 0 {
					m.yOffset = 0
				}
			}

		// Toggle Stack Trace Folding
		case "z":
			m.foldStackTraces = !m.foldStackTraces
			m.applyFilters(true)

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
			row := m.yOffset
			if _, exists := m.bookmarks[row]; exists {
				delete(m.bookmarks, row)
			} else {
				m.bookmarks[row] = struct{}{}
			}
			// Invalidate cache for this line to ensure layout updates (e.g. bookmark icon vs gutter)
			delete(m.layoutCache, row)

		case "n":
			// Jump to next bookmark > current YOffset
			start := m.yOffset + 1
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
				m.yOffset = next
			}

		case "N":
			// Jump to prev bookmark < current YOffset
			start := m.yOffset - 1
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
				m.yOffset = prev
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
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if msg.Shift || msg.Alt || msg.Ctrl {
				m.xOffset -= 5
				if m.xOffset < 0 {
					m.xOffset = 0
				}
			} else {
				m.yOffset--
			}
		case tea.MouseButtonWheelDown:
			if msg.Shift || msg.Alt || msg.Ctrl {
				m.xOffset += 5
			} else {
				m.yOffset++
			}
		case tea.MouseButtonWheelLeft:
			m.xOffset -= 5
			if m.xOffset < 0 {
				m.xOffset = 0
			}
		case tea.MouseButtonWheelRight:
			m.xOffset += 5
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

	// We manage mouse scrolling via m.yOffset. Passing mouse events to viewport
	// causes duplicate wheel handling and over-scroll behavior.
	if _, isMouse := msg.(tea.MouseMsg); !isMouse {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Timeline viewport only needs non-mouse updates too.
		if m.showTimeline {
			m.timelineViewport, cmd = m.timelineViewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) canFastAppendWithoutRefilter() bool {
	return m.filterText == "" &&
		m.startDate == nil &&
		m.endDate == nil &&
		m.showError &&
		m.showWarn &&
		m.showInfo &&
		m.showDebug &&
		!m.foldStackTraces
}

func (m *Model) appendIncomingLines(newLines []string) {
	if len(newLines) == 0 {
		return
	}

	m.originalLines = append(m.originalLines, newLines...)

	if m.canFastAppendWithoutRefilter() {
		for _, line := range newLines {
			line = strings.ReplaceAll(line, "\t", "    ")
			m.filteredLines = append(m.filteredLines, line)
		}
		return
	}

	// Keep current viewport state while recomputing.
	m.applyFilters(false)
}

func splitIncomingContent(content string) []string {
	lines := strings.Split(content, "\n")
	// File appends usually end with '\n', which creates a trailing empty element.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (m *Model) applyFilters(resetView bool) {
	var filtered []string
	// Directly iterate over originalLines
	lines := m.originalLines

	// Pre-compile regex if in regex mode
	if m.filterText != "" {
		var err error
		if m.regexMode {
			m.regex, err = regexp.Compile(m.filterText)
		} else {
			m.regex, err = regexp.Compile("(?i)" + regexp.QuoteMeta(m.filterText))
		}
		if err != nil {
			m.regex = nil
		}
	} else {
		m.regex = nil
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
			if m.regexMode {
				if m.regex != nil && !m.regex.MatchString(line) {
					continue
				}
			} else {
				// Case-insensitive contains (old robust behavior)
				if !strings.Contains(strings.ToLower(line), strings.ToLower(m.filterText)) {
					continue
				}
			}
		}

		// Tab Normalization (Fixes offset drift in selection)
		line = strings.ReplaceAll(line, "\t", "    ")

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

	if resetView {
		// Clear selection on filter change
		m.selectionStart = nil
		m.selectionEnd = nil
		// Clear bookmarks on filter change? indices are invalid.
		m.bookmarks = make(map[int]struct{})

		// Virtualization reset
		m.yOffset = 0
	}
	// Always clear viewport content as View() reconstructs it
	m.viewport.SetContent("")

	// Clear height cache on filter change
	if resetView {
		m.layoutCache = make(map[int][]string)
	}
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	if m.showHelp {
		// Calculate total height
		height := m.viewport.Height + m.headerHeight + m.footerHeight
		if height == 0 {
			height = 20
		} // fallback
		width := m.screenWidth
		if width == 0 {
			width = 80
		}

		return lipgloss.Place(width, height,
			lipgloss.Center, lipgloss.Center,
			m.helpView(),
		)
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
			// Check Cache first
			parts, cached := m.layoutCache[realLineIndex]
			var wrapped string

			if cached {
				// If cached, join parts to reconstruct wrapped line
				// This is much faster than re-wrapping with Lipgloss
				wrapped = strings.Join(parts, "\n")

				// Note: Cached parts are just "plain" wrapped or "decorated" wrapped?
				// resolvePos logic puts "plain" wrapped in cache usually to optimize search.
				// But View needs DECORATED lines (highlighted).
				// If we cache PLAIN lines, we miss highlighting.
				// If we cache DECORATED lines, resolvePos logic might be tricky?
				//
				// Let's check resolvePos usage of cache:
				// resolvePos puts `lipgloss.NewStyle().Width(width).Render(plain)` -> parts
				// So it caches formatted/wrapped but maybe NOT highlighted for search/regex?
				// Actually resolvePos uses:
				//   plain = stripAnsi(line)
				//   wrapped = lipgloss...Render(plain)
				// So resolvePos caches stripped content.
				//
				// View needs HIGHLIGHTED content.
				//
				// DECISION: We probably need TWO caches or one robust cache.
				// Given performance is the goal, caching the FINAL RENDERED wrapped lines is best for View.
				// But resolvePos needs to map valid indices.
				//
				// If we change layoutCache to store the Final View Content, resolvePos might break if it relied on stripping.
				//
				// Let's refactor layoutCache to strictly store "Visual Lines" (what View needs).
				// `resolvePos` can interpret Visual Lines if needed, OR we just accept we cache View lines.
				//
				// Optimization: The 'slow' part is Lipgloss Wrapping & Regex highlighting on long lines.
				// If we do decoration BEFORE wrapping, we assume valid ANSI wrapping.
				// Lipgloss handles ANSI wrapping well.
				//
				// Let's store the Fully Decorated & Wrapped lines in cache.
			}

			if !cached {
				// 0. Highlight Matches (Priority)
				line = m.getDecoratedLine(realLineIndex, line)

				// 1. Apply Selection
				// (Selection is dynamic! If we cache with selection, we break selection updates on drag)
				// Selection is fast (string manipulation). Wrapping is slow.
				// We should cache the WRAPPED line *without* selection?
				// But Selection highlights spans.

				// Alternative: Cache the WRAPPED result of decorated line (regex, etc).
				// apply selection On Top?
				// If we apply selection on wrapped lines, we need to map logical-to-visual indices... complex.

				// Let's look at how slow "applying selection" is.
				// It's just string slicing. Fast.
				// The slow part is `lipgloss.NewStyle().Width(width).Render(line)`.

				// SO:
				// 1. Decorate Line (Regex, Levels) -> Cached? No, fast enough usually.
				// 2. Wrap Line -> SLOW.
				// 3. Apply Selection -> dynamic.
				//
				// Problem: If we wrap first, selection indices are hard.
				// Current code: Decorate -> Select -> Wrap.

				// If we want to cache, we must cache the Result of (Decorate -> Wrap) MINUS selection?
				// Or does selection happen before wrap? YES.
				// "line = pre + sel + post"
				// "wrapped = lipgloss...Render(line)"

				// If selection changes, we MUST re-wrap because correct wrapping depends on escape codes?
				// Actually, selection just adds ANSI codes. It shouldn't change word boundaries (unless selection is bold?).
				// Selected style: Background color. Zero width ANSI.
				// So wrapping should be identical "shape".

				// Optimization:
				// Cache the "Base Wrapped Lines" (Decorated, No Selection).
				// When Selection is active, we might have to bite the bullet and re-wrap?
				// OR process selection on the wrapped parts?

				// User complaint: "After applying a filter pressed w scrolling is very slow".
				// They didn't say "during selection". Just scrolling.
				// In normal scrolling, selection is nil.

				// So, if !selecting, we can cache result!

				width := m.screenWidth
				if width <= 0 {
					width = 80
				}

				renderedLine := line // Start with raw

				// Apply decorations (Regex, Level)
				renderedLine = m.getDecoratedLine(realLineIndex, renderedLine)

				// Cache Key: We use index. If content changes, filter clears cache.
				// If selection exists, we might normally bypass cache or modify key.
				// But simplest fix for "scrolling is slow":
				// Cache the final wrapped string *if no selection*.

				hasSelection := m.selectionStart != nil && m.selectionEnd != nil
				// If selection touches this line, do dynamic.
				// Check intersection
				lineSelected := false
				if hasSelection {
					startSel, endSel := *m.selectionStart, *m.selectionEnd
					if startSel.Y > endSel.Y || (startSel.Y == endSel.Y && startSel.X > endSel.X) {
						startSel, endSel = endSel, startSel
					}
					if realLineIndex >= startSel.Y && realLineIndex <= endSel.Y {
						lineSelected = true
					}
				}

				// To minimize code drift, let's just Wrap and cache if !lineSelected.

				// Re-running generation:
				line = m.getDecoratedLine(realLineIndex, line)
				if lineSelected {
					// Apply selection logic (same as before)
					startSel, endSel := *m.selectionStart, *m.selectionEnd
					if startSel.Y > endSel.Y || (startSel.Y == endSel.Y && startSel.X > endSel.X) {
						startSel, endSel = endSel, startSel
					}
					// ... logic copy ...
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
					if startCol < 0 {
						startCol = 0
					}
					if startCol > len(runes) {
						startCol = len(runes)
					}
					if endCol < 0 {
						endCol = 0
					}
					if endCol > len(runes) {
						endCol = len(runes)
					}

					if startCol < endCol {
						pre := string(runes[:startCol])
						sel := selectedStyle.Render(string(runes[startCol:endCol]))
						post := string(runes[endCol:])
						line = pre + sel + post
					}
				}

				wrapped = lipgloss.NewStyle().Width(width).Render(line)

				// Store in cache only if NOT selected (or if selected? selection changes often)
				// If we cache selected state, dragging execution is slow?
				// But dragging is mouse motion.
				// User complaint is SCROLLING.
				// So caching the clean wrapped state is critical.

				if !lineSelected {
					m.layoutCache[realLineIndex] = strings.Split(wrapped, "\n")
				}
			}

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
				visiblePart := string(rawRunes[m.xOffset:end])

				// Highlight visible part
				visiblePart = highlightMatches(visiblePart, m.regex)
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
						if visStart < 0 {
							visStart = 0
						}
						if visStart > len([]rune(visiblePart)) {
							visStart = len([]rune(visiblePart))
						}
						if visEnd < 0 {
							visEnd = 0
						}
						if visEnd > len([]rune(visiblePart)) {
							visEnd = len([]rune(visiblePart))
						}

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
	if interval == 0 {
		interval = time.Minute
	}

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

func highlightMatches(line string, re *regexp.Regexp) string {
	if re == nil {
		return line
	}

	return re.ReplaceAllStringFunc(line, func(match string) string {
		return matchStyle.Render(match)
	})
}

var ansiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\x07)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
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
	loc := dateRegex.FindStringIndex(line)
	if loc != nil {
		s := line[loc[0]:loc[1]]
		t, err := parseDate(s)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var dateRegex = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]?(\d{2}:\d{2}:\d{2})?`)

func colorizeJSON(s string) string {
	return jsonRegex.ReplaceAllStringFunc(s, func(match string) string {
		return jsonKeyStyle.Render(match)
	})
}

var jsonRegex = regexp.MustCompile(`"([^"]+)":`)

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
		if percent < 0 {
			percent = 0
		}
		if percent > 1 {
			percent = 1
		}
	}
	status := fmt.Sprintf(" %3.f%% ", percent*100)

	// Line Counts
	status += fmt.Sprintf("│ Lines: %d/%d ", len(m.filteredLines), len(m.originalLines))
	status += fmt.Sprintf("│ X: %d ", m.xOffset)

	if m.startDate != nil {
		status += fmt.Sprintf("│ Start: %s ", m.startDate.Format("15:04"))
	}
	if m.endDate != nil {
		status += fmt.Sprintf("│ End: %s ", m.endDate.Format("15:04"))
	}

	if m.following {
		// Blinking indicator? Or just bold color?
		status += "│ " + infoStyle.Render("LIVE")
	}

	// Right aligned help hint
	help := " ? Help "

	// Assemble
	totalWidth := m.viewport.Width
	leftSide := status

	// Spacer
	spaceCount := max(0, totalWidth-lipgloss.Width(leftSide)-lipgloss.Width(help))
	line := strings.Repeat("─", spaceCount)

	return lipgloss.JoinHorizontal(lipgloss.Center, leftSide, line, help)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) helpView() string {
	// Define helper struct for keys
	type helpEntry struct {
		key, desc string
	}

	// Group keys
	general := []helpEntry{
		{"?", "Close Help"},
		{"q / esc", "Quit / Close"},
		{"ctrl+c", "Quit"},
	}

	nav := []helpEntry{
		{"j / k", "Scroll Down / Up"},
		{"g / G", "Scroll Top / Bottom"},
		{"f", "Toggle Follow"},
		{"J", "Jump to Time"},
		{"space", "Page Down"},
		{"b / u", "PgUp / PgDown"},
	}

	filtering := []helpEntry{
		{"/", "Filter Logs"},
		{"c", "Clear Filters"},
		{"R", "Regex Toggle"},
		{"s / e", "Set Start / End (Time)"},
		{"1-4", "Toggle Levels (Err/Warn...)"},
	}

	viewing := []helpEntry{
		{"w", "Toggle Wrap"},
		{"z", "Fold Stack Traces"},
		{"t", "Toggle Timeline"},
		{"m", "Toggle Bookmark"},
		{"l / h", "Scroll Right / Left"},
		{"Shift+Wheel", "Scroll Right / Left"},
		{"r", "Reload File"},
	}

	// Styles
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Margin(1)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Bold(true).
		Underline(true).
		MarginBottom(1)

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Helper to render a column
	renderColumn := func(title string, entries []helpEntry) string {
		s := headerStyle.Render(title) + "\n"
		for _, e := range entries {
			k := keyStyle.Width(10).Render(e.key)
			d := descStyle.Render(e.desc)
			s += fmt.Sprintf("%s %s\n", k, d)
		}
		return s
	}

	// Layout
	col1 := renderColumn("General", general) + "\n" + renderColumn("Navigation", nav)
	col2 := renderColumn("Filtering", filtering) + "\n" + renderColumn("View & Tools", viewing)

	// Join columns with gap
	content := lipgloss.JoinHorizontal(lipgloss.Top, col1, "    ", col2)

	return boxStyle.Render(content)
}

func (m *Model) copySelection() {
	if m.selectionStart != nil && m.selectionEnd != nil {
		start, end := *m.selectionStart, *m.selectionEnd
		// Normalize
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
			if startCol < 0 {
				startCol = 0
			}
			if startCol > len(runes) {
				startCol = len(runes)
			}
			if endCol < 0 {
				endCol = 0
			}
			if endCol > len(runes) {
				endCol = len(runes)
			}

			if startCol < endCol {
				selectedLines = append(selectedLines, string(runes[startCol:endCol]))
			} else {
				// Empty line or invalid range
				selectedLines = append(selectedLines, "")
			}
		}

		text := strings.Join(selectedLines, "\n")
		clipboard.WriteAll(text)

		// Clear selection after copy?
		// User preference: might want to keep selection?
		// Existing behavior was to clear. Let's keep it consistent.
		m.selectionStart = nil
		m.selectionEnd = nil
	}
}

func (m Model) getDecoratedLine(i int, line string) string {
	line = highlightMatches(line, m.regex)
	line = highlightLine(line)
	if _, ok := m.bookmarks[i]; ok {
		line = "🔖 " + line
	}
	return line
}

func (m Model) resolvePos(visualX, visualY int) (int, int) {
	if !m.wrap {
		// Default behavior (No Wrap)
		logicalLine := m.yOffset + visualY
		gutterOffset := 3
		logicalX := m.xOffset + visualX - gutterOffset
		if logicalX < 0 {
			logicalX = 0
		}
		return logicalLine, logicalX
	}

	width := m.screenWidth
	if width <= 0 {
		width = 80
	}

	currentVisualY := 0
	targetLineIndex := -1
	targetCharIndex := 0

	// Iterate through lines starting from scroll offset
	// Check until we reach the visualY we clicked on
	for i := 0; i+m.yOffset < len(m.filteredLines); i++ {
		idx := m.yOffset + i
		line := m.filteredLines[idx]

		// Use Cache to skip expensive wrapping
		parts, cached := m.layoutCache[idx]

		var plain string

		if !cached {
			// OPTIMIZATION: operate on plain string.
			// Decoration adds ANSI (zero width) + potentially Bookmark (2 chars).
			// Timestamps/JSON coloring are just ANSI.
			// So wrapping 'plain' should match wrapping 'decorated'.

			plain = stripAnsi(line)
			if _, ok := m.bookmarks[idx]; ok {
				plain = "🔖 " + plain
			}

			// Wrap plain text
			wrapped := lipgloss.NewStyle().Width(width).Render(plain)
			parts = strings.Split(wrapped, "\n")
			m.layoutCache[idx] = parts
		}

		h := len(parts)
		if visualY < currentVisualY+h {
			// Found the line!
			targetLineIndex = idx

			// Need plain string now
			if plain == "" {
				plain = stripAnsi(line)
				if _, ok := m.bookmarks[idx]; ok {
					plain = "🔖 " + plain
				}
			}

			// Reconstruct offset by matching parts against original plain line
			originalClean := plain // This is what we built above
			currentByteOffset := 0
			currentRuneOffset := 0

			localRow := visualY - currentVisualY

			// Iterate up to localRow to find start index of current line segment
			for k := 0; k < localRow; k++ {
				part := parts[k]

				// Safety check: if we already exceeded length, stop
				if currentByteOffset >= len(originalClean) {
					break
				}

				matchIdx := strings.Index(originalClean[currentByteOffset:], part)
				if matchIdx == -1 {
					// Fallbck: assume it was just skipped or expanded?
					// Just advance by part length to be safe-ish
					currentByteOffset += len(part)
					currentRuneOffset += utf8.RuneCountInString(part)
				} else {
					// 1. Add skipped characters (e.g. spaces eaten by wrap)
					if matchIdx > 0 {
						skippedBytes := matchIdx
						skippedPart := originalClean[currentByteOffset : currentByteOffset+skippedBytes]
						currentRuneOffset += utf8.RuneCountInString(skippedPart)
						currentByteOffset += skippedBytes
					}

					// 2. Add the part itself
					currentByteOffset += len(part)
					currentRuneOffset += utf8.RuneCountInString(part)
				}
			}

			// Find start of current line (target line)
			currentPart := parts[localRow]
			startOfLineRuneIdx := currentRuneOffset

			if currentByteOffset < len(originalClean) {
				matchIdx := strings.Index(originalClean[currentByteOffset:], currentPart)
				if matchIdx != -1 {
					// Add any skipped chars before this line starts
					skippedPart := originalClean[currentByteOffset : currentByteOffset+matchIdx]
					startOfLineRuneIdx += utf8.RuneCountInString(skippedPart)
				}
			}

			// Add current visual X logic (runewidth)
			currentSegRunes := []rune(currentPart)

			// Iterate runes to find which one covers visualX
			cw := 0
			foundIdx := len(currentSegRunes)

			for rIdx, r := range currentSegRunes {
				w := runewidth.RuneWidth(r)
				if cw+w > visualX {
					foundIdx = rIdx
					break
				}
				cw += w
			}

			// If bookmarked, the first 2 chars are "🔖 " (idx 0, 1? rune length 2?)
			// Bookmark is "🔖 " -> Rune count: 2 (Bookmark char + space).
			// We want index into the LOG LINE (without bookmark).

			finalIdx := startOfLineRuneIdx + foundIdx

			if _, ok := m.bookmarks[idx]; ok {
				// Original plain was "🔖 " + content
				// We want index into content.
				// "🔖 " is 2 runes?
				bookmarkPrefixLen := utf8.RuneCountInString("🔖 ")
				finalIdx -= bookmarkPrefixLen
				if finalIdx < 0 {
					finalIdx = 0
				}
			}

			targetCharIndex = finalIdx
			return targetLineIndex, targetCharIndex
		}

		currentVisualY += h

		if currentVisualY > visualY {
			break
		}
	}

	return -1, -1
}
