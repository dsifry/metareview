package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/artifactreview"
	"github.com/dsifry/metareview/internal/contextpack"
	"github.com/dsifry/metareview/internal/epicready"
	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/learning"
	"github.com/dsifry/metareview/internal/prready"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/setup"
	"github.com/dsifry/metareview/internal/taskdone"
	"github.com/dsifry/metareview/internal/version"
)

func printHelp() {
	fmt.Printf(`metareview %s

Usage:
  metareview setup --check
  metareview setup --bootstrap-prereqs --dry-run
  metareview status
  metareview override request <finding-id> --reason "<text>" [--by <who>] [--escalation "<text>"]
  metareview override grant <finding-id> --reason "<text>" [--by <who>]
  metareview override list [--pending]
  metareview context build <path>
  metareview context diff [--base <ref>]
  metareview evidence run -- <command> [args...]
  metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]
  metareview review artifact <path> [--previous-run <run-id>] [--scaffold-only]
  metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--shard-result <path>]... [--cross-shard-result <path>]
  metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
  metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree] [--shard-result <path>]... [--cross-shard-result <path>]
  metareview learn --post-merge <pr-number> [--base <ref>] [--github-pr <number>] [--session-root <path>]

Commands:
  setup --check              Detect repository mode and prerequisites without writing files
  setup --bootstrap-prereqs  Print or execute prerequisite bootstrap actions
  status                     Print repository review capability status
  override request           Record an out-of-workflow escalation against a finding (still blocks)
  override grant             Acknowledge a process exception from outside the workflow (stops blocking)
  override list              List process exceptions; --pending exits 1 while any are unacknowledged
  context build <path>       Build a Markdown context pack for an artifact
  context diff               Print git diff context as JSON
  evidence run               Run a command and print a structured JSON receipt
  evidence import            Import external validation receipts
  review artifact <path>     Create an incomplete artifact review scaffold
  review task-done <target>  Run task-done code review
  review epic-ready <target> Run epic-ready integration review
  review pr-ready            Run PR-ready branch review
  learn --post-merge         Curate post-merge repository learning
`, version.Version)
}

