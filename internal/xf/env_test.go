package xf

import "testing"

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "plain", want: false},
		{value: "has space", want: true},
		{value: "$VAR", want: true},
		{value: "${VAR}", want: false},
	}

	for _, tt := range tests {
		if got := needsQuoting(tt.value); got != tt.want {
			t.Fatalf("needsQuoting(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
