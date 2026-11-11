package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTabTextStaysReadableOnStatusColor(t *testing.T) {
	tests := []struct {
		status string
		want   any
	}{
		{"#70728F", lightTabText}, // Taiga's default "New"
		{"#5C3566", lightTabText},
		{"#A8E440", darkTabText},
		{"#FFFFFF", darkTabText},
		{"#000", lightTabText},
		{"", darkTabText},       // fallback grey
		{"purple", darkTabText}, // invalid, fallback grey
	}
	for _, tt := range tests {
		if got := textOn(statusColor(tt.status)); got != tt.want {
			t.Errorf("text on %q: got %v, want %v", tt.status, got, tt.want)
		}
	}
	if got := statusColor("#abc"); got != lipgloss.Color("#abc") {
		t.Errorf("short hex not accepted: %v", got)
	}
}
