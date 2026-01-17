
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestResolvePos checks the logic of resolvePos for correctness directly.


func TestResolvePos_WithBookmark_NoWrap(t *testing.T) {
	line := "Hello World"
	m := InitialModel("test.log", []string{line}, nil)
	m.screenWidth = 80
	m.wrap = false
	
	// Add bookmark at row 0
	m.bookmarks[0] = struct{}{}
	
	// Visual Layout (No Wrap):
	// "   " + line (Normal)
	// "🔖 " + line (Bookmarked)
	// "🔖 " is 2 runes? No, "🔖" is 1 rune (U+1F516), Space is 1 rune.
	// Visual Width: "🔖" is 2 cells? Space is 1 cell. Total 3 cells.
	// "   " is 3 cells.
	// So offsets align.
	
	// We want to click 'H' in "Hello".
	// "🔖 " takes 3 cells (0, 1, 2). 'H' is at 3.
	visualX := 3
	visualY := 0
	
	logicalLine, logicalX := m.resolvePos(visualX, visualY)
	
	if logicalLine != 0 {
		t.Errorf("Expected line 0, got %d", logicalLine)
	}
	
	// logicalX should be index in "Hello World" -> 0.
	// existing logic: 
	// gutterOffset = 3 (if !wrap)
	// logicalX = xOffset (0) + visualX (3) - gutterOffset (3) = 0.
	
	if logicalX != 0 {
		t.Errorf("Expected logicalX 0 for 'H', got %d", logicalX)
	}
	
	// Now click 'W' (index 6 in "Hello ").
	// "Hello " is 6 chars. 'W' is 7th char?
	// "Hello World" -> H(0), e(1), l(2), l(3), o(4), " "(5), W(6).
	// Visual position of 'W': 3 (gutter) + 6 = 9.
	
	visualX = 9
	_, logicalX = m.resolvePos(visualX, visualY)
	if logicalX != 6 {
		t.Errorf("Expected logicalX 6 for 'W', got %d", logicalX)
	}
}

func TestResolvePos_CacheInvalidation(t *testing.T) {
	line := "Hello World"
	m := InitialModel("test.log", []string{line}, nil)
	m.screenWidth = 20
	m.wrap = true
	
	// 1. Resolve Pos (Populates Cache without bookmark)
	_, logX := m.resolvePos(0, 0)
	if logX != 0 {
		t.Fatalf("Initial resolve failed: got %d", logX)
	}
	
	// 2. Use Update to toggle bookmark (executes the Fix logic)
    // Key "m" triggers bookmark toggle.
    keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}
    updatedModel, _ := m.Update(keyMsg)
    m = updatedModel.(Model)
	
    // Verify bookmark is set (implicit check of Update logic)
    if _, ok := m.bookmarks[0]; !ok {
        t.Fatal("Bookmark was not set by Update")
    }

	// 3. Resolve Pos again. Cache should have been invalidated by Update.
	visualX := 3 // "🔖 " + "H"
	_, logX2 := m.resolvePos(visualX, 0)
	
	// Expectation: If cache was invalidated, wrap logic runs again with bookmark.
    // offsets shift. Visual 3 -> Index 0.
	if logX2 != 0 {
		t.Errorf("Cache staleness detected! Expected 0 (H), got %d. Cache was likely not cleared.", logX2)
	}
}


func TestResolvePos_ANSI(t *testing.T) {
	// "\x1b[31mHello\x1b[0m World"
	// "Hello" is red.
    // Visual: "Hello World" (11 chars).
    // Raw: has ANSI.
    // stripAnsi -> "Hello World".
    
	line := "\x1b[31mHello\x1b[0m World" 
	m := InitialModel("test.log", []string{line}, nil)
	m.screenWidth = 80
	m.wrap = false
	
	// Click 'e' in "Hello". (Index 1).
	// Visual X: 3 (gutter) + 1 = 4.
	visualX := 4
	visualY := 0
	
	_, logicalX := m.resolvePos(visualX, visualY)
	
	// logicalX should be index in stripAnsi(line) -> "Hello World".
	// Index 1 is 'e'.
	if logicalX != 1 {
		t.Errorf("Expected logicalX 1 for 'e', got %d", logicalX)
	}
}

func TestResolvePos_Tabs(t *testing.T) {
    // "\tHello" -> "    Hello" (4 spaces).
    // stripAnsi -> "    Hello".
    
    line := "    Hello" // applyFilters does expanding before model storage usually?
    // Wait, applyFilters expands tabs. m.filteredLines contains expaned tabs.
    // So if we pass "    Hello" to InitialModel (simulating applyFilters result), it mimics real state.
    
    m := InitialModel("test.log", []string{line}, nil)
    m.screenWidth = 80
    m.wrap = false
    
    // Click 'H' (Index 4).
    // Visual X: 3 (gutter) + 4 = 7.
    visualX := 7
    visualY := 0
    
    _, logicalX := m.resolvePos(visualX, visualY)
    
    if logicalX != 4 {
        t.Errorf("Expected logicalX 4 for 'H', got %d", logicalX)
    }
}


func TestResolvePos_WithBookmark_Wrap(t *testing.T) {
	line := "Hello World"
	m := InitialModel("test.log", []string{line}, nil)
	m.screenWidth = 20 // Narrow enough, but fits "Hello World" (11 chars)
	m.wrap = true
	
	// Add bookmark
	m.bookmarks[0] = struct{}{}
	
	// Wrap logic in resolvePos ADDS "🔖 " to plain string.
	// "🔖 " + "Hello World"
	// "🔖" (width 2? let's assume), " "(1), "Hello World"(11). Total Width ~14.
	// Should fit in 20.
	
	// Visual layout:
	// "🔖 Hello World"
	// 'H' is after space.
	// If "🔖" is 2 cells, " " is 1 cell. 'H' starts at 3.
	
	visualX := 3
	visualY := 0
	
	logicalLine, logicalX := m.resolvePos(visualX, visualY)
	
	// Expected: Index of 'H' in "Hello World" is 0.
	if logicalLine != 0 {
		t.Errorf("Expected line 0, got %d", logicalLine)
	}
	if logicalX != 0 {
		t.Errorf("Expected logicalX 0 for 'H' in wrap mode, got %d", logicalX)
	}
}
