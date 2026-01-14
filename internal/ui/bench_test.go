package ui

import (
	"os"
	"strings"
	"testing"
)

// BenchmarkStringProcessing simulates the overhead of the current architecture
// 1. Join all lines (simulates applyFilters)
// 2. Split all lines (simulates viewport.SetContent internal behavior)
func BenchmarkStringProcessing(b *testing.B) {
	// 1. Setup: Load large.log
	content, err := os.ReadFile("../../large.log")
	if err != nil {
		b.Skip("large.log not found")
	}
	lines := strings.Split(string(content), "\n")
	b.Logf("Loaded %d lines", len(lines))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Step 1: Join (simulate applyFilters ending)
		joined := strings.Join(lines, "\n")
		
		// Step 2: Split (simulate viewport initialization/SetContent)
		_ = strings.Split(joined, "\n")
	}
}

// BenchmarkDirectSlice simulates the proposed optimized architecture
// 1. Just access the slice (instant)
func BenchmarkDirectSlice(b *testing.B) {
    content, err := os.ReadFile("../../large.log")
	if err != nil {
		b.Skip("large.log not found")
	}
	lines := strings.Split(string(content), "\n")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Simulating accessing a window of lines
        // e.g. lines[0:100]
        start := 0
        end := 100
        if len(lines) < 100 { end = len(lines) }
        _ = lines[start:end]
    }
}

func BenchmarkResolvePos(b *testing.B) {
    // Setup model with long lines
    longLine := strings.Repeat("A long line with words and spaces to trigger lipgloss wrapping several times over. ", 50) // ~3000 chars
    lines := []string{longLine}
    m := InitialModel("bench.log", lines, nil)
    m.screenWidth = 80
    m.wrap = true
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Resolve a click near the end
        _, _ = m.resolvePos(40, 10) 
    }
}