func printLearnHelp() {
	fmt.Printf(`metareview learn

Usage:
  metareview learn --post-merge <pr-number> [--base <ref>] [--github-pr <number>] [--session-root <path>]

Commands:
  --post-merge <pr-number>  Curate local learning from a completed PR/review/session context
`)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Println(version.Version)
		return
	}

	if len(args) >= 1 && args[0] == "setup" {
		handleSetup(args[1:])
		return
	}

	if len(args) == 1 && args[0] == "status" {
		report := repo.Detect(mustCwd())
		fmt.Printf("metareview %s\n", version.Version)
		fmt.Printf("mode: %s\n", report.Mode)
		fmt.Printf("git: %s\n", present(report.Capabilities.Git))
		fmt.Printf("beads: %s\n", present(report.Capabilities.Beads))
		fmt.Printf("metaswarm: %s\n", present(report.Capabilities.Metaswarm))
		return
	}

	if args[0] == "override" {
		handleOverride(args[1:])
		return
	}

	if len(args) == 3 && args[0] == "context" && args[1] == "build" {
		result, err := contextpack.Build(mustCwd(), args[2], time.Now())
		exitOnErr(err)
		fmt.Println(result.ContextRel)
		return
	}

	if len(args) >= 2 && args[0] == "context" && args[1] == "diff" {
		base := ""
		for i := 2; i < len(args); i++ {
			if args[i] == "--base" {
				base = flagValue(args, i, "--base")
				i++
				continue
			}
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
			os.Exit(2)
		}
		result, err := gitcontext.Collect(mustCwd(), base)
		exitOnErr(err)
		bytes, err := json.MarshalIndent(result, "", "  ")
		exitOnErr(err)
		fmt.Println(string(bytes))
		return
	}

	if len(args) >= 1 && args[0] == "evidence" {
		handleEvidence(args[1:])
		return
	}

	if len(args) >= 3 && args[0] == "review" && args[1] == "artifact" {
		previousRun := ""
		scaffoldOnly := false
		for i := 3; i < len(args); i++ {
			if args[i] == "--previous-run" {
				previousRun = flagValue(args, i, "--previous-run")
				i++
				continue
			}
			if args[i] == "--scaffold-only" {
				scaffoldOnly = true
				continue
			}
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
			os.Exit(2)
		}
		result, err := artifactreview.Create(mustCwd(), args[2], previousRun, time.Now())
		exitOnErr(err)
		fmt.Println(result.ReviewRel)
		if !scaffoldOnly {
			fmt.Fprintln(os.Stderr, "Artifact review scaffold created but not completed.")
			fmt.Fprintln(os.Stderr, "Complete all required reviewer rows and update the verdict to PASS or PASS_ADVISORY with zero blockers, or re-run with --scaffold-only when only scaffold creation is intended.")
			os.Exit(1)
		}
		return
	}

	if len(args) >= 3 && args[0] == "review" && args[1] == "task-done" {
		options := taskdone.Options{}
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--base":
				options.Base = flagValue(args, i, "--base")
				i++
			case "--previous-run":
				options.PreviousRunID = flagValue(args, i, "--previous-run")
				i++
			case "--max-attempts":
				options.MaxAttempts = mustPositiveInt(flagValue(args, i, "--max-attempts"), "--max-attempts")
				i++
			case "--evidence":
				options.EvidencePath = flagValue(args, i, "--evidence")
				i++
			case "--shard-result":
				options.ShardResultPaths = append(options.ShardResultPaths, mustResultFile(flagValue(args, i, "--shard-result")))
				i++
			case "--cross-shard-result":
				options.CrossShardResultPaths = appendCrossShardResult(options.CrossShardResultPaths, flagValue(args, i, "--cross-shard-result"))
				i++
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		result, err := taskdone.Create(mustCwd(), args[2], options)
		exitOnErr(err)
		fmt.Println(result.ReviewRel)
		if result.Blocking {
			os.Exit(1)
		}
		return
	}

	if len(args) >= 3 && args[0] == "review" && args[1] == "epic-ready" {
		options := epicready.Options{}
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--base":
				options.Base = flagValue(args, i, "--base")
				i++
			case "--previous-run":
				options.PreviousRunID = flagValue(args, i, "--previous-run")
				i++
			case "--max-attempts":
				options.MaxAttempts = mustPositiveInt(flagValue(args, i, "--max-attempts"), "--max-attempts")
				i++
			case "--evidence":
				options.EvidencePath = flagValue(args, i, "--evidence")
				i++
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		result, err := epicready.Create(mustCwd(), args[2], options)
		exitOnErr(err)
		fmt.Println(result.ReviewRel)
		if result.Blocking {
			os.Exit(1)
		}
		return
	}

	if len(args) >= 2 && args[0] == "review" && args[1] == "pr-ready" {
		options := prready.Options{}
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--base":
				options.Base = flagValue(args, i, "--base")
				i++
			case "--previous-run":
				options.PreviousRunID = flagValue(args, i, "--previous-run")
				i++
			case "--max-attempts":
				options.MaxAttempts = mustPositiveInt(flagValue(args, i, "--max-attempts"), "--max-attempts")
				i++
			case "--evidence":
				options.EvidencePath = flagValue(args, i, "--evidence")
				i++
			case "--github-pr":
				options.GitHubPR = flagValue(args, i, "--github-pr")
				i++
			case "--shard-result":
				options.ShardResultPaths = append(options.ShardResultPaths, mustResultFile(flagValue(args, i, "--shard-result")))
				i++
			case "--cross-shard-result":
				options.CrossShardResultPaths = appendCrossShardResult(options.CrossShardResultPaths, flagValue(args, i, "--cross-shard-result"))
				i++
			case "--include-working-tree":
				options.IncludeWorkingTree = true
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		result, err := prready.Create(mustCwd(), options)
		exitOnErr(err)
		fmt.Println(result.ReviewRel)
		if result.Blocking {
			os.Exit(1)
		}
		return
	}

	if len(args) >= 1 && args[0] == "learn" {
		if len(args) == 1 || args[1] == "--help" || args[1] == "-h" {
			printLearnHelp()
			return
		}
		options := learning.ReviewOptions{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--post-merge":
				options.PostMergePR = flagValue(args, i, "--post-merge")
				i++
			case "--base":
				options.Base = flagValue(args, i, "--base")
				i++
			case "--github-pr":
				options.GitHubPR = flagValue(args, i, "--github-pr")
				i++
			case "--session-root":
				options.SessionRoot = flagValue(args, i, "--session-root")
				i++
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		if options.PostMergePR == "" {
			fmt.Fprintln(os.Stderr, "Missing value for --post-merge")
			os.Exit(2)
		}
		result, err := learning.RunPostMerge(mustCwd(), options)
		exitOnErr(err)
		fmt.Println(result.AcceptedRel)
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
	printHelp()
	os.Exit(2)
}

func handleEvidence(args []string) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: metareview evidence run -- <command> [args...]")
		fmt.Println("       metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]")
		return
	}
	switch args[0] {
	case "run":
		separator := -1
		for i := 1; i < len(args); i++ {
			if args[i] == "--" {
				separator = i
				break
			}
		}
		if separator == -1 || separator+1 >= len(args) {
			fmt.Fprintln(os.Stderr, "Usage: metareview evidence run -- <command> [args...]")
			os.Exit(2)
		}
		receipt, runErr := evidence.Run(context.Background(), args[separator+1:], evidence.RunOptions{})
		if runErr != nil && receipt.SchemaVersion == 0 {
			exitOnErr(runErr)
		}
		bytes, err := json.Marshal(receipt)
		exitOnErr(err)
		fmt.Println(string(bytes))
		if runErr != nil {
			fmt.Fprintln(os.Stderr, runErr)
			if receipt.ExitCode != 0 {
				os.Exit(receipt.ExitCode)
			}
			os.Exit(1)
		}
		if receipt.ExitCode != 0 {
			os.Exit(receipt.ExitCode)
		}
	case "import":
		pr := ""
		repository := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--github-checks":
				pr = flagValue(args, i, "--github-checks")
				i++
			case "--repo":
				repository = flagValue(args, i, "--repo")
				i++
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		if pr == "" {
			fmt.Fprintln(os.Stderr, "Missing value for --github-checks")
			os.Exit(2)
		}
		bundle, err := evidence.ImportGitHubChecks(context.Background(), pr, evidence.GitHubCheckOptions{Repo: repository})
		exitOnErr(err)
		bytes, err := bundle.JSONL()
		exitOnErr(err)
		fmt.Print(string(bytes))
		if bundleExitCode(bundle) != 0 {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown evidence command: %s\n", args[0])
		os.Exit(2)
	}
}

