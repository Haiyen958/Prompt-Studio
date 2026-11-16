package handlers_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Architecture guard (audit P1-c): HTTP handlers talk to studio.Service only. Calling the
// repository directly would put business rules back into the HTTP layer.
func TestHandlersDoNotCallRepositoryDirectly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), ".Repo()") {
			t.Errorf("%s calls svc.Repo(); add a studio.Service method instead", f)
		}
	}
}
