// Top-level tests that exercise every rule directory shipped with
// pasta:
//
//   - analyzers/* — the production analyzers (iferr, negcmp, etc.).
//   - testdata/* — extension/integration demos (e.g. external language
//     modules, custom file extensions).
//
// Each directory is treated as a rule package: its *.cue files are
// loaded as analyzers and/or language declarations, and run against
// its testdata/ tree.

package pasta

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imjasonh/pasta/internal/runner"
)

func TestAnalyzers(t *testing.T) {
	runRuleDirs(t, "analyzers")
}

func TestExtensionDemos(t *testing.T) {
	runRuleDirs(t, "testdata")
}

func runRuleDirs(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s/: %v", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			report, err := runner.TestDir(t.Context(), dir)
			if err != nil {
				t.Fatalf("%s: %v", dir, err)
			}
			if report.Failed() {
				for _, f := range report.Failures {
					t.Errorf("%s: %s", dir, f)
				}
			}
		})
	}
}