func bundleExitCode(bundle evidence.Bundle) int {
	for _, receipt := range bundle.Receipts {
		if receipt.ExitCode != 0 {
			return 1
		}
	}
	return 0
}

func mustCwd() string {
	cwd, err := os.Getwd()
	exitOnErr(err)
	return cwd
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func handleSetup(args []string) {
	if len(args) == 1 && args[0] == "--check" {
		report := setup.Check(mustCwd(), setup.Options{ExecutablePath: executablePath()})
		bytes, err := json.MarshalIndent(report, "", "  ")
		exitOnErr(err)
		fmt.Println(string(bytes))
		return
	}

	bootstrap := false
	options := setup.BootstrapOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--bootstrap-prereqs":
			bootstrap = true
		case "--dry-run":
			options.DryRun = true
		case "--confirm-bootstrap-prereqs":
			options.Confirm = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
			os.Exit(2)
		}
	}
	if !bootstrap {
		fmt.Fprintln(os.Stderr, "Usage: metareview setup --check OR metareview setup --bootstrap-prereqs [--dry-run] [--confirm-bootstrap-prereqs]")
		os.Exit(2)
	}
	plan, err := setup.BootstrapPrereqs(mustCwd(), options)
	if errors.Is(err, setup.ErrConfirmationRequired) {
		fmt.Fprintln(os.Stderr, "setup --bootstrap-prereqs requires --confirm-bootstrap-prereqs without --dry-run")
		os.Exit(2)
	}
	exitOnErr(err)
	fmt.Printf("metareview prerequisite bootstrap plan\n")
	fmt.Printf("dry-run: %t\n", plan.DryRun)
	for _, action := range plan.Actions {
		fmt.Printf("- %s\n", action)
	}
}

func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func flagValue(args []string, index int, name string) string {
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
		fmt.Fprintf(os.Stderr, "Missing value for %s\n", name)
		os.Exit(2)
	}
	return args[index+1]
}

func mustPositiveInt(value, name string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		fmt.Fprintf(os.Stderr, "%s must be an integer greater than 0\n", name)
		os.Exit(2)
	}
	return parsed
}

