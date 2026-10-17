package gorged

import (
	"flag"
	"io"
)

// ServeFlagSet returns a fresh copy of the flag set Main parses, set to
// ContinueOnError with no output, so a caller outside this module can check
// an argument list against the real flags and read back the parsed values by
// name (fs.Lookup(name).Value). four-yay's deploy test uses it to prove that
// what its demo deploy script passes is accepted by the server it starts.
func ServeFlagSet() *flag.FlagSet {
	fs, _ := serveFlags()
	fs.Init(fs.Name(), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
