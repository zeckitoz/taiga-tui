package taiga_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/taiga/taigarec"
)

func TestFixturesHoldNoEmailAddressesOrJWTs(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if leaks := taigarec.Leaks(data); len(leaks) > 0 {
			t.Errorf("%s leaks %q", name, leaks)
		}
	}
}
