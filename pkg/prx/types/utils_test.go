package types

import (
	"strings"
	"testing"
)

func TestContainsQuestion(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Question mark tests
		{
			name:     "explicit question mark",
			text:     "What is this?",
			expected: true,
		},
		{
			name:     "question mark in middle",
			text:     "Really? I don't think so.",
			expected: true,
		},
		// Direct questions
		{
			name:     "how can pattern",
			text:     "How can we fix this?",
			expected: true,
		},
		{
			name:     "how do pattern",
			text:     "How do you think we should proceed",
			expected: true,
		},
		{
			name:     "how would pattern",
			text:     "How would this work",
			expected: true,
		},
		{
			name:     "how should pattern",
			text:     "How should I implement this",
			expected: true,
		},
		{
			name:     "how about pattern",
			text:     "How about we try a different approach",
			expected: true,
		},
		{
			name:     "how to pattern",
			text:     "How to test this properly",
			expected: true,
		},
		// Modal questions
		{
			name:     "should I pattern",
			text:     "Should I merge this now",
			expected: true,
		},
		{
			name:     "should we pattern",
			text:     "Should we wait for approval",
			expected: true,
		},
		{
			name:     "can I pattern",
			text:     "Can I get a review on this",
			expected: true,
		},
		{
			name:     "can you pattern",
			text:     "Can you take a look at this",
			expected: true,
		},
		{
			name:     "could you pattern",
			text:     "Could you review this code",
			expected: true,
		},
		{
			name:     "would you pattern",
			text:     "Would you approve this change",
			expected: true,
		},
		// What questions
		{
			name:     "what do you think",
			text:     "What do you think about this approach",
			expected: true,
		},
		{
			name:     "what's the pattern",
			text:     "What's the best way to handle this",
			expected: true,
		},
		{
			name:     "what is the pattern",
			text:     "What is the status of the tests",
			expected: true,
		},
		{
			name:     "what about pattern",
			text:     "What about using a different library",
			expected: true,
		},
		// Request patterns
		{
			name:     "any suggestions",
			text:     "Any suggestions for improving this",
			expected: true,
		},
		{
			name:     "any ideas",
			text:     "Any ideas on how to fix this",
			expected: true,
		},
		{
			name:     "anyone know",
			text:     "Anyone know why this is failing",
			expected: true,
		},
		{
			name:     "does anyone pattern",
			text:     "Does anyone have time to review",
			expected: true,
		},
		{
			name:     "do we pattern",
			text:     "Do we need to update the docs",
			expected: true,
		},
		// Possibility questions
		{
			name:     "is it possible",
			text:     "Is it possible to merge this today",
			expected: true,
		},
		{
			name:     "is there a way",
			text:     "Is there a way to speed this up",
			expected: true,
		},
		{
			name:     "is this pattern",
			text:     "Is this ready to merge",
			expected: true,
		},
		// Indirect questions
		{
			name:     "wondering if",
			text:     "I was wondering if this is correct",
			expected: true,
		},
		{
			name:     "thoughts on",
			text:     "Thoughts on this implementation",
			expected: true,
		},
		{
			name:     "need help",
			text:     "I need help with this error",
			expected: true,
		},
		// Why/when/where questions
		{
			name:     "why is pattern",
			text:     "Why is this test failing",
			expected: true,
		},
		{
			name:     "when should pattern",
			text:     "When should we deploy this",
			expected: true,
		},
		{
			name:     "where can pattern",
			text:     "Where can I find the documentation",
			expected: true,
		},
		{
			name:     "who can pattern",
			text:     "Who can approve this PR",
			expected: true,
		},
		// Have/has questions
		{
			name:     "have you pattern",
			text:     "Have you tested this locally",
			expected: true,
		},
		{
			name:     "has anyone pattern",
			text:     "Has anyone seen this issue before",
			expected: true,
		},
		// Non-questions
		{
			name:     "statement with question word",
			text:     "I know how this works",
			expected: false,
		},
		{
			name:     "partial match should not trigger",
			text:     "I showed the results to the team",
			expected: false,
		},
		{
			name:     "empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "very short text",
			text:     "ok",
			expected: false,
		},
		{
			name:     "statement without question patterns",
			text:     "This looks good to me. LGTM!",
			expected: false,
		},
		{
			name:     "imperative command",
			text:     "Please merge this when ready",
			expected: false,
		},
		// Word boundary tests
		{
			name:     "word boundary - how in middle",
			text:     "I showed them the solution",
			expected: false,
		},
		{
			name:     "word boundary - can in cannot",
			text:     "I cannot approve this yet",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsQuestion(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsQuestion(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string",
			input:    "Hello",
			expected: "Hello",
		},
		{
			name:     "exactly max length",
			input:    strings.Repeat("a", maxTruncateLength),
			expected: strings.Repeat("a", maxTruncateLength),
		},
		{
			name:     "one over max length",
			input:    strings.Repeat("a", maxTruncateLength+1),
			expected: strings.Repeat("a", maxTruncateLength),
		},
		{
			name:     "much longer than max",
			input:    strings.Repeat("a", 500),
			expected: strings.Repeat("a", maxTruncateLength),
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:  "unicode characters",
			input: strings.Repeat("→", 200), // Each → is 3 bytes, so 200 chars = 600 bytes
			// Truncate operates on byte length, so result will be truncated at 256 bytes
			// which will cut off mid-character, but Go handles this gracefully
			expected: strings.Repeat("→", 200)[:maxTruncateLength],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input)
			if result != tt.expected {
				t.Errorf("Truncate() length = %d, expected %d", len(result), len(tt.expected))
			}
			if len(result) > maxTruncateLength {
				t.Errorf("Truncate() returned string longer than max: %d > %d", len(result), maxTruncateLength)
			}
		})
	}
}

func BenchmarkContainsQuestion(b *testing.B) {
	texts := []string{
		"What is this?",
		"How can we fix this issue",
		"This is a statement without questions",
		"Any suggestions on this approach",
		"I showed them the solution",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, text := range texts {
			ContainsQuestion(text)
		}
	}
}
