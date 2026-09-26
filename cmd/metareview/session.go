package main

import (
	"fmt"
	"time"

	"github.com/dsifry/metareview/internal/session"
)

const sessionUsage = "Usage: metareview session bind <session-id> <worktree-path> | resolve [<session-id>] | unbind <session-id>"

func printSessionHelp() {
	_, _ = fmt.Fprint(stdout, `metareview session

Usage:
  metareview session bind <session-id> <worktree-path>
  metareview session resolve [<session-id>]
  metareview session unbind <session-id>

A host's Stop hook only knows the checkout the session was launched in. When the work happens in
another worktree of the same repository, bind the session to it so the hook evaluates that
worktree instead. The hook's block reason quotes the exact bind command, session id included.
A binding selects a checkout; it never exempts one — the bound worktree's own pending reviews
still block.

Commands:
  bind     Record that <session-id>'s work is in the worktree containing <worktree-path>
  resolve  Print the checkout root to evaluate for <session-id>, then "bound" on a second line
           when a binding chose it (exit 0; warnings on stderr)
  unbind   Remove <session-id>'s binding
`)
}

func handleSession(args []string) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printSessionHelp()
		return
	}
	switch {
	case len(args) == 3 && args[0] == "bind":
		b, err := session.Bind(workdir, args[1], args[2], defaultActor(), time.Now())
		exitOnErr(err)
		_, _ = fmt.Fprintf(stdout, "metareview: session %s bound to %s (branch %s)\n", b.SessionID, b.Worktree, b.Branch)
	case (len(args) == 1 || len(args) == 2) && args[0] == "resolve":
		id := ""
		if len(args) == 2 {
			id = args[1]
		}
		r := session.Resolve(workdir, id)
		if r.Warning != "" {
			_, _ = fmt.Fprintln(stderr, "metareview: "+r.Warning)
		}
		_, _ = fmt.Fprintln(stdout, r.Dir)
		if r.Bound {
			// A second line, so a hook reads "bound" from the answer instead of inferring it from
			// a path comparison that a nested repository marker can fool.
			_, _ = fmt.Fprintln(stdout, "bound")
		}
	case len(args) == 2 && args[0] == "unbind":
		removed, err := session.Unbind(workdir, args[1])
		exitOnErr(err)
		if removed {
			_, _ = fmt.Fprintf(stdout, "metareview: session %s unbound\n", args[1])
		} else {
			_, _ = fmt.Fprintf(stdout, "metareview: session %s had no binding\n", args[1])
		}
	default:
		_, _ = fmt.Fprintln(stderr, sessionUsage)
		exit(2)
	}
}
