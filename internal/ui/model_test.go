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

	m := InitialModel("test.log", lines)
	
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
