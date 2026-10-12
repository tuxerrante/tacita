package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/tuxerrante/tacita/internal/report"
)

const (
	rootUsage      = "usage: tacita <command> [flags]\n\ncommands:\n  calibrate\n  select"
	calibrateUsage = "usage: tacita calibrate --revision <revision> <repository>"
	selectUsage    = "usage: tacita select [--operational-failure <repository>]... " +
		"<golang/tools report> <golang/tools repeat> <kubernetes report> <kubernetes repeat> " +
		"<go-starter report> <proficiency report>"

	// exitNoSelection reports a completed selection that wrote no lock because
	// the development lane failed.
	exitNoSelection = 3
)

// errNoSelection identifies a selection that wrote no lock. Its summary has
// already been written to stdout.
var errNoSelection = errors.New("no selectable configuration: the development lane fails")

type usageError struct {
	detail string
	cause  error
}

func (e *usageError) Error() string {
	return e.detail
}

func (e *usageError) Unwrap() error {
	return e.cause
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

func realMain(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := run(ctx, args, stdout)
	if err == nil {
		return 0
	}

	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		return 1
	}

	return exitCode(err)
}

// exitCode maps a command failure to its process exit code: 2 for usage, 3
// for a selection that wrote no lock, and 1 for every operational failure.
func exitCode(err error) int {
	if _, ok := errors.AsType[*usageError](err); ok {
		return 2
	}
	if errors.Is(err, errNoSelection) {
		return exitNoSelection
	}
	return 1
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		if _, err := fmt.Fprintln(stdout, rootUsage); err != nil {
			return fmt.Errorf("writing help: %w", err)
		}
		return nil
	}
	if len(args) == 0 {
		return &usageError{detail: rootUsage, cause: flag.ErrHelp}
	}

	switch args[0] {
	case "calibrate":
		return runCalibrate(ctx, args[1:], stdout)
	case "select":
		return runSelect(ctx, args[1:], stdout)
	default:
		return &usageError{detail: rootUsage, cause: flag.ErrHelp}
	}
}

// newFlagSet captures help and parse output so the CLI boundary remains the
// only place that prints an error.
func newFlagSet(name string, usage string, output *bytes.Buffer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(output, usage)
		flags.PrintDefaults()
	}
	return flags
}

// parseFlags parses args, copying help to stdout. It reports handled when
// help was printed and the command should stop without error.
func parseFlags(flags *flag.FlagSet, output *bytes.Buffer, args []string, stdout io.Writer) (bool, error) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, copyErr := io.Copy(stdout, output); copyErr != nil {
				return true, fmt.Errorf("writing help: %w", copyErr)
			}
			return true, nil
		}
		return true, &usageError{detail: strings.TrimSpace(output.String()), cause: err}
	}
	return false, nil
}

func runCalibrate(ctx context.Context, args []string, stdout io.Writer) error {
	var flagOutput bytes.Buffer
	flags := newFlagSet("calibrate", calibrateUsage, &flagOutput)
	revision := flags.String("revision", "", "frozen corpus commit to calibrate")
	if handled, err := parseFlags(flags, &flagOutput, args, stdout); handled {
		return err
	}
	if flags.NArg() != 1 || *revision == "" {
		return &usageError{
			detail: calibrateUsage,
			cause:  errors.New("expected --revision and exactly one repository path"),
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("starting calibration: %w", err)
	}

	encoded, err := calibrate(ctx, flags.Arg(0), *revision, report.CorpusRepositoryFor)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(encoded); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	return nil
}
