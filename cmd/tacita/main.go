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
)

const (
	rootUsage     = "usage: tacita <command> [flags]\n\ncommands:\n  backtest"
	backtestUsage = "usage: tacita backtest [flags] <repository>"
)

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

	if err := run(ctx, args, stdout); err != nil {
		if usageErr, ok := errors.AsType[*usageError](err); ok {
			if _, writeErr := fmt.Fprintln(stderr, usageErr); writeErr != nil {
				return 1
			}
			return 2
		}

		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}

	return 0
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		if _, err := fmt.Fprintln(stdout, rootUsage); err != nil {
			return fmt.Errorf("writing help: %w", err)
		}
		return nil
	}
	if len(args) == 0 || args[0] != "backtest" {
		return &usageError{
			detail: rootUsage,
			cause:  flag.ErrHelp,
		}
	}

	return runBacktest(ctx, args[1:], stdout)
}

func runBacktest(ctx context.Context, args []string, stdout io.Writer) error {
	// flag.FlagSet writes parse errors and help as a side effect.
	// Capture them so run can classify the result and the
	// CLI boundary remains the only place that prints an error.
	var flagOutput bytes.Buffer

	flags := flag.NewFlagSet("backtest", flag.ContinueOnError)
	flags.SetOutput(&flagOutput)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(&flagOutput, backtestUsage)
		flags.PrintDefaults()
	}

	revision := flags.String("revision", "HEAD", "commit to analyze")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, copyErr := io.Copy(stdout, &flagOutput); copyErr != nil {
				return fmt.Errorf("writing help: %w", copyErr)
			}
			return nil
		}

		return &usageError{
			detail: strings.TrimSpace(flagOutput.String()),
			cause:  err,
		}
	}

	if flags.NArg() != 1 {
		return &usageError{
			detail: backtestUsage,
			cause:  errors.New("expected exactly one repository path"),
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("starting backtest: %w", err)
	}

	if _, err := fmt.Fprintf(
		stdout,
		"repository=%s revision=%s\n",
		flags.Arg(0),
		*revision,
	); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}
