package cmd

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var commands = map[string]func([]string) error{
	"merge":   runMerge,
	"tags":    runTags,
	"update":  runUpdate,
	"version": runVersion,
}

func Execute() error {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcmd := os.Args[1]
	fn, ok := commands[subcmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", subcmd)
		printUsage()
		os.Exit(1)
	}

	return fn(os.Args[2:])
}

func runVersion(args []string) error {
	fmt.Println(Version)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: soundrig <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  merge    Merge MP3 files in a directory into a single file\n")
	fmt.Fprintf(os.Stderr, "  tags     View and edit audio file tags (TUI)\n")
	fmt.Fprintf(os.Stderr, "  update   Update soundrig to the latest version\n")
	fmt.Fprintf(os.Stderr, "  version  Display version information\n")
}
