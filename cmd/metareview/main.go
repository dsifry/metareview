package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/artifactreview"
	"github.com/dsifry/metareview/internal/contextpack"
	"github.com/dsifry/metareview/internal/epicready"
	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/learning"
	"github.com/dsifry/metareview/internal/prready"
	"github.com/dsifry/metareview/internal/repo"
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
  metareview context build <path>
  metareview context diff [--base <ref>]
  metareview evidence run -- <command> [args...]
  metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]
  metareview review artifact <path> [--previous-run <run-id>] [--scaffold-only]
  metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
  metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
  metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree]
  metareview learn --post-merge <pr-number> [--base <ref>] [--github-pr <number>] [--session-root <path>]

Commands:
  setup --check              Detect repository mode and prerequisites without writing files
  setup --bootstrap-prereqs  Print or execute prerequisite bootstrap actions
  status                     Print repository review capability status
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
