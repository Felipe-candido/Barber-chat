package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnvironmentCredentialsAreNotTracked(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		t.Skip("source archive has no Git index to inspect")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal("cannot inspect tracked files", err)
	}
	for _, name := range strings.Split(string(out), "\x00") {
		base := filepath.Base(name)
		if (base == ".env" || strings.HasPrefix(base, ".env.")) && base != ".env.example" {
			t.Errorf("environment file is tracked: %s; keep credentials local and rotate exposed values", name)
		}
	}
}
