package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestResolveModelPlatform(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-4o", domain.PlatformOpenAI},
		{"gpt-4.1-mini", domain.PlatformOpenAI},
		{"gpt-4o-mini-2024-07-18", domain.PlatformOpenAI},
		{"o1-preview", domain.PlatformOpenAI},
		{"o3-mini", domain.PlatformOpenAI},
		{"chatgpt-4o-latest", domain.PlatformOpenAI},
		{"dall-e-3", domain.PlatformOpenAI},
		{"claude-opus-4-8-20250609", domain.PlatformAnthropic},
		{"claude-sonnet-4-6-20250514", domain.PlatformAnthropic},
		{"claude-3-5-haiku-20241022", domain.PlatformAnthropic},
		{"gemini-2.5-pro", domain.PlatformGemini},
		{"gemini-2.0-flash", domain.PlatformGemini},
		{"deepseek-chat", domain.PlatformAnthropic},
		{"deepseek-reasoner", domain.PlatformAnthropic},
		{"unknown-model", domain.PlatformAnthropic}, // default fallback
		{"GPT-4O", domain.PlatformOpenAI},         // case insensitive
		{"Claude-Opus-4", domain.PlatformAnthropic},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := resolveModelPlatform(tt.model)
			require.Equal(t, tt.expected, got, "resolveModelPlatform(%q)", tt.model)
		})
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		match   bool
	}{
		// Exact match (no wildcard).
		{"deepseek-chat", "deepseek-chat", true},
		{"deepseek-chat", "deepseek-reasoner", false},

		// Wildcard at end.
		{"claude-opus-*", "claude-opus-4-8-20250609", true},
		{"claude-opus-*", "claude-sonnet-4-6", false},
		{"gpt-4*", "gpt-4o", true},
		{"gpt-4*", "gpt-4.1", true},
		{"gpt-4*", "gpt-3.5", false},
		{"claude-*", "claude-opus-4-8", true},
		{"claude-opus-4*", "claude-opus-4-8-20250609", true},

		// Star-only pattern matches everything.
		{"*", "anything", true},
		{"*", "", true},

		// Empty pattern matches everything (treated as match-all).
		{"", "anything", true},
	}

	for _, tt := range tests {
		name := tt.pattern + "_" + tt.s
		t.Run(name, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.s)
			require.Equal(t, tt.match, got, "matchGlob(%q, %q)", tt.pattern, tt.s)
		})
	}
}
