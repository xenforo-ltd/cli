package setup

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPendingAddOnComposerProjectsHandlesGlobCharactersInRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project[1]")
	addOn := filepath.Join(root, "src", "addons", "Vendor", "Gallery")
	for path, content := range map[string]string{
		filepath.Join(addOn, "composer.json"): `{}`,
		filepath.Join(addOn, "addon.json"):    `{"composer_autoload":"_vendor"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"src/addons/Vendor/Gallery"}
	if got := pendingAddOnComposerProjects(root); !slices.Equal(got, want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}
