// Package filelock keeps a file for one process at a time, with the locks
// of the system (flock, LockFileEx): the system releases them when the
// process ends, even when it crashes.
package filelock

import "os"

// TryLock opens the file at path, creating it, and locks it, unless
// another process holds it: ok is false then. Close releases the lock.
// Within one process, the same file cannot be locked twice either.
func TryLock(path string) (f *os.File, ok bool, err error) {
	f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	if ok, err = lock(f); !ok || err != nil {
		f.Close()
		return nil, false, err
	}
	return f, true, nil
}
