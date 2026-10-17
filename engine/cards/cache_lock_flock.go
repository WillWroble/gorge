//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package cards

import (
	"os"
	"syscall"
)

// lockCacheAttempts bounds lockCache's retry when the lock file it locked was
// unlinked underneath it (pruneCacheLocks). One retry is the realistic case;
// the bound only stops a pathological prune loop from spinning forever.
const lockCacheAttempts = 8

// lockCache takes the exclusive single-flight lock for cache, blocking until
// any peer compiling the same cache releases it, and returns the release
// function. It never fails: when the lock cannot be taken it returns a no-op
// release and the caller proceeds unlocked (see cache_lock.go).
//
// flock is held by the open file description, so the kernel drops it when
// the holder exits or crashes; a dead compiler can never wedge its waiters.
func lockCache(cache string) (unlock func()) {
	path := cacheLockPath(cache)
	for range lockCacheAttempts {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return func() {}
		}
		if err := flockRetry(f, syscall.LOCK_EX); err != nil {
			f.Close()
			return func() {}
		}
		// A pruner may have unlinked the file between our open and our lock;
		// a peer opening the path now gets a NEW file and would not exclude
		// us. Only a lock on the file still at the path counts.
		held, err1 := f.Stat()
		cur, err2 := os.Stat(path)
		if err1 == nil && err2 == nil && os.SameFile(held, cur) {
			return func() {
				_ = flockRetry(f, syscall.LOCK_UN)
				f.Close()
			}
		}
		f.Close()
	}
	return func() {}
}

// tryRemoveCacheLock unlinks the lock file at path only when nobody holds it
// (a non-blocking exclusive lock succeeds); it unlinks while holding the lock,
// so a waiter on the old file sees the identity mismatch in lockCache and
// retries on a fresh one.
func tryRemoveCacheLock(path string) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer f.Close()
	if flockRetry(f, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return
	}
	_ = os.Remove(path)
	_ = flockRetry(f, syscall.LOCK_UN)
}

// flockRetry is syscall.Flock retried across EINTR, which a blocking flock
// returns when a signal (the Go runtime's preemption signal included) lands
// while it waits.
func flockRetry(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			return err
		}
	}
}