func present(value bool) string {
	if value {
		return "present"
	}
	return "missing"
}

// handleOverride implements the process-exception commands. An override records
// that the workflow was deliberately stepped outside of: requesting is available
// to whoever is driving the run, granting is the acknowledgement from outside it,
// and CI stays red while any request is unacknowledged.
func handleOverride(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: metareview override request|grant|list")
		os.Exit(2)
	}
	root := mustCwd()
	switch args[0] {
	case "list":
		pendingOnly := false
		for _, arg := range args[1:] {
			// A silently ignored option (a misspelled --pending, say) would make a
			// CI check look green while overrides were still unacknowledged.
			if arg != "--pending" {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
				os.Exit(2)
			}
			pendingOnly = true
		}
		records, err := findings.ListOverrides(root)
		exitOnErr(err)
		pending := 0
		for _, record := range records {
			if record.Status == findings.StatusOverridePending {
				pending++
			}
			if pendingOnly && record.Status != findings.StatusOverridePending {
				continue
			}
			printOverride(record)
		}
		if len(records) == 0 {
			fmt.Println("no process overrides recorded")
		}
		if pendingOnly && pending > 0 {
			fmt.Fprintf(os.Stderr, "%d override(s) awaiting acknowledgement\n", pending)
			os.Exit(1)
		}
	case "request", "grant":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: metareview override %s <finding-id> --reason \"<text>\"\n", args[0])
			os.Exit(2)
		}
		id := args[1]
		reason, by, escalation := "", "", ""
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--reason":
				if i+1 < len(args) {
					reason = args[i+1]
					i++
				}
			case "--by":
				if i+1 < len(args) {
					by = args[i+1]
					i++
				}
			case "--escalation":
				if i+1 < len(args) {
					escalation = args[i+1]
					i++
				}
			default:
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				os.Exit(2)
			}
		}
		if by == "" {
			by = defaultActor()
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if args[0] == "request" {
			exitOnErr(findings.RequestOverride(root, id, findings.OverrideRequest{
				By: by, Reason: reason, Escalation: escalation, Now: now,
			}))
			fmt.Printf("%s: override requested by %s (still blocking until granted)\n", id, by)
			return
		}
		exitOnErr(findings.GrantOverride(root, id, findings.OverrideGrant{By: by, Reason: reason, Now: now}))
		fmt.Printf("%s: override granted by %s\n", id, by)
	default:
		fmt.Fprintln(os.Stderr, "Usage: metareview override request|grant|list")
		os.Exit(2)
	}
}

func printOverride(record findings.Record) {
	switch record.Status {
	case findings.StatusOverridePending:
		fmt.Printf("%s  pending  %s\n    requested by %s at %s: %s\n",
			record.ID, record.Title, record.OverrideRequestedBy, record.OverrideRequestedAt, record.OverrideRequestReason)
	default:
		fmt.Printf("%s  granted  %s\n    granted by %s at %s: %s\n",
			record.ID, record.Title, record.OverrideGrantedBy, record.OverrideGrantedAt, record.OverrideGrantReason)
	}
}

// defaultActor identifies who is acting, from git config.
func defaultActor() string {
	cmd := exec.Command("git", "config", "user.email")
	cmd.Dir = mustCwd()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// appendCrossShardResult keeps --cross-shard-result single valued. A plan has
// exactly one cross-shard result, and discovery would otherwise let the last
// matching file win silently, making the outcome depend on flag order.
func appendCrossShardResult(existing []string, path string) []string {
	if len(existing) > 0 {
		fmt.Fprintln(os.Stderr, "Repeated --cross-shard-result: a plan has one cross-shard result")
		os.Exit(2)
	}
	return append(existing, mustResultFile(path))
}

// mustResultFile validates an explicit shard result before the review package
// runs, so a bad path exits 2 with nothing written.
func mustResultFile(path string) string {
	reject := func(reason string) {
		fmt.Fprintf(os.Stderr, "Invalid result file %s: %s\n", path, reason)
		os.Exit(2)
	}
	info, err := os.Stat(path)
	if err != nil {
		reject("does not exist")
	} else if !info.Mode().IsRegular() {
		reject("is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		reject("cannot be read")
	}
	if _, _, err := reviewmanifest.ParseResult(data); err != nil {
		reject("is not a metareview review result")
	}
	return path
}
