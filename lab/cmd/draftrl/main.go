// Command draftrl is the draft-RL program's driver (spec
// docs/design/2026-10-14-draft-rl-design.md). Its
// subcommands are gate (DR-4, §3.0): homogeneous or mixed pods drafted
// from fixed seeds, each seat's built deck measured against the
// fdn-limited gauntlet under the scripted bot or the closed-list az pilot;
// and rollout (DR-7, §3.3 with the §11 reward ruling): league pods scored
// by the round robin within the pod, writing the training records; and bc
// (DR-8, §3.2): draft-only pods recording the teacher's scores and the
// seats' colour-pair labels for behaviour cloning.
//
// draftrl is a bot-only sim tool: the az pilot is an Env seat, admitted
// only through draftlab's AllowDecklistBots opt-in, which no event or
// server path sets.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "draftrl:", err)
		os.Exit(1)
	}
}

// run dispatches the subcommand; it is main's testable body.
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: draftrl gate|rollout|bc [flags] (calibrate is not implemented yet)")
	}
	switch args[0] {
	case "gate":
		_, err := gate(args[1:], out)
		return err
	case "rollout":
		_, err := rollout(args[1:], out)
		return err
	case "bc":
		_, err := bc(args[1:], out)
		return err
	case "belief":
		return belief(args[1:], out)
	case "beliefplay":
		return beliefPlay(args[1:], out)
	default:
		return fmt.Errorf("unknown subcommand %q (want gate, rollout, bc, belief or beliefplay)", args[0])
	}
}
