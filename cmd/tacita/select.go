package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/tuxerrante/tacita/internal/report"
)

// maxReportFile bounds a report read from disk at the frozen report byte
// budget, so an oversized file fails before it is decoded.
const maxReportFile = 64 << 20

// repositoryList collects repeated --operational-failure values.
type repositoryList []string

func (l *repositoryList) String() string {
	return strings.Join(*l, ",")
}

func (l *repositoryList) Set(value string) error {
	for _, repository := range report.CalibrationCorpus() {
		if value == repository.Name {
			*l = append(*l, value)
			return nil
		}
	}
	return fmt.Errorf("%q is not a calibration corpus repository", value)
}

func runSelect(ctx context.Context, args []string, stdout io.Writer) error {
	var flagOutput bytes.Buffer
	flags := newFlagSet("select", selectUsage, &flagOutput)
	var operationalFailures repositoryList
	flags.Var(&operationalFailures, "operational-failure",
		"corpus repository whose run the harness reports as failed or over its elapsed-time or memory budget")
	if handled, err := parseFlags(flags, &flagOutput, args, stdout); handled {
		return err
	}
	if flags.NArg() != 8 {
		return &usageError{detail: selectUsage, cause: errors.New("expected eight report paths")}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("starting selection: %w", err)
	}

	reports := make([][]byte, flags.NArg())
	for index, path := range flags.Args() {
		data, err := readReport(path)
		if err != nil {
			return err
		}
		reports[index] = data
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("reading reports: %w", err)
		}
	}

	runs := make([]report.RepositoryRun, len(report.CalibrationCorpus()))
	for index, repository := range report.CalibrationCorpus() {
		runs[index] = report.RepositoryRun{
			Report:          reports[2*index],
			Repeat:          reports[2*index+1],
			OperationalPass: !slices.Contains(operationalFailures, repository.Name),
		}
	}
	selection, err := report.Select(report.SelectionInput{
		Development: [2]report.RepositoryRun{runs[0], runs[1]},
		Controls:    [2]report.RepositoryRun{runs[2], runs[3]},
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		return fmt.Errorf("selecting the development configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("selecting the development configuration: %w", err)
	}

	if selection.Lock != nil {
		encoded, err := report.EncodeLock(*selection.Lock)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(encoded); err != nil {
			return fmt.Errorf("writing lock: %w", err)
		}
		return nil
	}

	if _, err := io.WriteString(stdout, selection.Summary()); err != nil {
		return fmt.Errorf("writing selection summary: %w", err)
	}

	return errNoSelection
}

// readReport reads one report file within the report byte budget.
func readReport(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // operator-named report file
	if err != nil {
		return nil, fmt.Errorf("opening report: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only file

	data, err := io.ReadAll(io.LimitReader(file, maxReportFile+1))
	if err != nil {
		return nil, fmt.Errorf("reading report %s: %w", path, err)
	}
	if len(data) > maxReportFile {
		return nil, fmt.Errorf("report %s exceeds %d bytes", path, maxReportFile)
	}

	return data, nil
}
