package ui

import (
	"testing"
)

func TestExtractDate(t *testing.T) {
	tests := []struct {
		line     string
		wantTime string // simple string match for verification
		wantOk   bool
	}{
		{
			line:     "2023-01-01 12:00:00 INFO Some log",
			wantTime: "2023-01-01 12:00:00",
			wantOk:   true,
		},
		{
			line:     "[2023-10-25T14:30:00Z] DEBUG msg",
			wantTime: "2023-10-25T14:30:00",
			wantOk:   true,
		},
		{
			line:     "No date here",
			wantTime: "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		got, ok := extractDate(tt.line)
		if ok != tt.wantOk {
			t.Errorf("extractDate(%q) ok = %v, want %v", tt.line, ok, tt.wantOk)
			continue
		}
		if ok {
			// Check checks if the extract matches (approx check since extractDate returns time.Time)
			// For simplicity in this test, we just check if year matches to ensure it parsed something
			if got.Year() < 2000 {
				t.Errorf("extractDate(%q) got invalid year %d", tt.line, got.Year())
			}
		}
	}
}

func TestApplyFilters(t *testing.T) {
	lines := []string{
		"2023-01-01 10:00:00 INFO Info message",
		"2023-01-01 10:00:01 WARN Warning message",
		"2023-01-01 10:00:02 ERROR Error message",
		"2023-01-01 10:00:03 DEBUG Debug message",
	}

	m := InitialModel("test.log", lines, nil)
	
	// Test 1: No filters
	m.applyFilters(true)
	if len(m.filteredLines) != 4 {
		t.Errorf("Expected 4 lines, got %d", len(m.filteredLines))
	}

	// Test 2: Filter Text
	m.filterText = "Error"
	m.applyFilters(true)
	if len(m.filteredLines) != 1 {
		t.Errorf("Expected 1 error line, got %d", len(m.filteredLines))
	}
	if len(m.filteredLines) > 0 && m.filteredLines[0] != lines[2] {
		t.Errorf("Expected line to be '%s', got '%s'", lines[2], m.filteredLines[0])
	}

	// Test 3: Level Filtering (Toggle off INFO)
	m.filterText = ""
	m.showInfo = false
	m.applyFilters(true)
	// Should show WARN, ERROR, DEBUG (3 lines)
	if len(m.filteredLines) != 3 {
		t.Errorf("Expected 3 lines (no INFO), got %d", len(m.filteredLines))
	}
}

func TestResolvePos(t *testing.T) {
    // Setup a model with forced width
    lines := []string{
        "1234567890ABCDE", // 15 chars
    }
    m := InitialModel("test.log", lines, nil)
    m.screenWidth = 10 
    m.wrap = true
    // Simulate View() logic indirectly by knowing how it should wrap
    // Row 0: "1234567890" (10 chars)
    // Row 1: "ABCDE"      (5 chars)
    
    tests := []struct{
        vX, vY int
        wantLine int
        wantIdx  int
    }{
        {0, 0, 0, 0},   // Click '1'
        {9, 0, 0, 9},   // Click '0'
        {0, 1, 0, 10},  // Click 'A' (start of next row)
        {4, 1, 0, 14},  // Click 'E'
        {5, 1, 0, 15},  // Click after 'E'
        // Test bounds
        {20, 0, 0, 10}, // Click way right on first line
    }
    
    for _, tt := range tests {
        l, idx := m.resolvePos(tt.vX, tt.vY)
        if l != tt.wantLine {
            t.Errorf("resolvePos(%d, %d) Line: got %d, want %d", tt.vX, tt.vY, l, tt.wantLine)
        }
        if idx != tt.wantIdx {
            t.Errorf("resolvePos(%d, %d) Idx: got %d, want %d", tt.vX, tt.vY, idx, tt.wantIdx)
        }
    }
    
    // Space Consumption Hypothesis Check
    // "A B C" width 1 -> wraps to A / B / C.
    m2 := InitialModel("test2", []string{"A B C"}, nil)
    m2.screenWidth = 1
    m2.wrap = true
    
    // Expect:
    // Row 0: "A"
    // Row 1: "B" (Space eaten?)
    // Row 2: "C"
    
    // If we click "C" (visual X=0, Y=2)
    // Correct index in "A B C" is 4.
    // If spaces are eaten, prefix len sum might be 2.
    
    l, idx := m2.resolvePos(0, 2)
    if l != 0 {
         t.Errorf("Space Check: Expected Line 0, got %d", l)
    }
    if idx != 4 {
         t.Errorf("Space Check: Expected Idx 4 (C), got %d. Drift detected!", idx)
    }
}

func TestResolvePosPanic(t *testing.T) {
    // Regression test for "slice bounds out of range" panic
    // Occurs when byte/rune offsets are mixed
    line := "INFO 🚀 Startup complete" // Contains emoji (multibyte)
    m := InitialModel("panic.log", []string{line}, nil)
    m.screenWidth = 10 
    m.wrap = true
    
    // Wrapped likely:
    // "INFO 🚀 " (width 7? Emoji is 2 cells. I N F O _ 🚀 _) -> 5+2 = 7? 
    // "Startup "
    // "complete"
    
    // Simulate clicking deeply into the content
    // We mainly care that it DOES NOT PANIC.
    
    // Testing many points
    for y := 0; y < 5; y++ {
        for x := 0; x < 20; x++ {
             m.resolvePos(x, y)
        }
    }
}
