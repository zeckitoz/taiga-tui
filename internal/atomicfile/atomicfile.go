// Package atomicfile writes files so that readers see either the old or the
// new content, never a partial write.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes data to name through a temporary file in the same directory
// and a rename. The file gets mode 0600. A missing directory is created with
// mode 0700; an existing directory keeps its mode.
func Write(name string, data []byte) (err error) {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+"-*.tmp") // mode 0600
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
