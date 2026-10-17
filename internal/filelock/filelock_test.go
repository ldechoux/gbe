package filelock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.lock")
	f, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if _, ok, err := TryLock(path); ok || err != nil {
		t.Errorf("locked twice: %v %v", ok, err)
	}
	f.Close()
	g, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("lock once released: %v %v", ok, err)
	}
	g.Close()
	if _, _, err := TryLock(filepath.Join(path, "in-a-file")); err == nil {
		t.Error("no error for a path that cannot be created")
	}
}

// Another process cannot take the lock: the test runs itself as that
// process.
func TestTryLockOtherProcess(t *testing.T) {
	if path := os.Getenv("FILELOCK_CHILD"); path != "" {
		if _, ok, err := TryLock(path); ok || err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "game.lock")
	f, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	defer f.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestTryLockOtherProcess$")
	cmd.Env = append(os.Environ(), "FILELOCK_CHILD="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("the other process took the lock: %v %s", err, out)
	}
}
