package cache

import "testing"

func TestSanitizePathComponent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "XF123-ABCD_1.2.3", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "slash", input: "../x", wantErr: true},
		{name: "parent", input: "..", wantErr: true},
		{name: "pathsep", input: "a/b", wantErr: true},
		{name: "backslash", input: `a\\b`, wantErr: true},
		{name: "dotdot", input: "a..b", wantErr: true},
		{name: "space", input: "bad name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizePathComponent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("sanitizePathComponent(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}
