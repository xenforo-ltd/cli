package initflow

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/customerapi"
)

func tv(id int, str string, stable bool, ts string) customerapi.Version {
	tm, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		panic(fmt.Sprintf("parse time %q: %v", ts, err))
	}

	return customerapi.Version{
		VersionID:  id,
		VersionStr: str,
		Stable:     stable,
		ReleaseDate: customerapi.UnixTime{
			Time: tm,
		},
	}
}

func TestBuildVersionOptions(t *testing.T) {
	versions := []customerapi.Version{
		tv(3, "2.3.9", true, "2025-10-01T00:00:00Z"),
		tv(2, "2.3.8", true, "2025-09-01T00:00:00Z"),
		tv(1, "2.4.0 RC1", false, "2025-08-01T00:00:00Z"),
	}
	SortVersionsDesc(versions)

	opts := BuildVersionOptions(versions, 2)
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d", len(opts))
	}

	if opts[0].Label != "2.3.9 (latest)" {
		t.Fatalf("label = %q", opts[0].Label)
	}

	if opts[1].Label != "2.3.8" {
		t.Fatalf("label = %q", opts[1].Label)
	}
}

func TestResolveVersionInput(t *testing.T) {
	versions := []customerapi.Version{
		tv(2030900, "2.3.9", true, "2025-10-01T00:00:00Z"),
	}
	if got, ok := ResolveVersionInput("v2.3.9", versions); !ok || got.VersionID != 2030900 {
		t.Fatalf("string resolve failed: ok=%v got=%+v", ok, got)
	}

	if got, ok := ResolveVersionInput("2030900", versions); !ok || got.VersionID != 2030900 {
		t.Fatalf("id resolve failed: ok=%v got=%+v", ok, got)
	}
}

func TestEnvParsingAndMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.txt")

	content := "# comment\nXF_TITLE=Site\nXF_DEBUG=1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	fileVals, err := ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}

	flagVals, err := ParseEnvFlags([]string{"XF_TITLE=Override"})
	if err != nil {
		t.Fatal(err)
	}

	merged, src := MergeEnvMaps(map[string]string{"XF_INSTANCE": "demo"}, fileVals, flagVals)
	if merged["XF_TITLE"] != "Override" {
		t.Fatalf("XF_TITLE = %q", merged["XF_TITLE"])
	}

	if src["XF_TITLE"] != "--env" {
		t.Fatalf("source = %q", src["XF_TITLE"])
	}
}
