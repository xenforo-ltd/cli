package environment_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/environment"
)

func TestInitConfiguresNewEnvironmentFile(t *testing.T) {
	dir := installation(t)
	if _, err := environment.Init(dir, environment.InitOptions{Contexts: []string{"mysql", "redis"}}); err != nil {
		t.Fatal(err)
	}

	values, err := environment.ReadEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if got := values["XF_INSTANCE"]; got != "my-forum" {
		t.Errorf("XF_INSTANCE = %q, want the name derived from the directory", got)
	}
	if got := values["XF_CONTEXTS"]; got != "mysql:redis" {
		t.Errorf("XF_CONTEXTS = %q, want the requested contexts", got)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".env")), "XF_DEBUG=") {
		t.Error("generated environment lost the defaults")
	}
}

func TestInitLeavesExistingEnvironmentFileAlone(t *testing.T) {
	dir := installation(t)
	envPath := filepath.Join(dir, ".env")
	writeFile(t, envPath, "XF_INSTANCE=custom\nXF_CONTEXTS=mysql\n")

	if _, err := environment.Init(dir, environment.InitOptions{Contexts: []string{"redis"}}); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, envPath); got != "XF_INSTANCE=custom\nXF_CONTEXTS=mysql\n" {
		t.Fatalf("existing .env changed: %q", got)
	}
	defaults, err := environment.GetDockerFile(".env.default")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, envPath+".default"); got != string(defaults) {
		t.Fatalf(".env.default = %q, want the shipped default", got)
	}
}
