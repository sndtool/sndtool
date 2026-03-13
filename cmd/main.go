package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	sndtool "github.com/cbrake/sndtool"
)

var version = "dev"

var commands = map[string]func([]string) error{
	"merge":   runMerge,
	"tags":    runTags,
	"update":  runUpdate,
	"version": runVersion,
}

func main() {
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

	if err := fn(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sndtool merge <directory>\n\n")
		fmt.Fprintf(os.Stderr, "Merge all MP3 files in <directory> into a single output file.\n")
		fmt.Fprintf(os.Stderr, "Output filename is derived from the directory name.\n")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	inputDir := strings.TrimRight(fs.Arg(0), "/")
	outputFile := strings.ToLower(inputDir) + ".mp3"

	if err := sndtool.MergeMp3Files(inputDir, outputFile); err != nil {
		return err
	}

	return sndtool.AddTags(outputFile)
}

func runVersion(args []string) error {
	fmt.Println(version)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: sndtool <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  merge    Merge MP3 files in a directory into a single file\n")
	fmt.Fprintf(os.Stderr, "  tags     View and edit audio file tags (TUI)\n")
	fmt.Fprintf(os.Stderr, "  update   Update sndtool to the latest version\n")
	fmt.Fprintf(os.Stderr, "  version  Display version information\n")
}
