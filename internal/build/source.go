package build

import (
	"os"
	"path/filepath"
)

// fileSource opens paths relative to the build root.
type fileSource interface {
	Open(string) (*os.File, error)
}

// directoryOpener reuses one parent directory handle for sequential file reads.
// It owns only the cached handle, not the build root, and is not concurrent.
type directoryOpener struct {
	root   *os.Root
	dir    *os.Root
	parent string
}

// Open opens a native, root-relative path. Callers validate it with Localize.
func (o *directoryOpener) Open(name string) (*os.File, error) {
	parent := filepath.Dir(name)
	if parent != o.parent {
		o.Close()
		if parent != "." {
			var err error
			o.dir, err = o.root.OpenRoot(parent)
			if err != nil {
				return nil, err
			}
		}
		o.parent = parent
	}

	if o.dir != nil {
		if f, err := o.dir.Open(filepath.Base(name)); err == nil {
			return f, nil
		}
		// A link can leave this directory while remaining inside the build root.
	}
	return o.root.Open(name)
}

// Close releases the cached directory handle without closing the build root.
func (o *directoryOpener) Close() {
	if o.dir != nil {
		o.dir.Close()
	}
	o.dir, o.parent = nil, ""
}
