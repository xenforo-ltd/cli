package xf

import (
	"os"
	"testing"
)

func TestReadMetadataMissingReturnsNil(t *testing.T) {
	meta, err := ReadMetadata(t.TempDir())
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}
	if meta != nil {
		t.Fatal("expected nil metadata")
	}
}

func TestWriteAndReadMetadata(t *testing.T) {
	dir := t.TempDir()
	in := &Metadata{
		LicenseKey:         "ABC123",
		InstanceName:       "demo",
		InstalledProducts:  []string{"xenforo", "xfmg"},
		InstalledVersion:   "2.3.8",
		InstalledVersionID: 2030871,
	}

	if err := WriteMetadata(dir, in); err != nil {
		t.Fatalf("WriteMetadata failed: %v", err)
	}

	out, err := ReadMetadata(dir)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}
	if out.LicenseKey != in.LicenseKey || out.InstanceName != in.InstanceName {
		t.Fatalf("metadata mismatch: %#v", out)
	}
	if out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		t.Fatal("timestamps should be set")
	}
}

func TestUpdateMetadataVersionCreatesFile(t *testing.T) {
	dir := t.TempDir()
	v := &Version{String: "2.3.9", ID: 2030971}

	if err := UpdateMetadataVersion(dir, v); err != nil {
		t.Fatalf("UpdateMetadataVersion failed: %v", err)
	}

	meta, err := ReadMetadata(dir)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}
	if meta.InstalledVersion != "2.3.9" || meta.InstalledVersionID != 2030971 {
		t.Fatalf("unexpected metadata: %#v", meta)
	}

	if _, err := os.Stat(MetadataPath(dir)); err != nil {
		t.Fatalf("expected metadata file: %v", err)
	}
}
