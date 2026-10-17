//go:build !unix && !windows

package filelock

import "os"

// lock always succeeds where the system has no file locks (WebAssembly):
// one process runs there.
func lock(*os.File) (bool, error) { return true, nil }
