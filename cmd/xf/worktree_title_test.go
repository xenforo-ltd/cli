package main

import "testing"

func TestRetitleBoard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		label string
		want  string
	}{
		{
			name:  "plain title gains a suffix",
			title: "XenForo",
			label: "slack-unfurl",
			want:  "XenForo [slack-unfurl]",
		},
		{
			name:  "existing suffix is replaced",
			title: "XenForo [main]",
			label: "slack-unfurl",
			want:  "XenForo [slack-unfurl]",
		},
		{
			name:  "version-style suffix is replaced",
			title: "XenForo [2.4]",
			label: "slack-unfurl",
			want:  "XenForo [slack-unfurl]",
		},
		{
			name:  "brackets elsewhere are left alone",
			title: "XenForo [beta] forums",
			label: "feature",
			want:  "XenForo [beta] forums [feature]",
		},
		{
			name:  "trailing whitespace is tidied",
			title: "XenForo   ",
			label: "feature",
			want:  "XenForo [feature]",
		},
		{
			name:  "empty title becomes just the label",
			title: "",
			label: "feature",
			want:  "[feature]",
		},
		{
			name:  "empty suffix is replaced rather than kept",
			title: "XenForo []",
			label: "feature",
			want:  "XenForo [feature]",
		},
		{
			name:  "nested brackets are not mangled",
			title: "XenForo [a [b]]",
			label: "feature",
			want:  "XenForo [a [b]] [feature]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := retitleBoard(tt.title, tt.label); got != tt.want {
				t.Errorf("retitleBoard(%q, %q) = %q, want %q", tt.title, tt.label, got, tt.want)
			}
		})
	}
}

// TestRetitleBoardIsIdempotent matters because cloning a clone must not
// accumulate suffixes.
func TestRetitleBoardIsIdempotent(t *testing.T) {
	t.Parallel()

	once := retitleBoard("XenForo [main]", "feature")
	twice := retitleBoard(once, "feature")

	if once != twice {
		t.Errorf("applying twice changed the result: %q then %q", once, twice)
	}
}
