package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/adams-shaun/gorge/internal/testbudget"
)

// rssDirEnv names the directory runExec records peak RSS in.
const rssDirEnv = testbudget.RSSDirEnv

// testlogFlag is the test binary flag `go test` uses to learn which files and
// environment variables a test read, which is what makes a result cacheable.
const testlogFlag = "-test.testlogfile"

// runExec runs a test binary exactly as `go test` would and records its peak
// RSS. Its exit code is the binary's, so `go test -exec` sees no difference.
//
// `go test -exec X` stops passing -test.testlogfile to the binary (go1.26.3
// cmd/go/internal/test/test.go:1581-1585: the flag is added only when no exec
// command is set), and runCache.saveOutput then finds no testlog.txt in the
// action's object directory and never caches the result. The cache key itself
// ignores the exec command, so runExec restores caching by passing the flag
// itself, pointing at testlog.txt next to the test binary (the action's
// Objdir, where saveOutput reads it). If a future Go moves the binary or
// renames the flag, the failure mode is cache MISSES only, never stale
// results: saveOutput validates the testlog and caches nothing without it.
// TestExecKeepsGoTestCacheUsable fails loudly when that happens.
func runExec(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "testbudget exec: no test binary")
		return 2
	}
	cmd := exec.Command(args[0], withTestlog(args)[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
		if code < 0 {
			code = 1
		}
	case err != nil:
		fmt.Fprintf(os.Stderr, "testbudget exec: %v\n", err)
		return 1
	}
	if dir := os.Getenv(rssDirEnv); dir != "" && cmd.ProcessState != nil {
		if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			if werr := testbudget.RecordRSS(dir, int64(ru.Maxrss)); werr != nil {
				fmt.Fprintf(os.Stderr, "testbudget exec: %v\n", werr)
			}
		}
	}
	return code
}

// withTestlog returns args with -test.testlogfile added after the binary,
// unless the caller already passed one. Only a `go test` binary (named
// *.test) gets it: another program would reject the unknown flag.
func withTestlog(args []string) []string {
	if !strings.HasSuffix(args[0], ".test") {
		return args
	}
	for _, a := range args[1:] {
		if a == testlogFlag || a == "-"+testlogFlag || strings.HasPrefix(a, testlogFlag+"=") || strings.HasPrefix(a, "-"+testlogFlag+"=") {
			return args
		}
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, args[0], testlogFlag+"="+filepath.Join(filepath.Dir(args[0]), "testlog.txt"))
	return append(out, args[1:]...)
}

func readRSSDir(dir string) (map[string]int64, error) {
	return testbudget.ReadRSSDir(dir)
}
