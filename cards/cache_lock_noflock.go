//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package cards

// lockCache is the single-flight lock on a platform without flock: a no-op,
// so every opener compiles independently as it did before the lock existed.
// Save's temp+rename keeps the result atomic either way (see cache_lock.go).
func lockCache(cache string) (unlock func()) { return func() {} }

// tryRemoveCacheLock never removes anything here: no lock file is created.
func tryRemoveCacheLock(path string) {}
