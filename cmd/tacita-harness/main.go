// Command tacita-harness runs the frozen calibration in the reference
// environment and records the experiment evidence: it runs `tacita calibrate`
// as a child process twice for every development and control repository,
// measures
// each run's elapsed time and peak resident memory from outside Tacita, writes
// the operational sidecars, applies the development selector, and writes the
// evidence index.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/tuxerrante/tacita/internal/report"
)

const (
	usage = "usage: tacita-harness --tacita <binary> --runner <description> --out <directory> " +
		"--golang-tools <repository> --kubernetes <repository> --go-starter <repository> --proficiency <repository>"

	// killMargin lets Tacita report its own elapsed-budget failure before the
	// harness kills it.
	killMargin = 5 * time.Minute
	// maxStdout bounds a run's report at the report byte budget.
	maxStdout = 64 << 20
	// maxStderr bounds a run's diagnostic output.
	maxStderr = 1 << 20
)

// errUnsupportedPlatform identifies a host whose resource accounting the
// harness cannot read in the frozen units.
var errUnsupportedPlatform = errors.New("the reference harness runs only on linux/amd64")

// repositoryRun is one planned calibration run.
type repositoryRun struct {
	repository report.CorpusRepository
	path       string
	run        int
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

// realMain prints the run summary and returns the exit code: 0 when the
// evidence was recorded, whatever the experiment outcome, and 1 otherwise.
func realMain(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	summary, err := run(ctx, args)
	if summary != "" {
		if _, writeErr := io.WriteString(stdout, summary); writeErr != nil {
			return 1
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string) (string, error) {
	flags := flag.NewFlagSet("tacita-harness", flag.ContinueOnError)
	var flagOutput bytes.Buffer
	flags.SetOutput(&flagOutput)
	tacita := flags.String("tacita", "", "Tacita binary to run")
	runner := flags.String("runner", "", "description of the reference runner")
	out := flags.String("out", "", "directory for reports, sidecars, the lock, and the evidence index")
	paths := map[string]*string{}
	for _, repository := range report.CalibrationCorpus() {
		paths[repository.Name] = flags.String(flagName(repository), "", "repository holding "+repository.Commit)
	}
	if err := flags.Parse(args); err != nil {
		return "", fmt.Errorf("%s\n%s", usage, flagOutput.String())
	}
	if *tacita == "" || *runner == "" || *out == "" || flags.NArg() != 0 {
		return "", errors.New(usage)
	}
	var plan []repositoryRun
	for _, repository := range report.CalibrationCorpus() {
		path := *paths[repository.Name]
		if path == "" {
			return "", errors.New(usage)
		}
		for number := 1; number <= 2; number++ {
			plan = append(plan, repositoryRun{repository: repository, path: path, run: number})
		}
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", errUnsupportedPlatform
	}

	return record(ctx, *tacita, *runner, *out, plan)
}

func flagName(repository report.CorpusRepository) string {
	switch repository.Commit {
	case report.GolangTools().Commit:
		return "golang-tools"
	case report.Kubernetes().Commit:
		return "kubernetes"
	case report.GoStarter().Commit:
		return "go-starter"
	default:
		return "proficiency"
	}
}

// record runs the plan, writes every artifact into out, and returns a
// human-readable summary.
func record(ctx context.Context, tacita string, runner string, out string, plan []repositoryRun) (string, error) {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	records := make([]report.RunRecord, 0, len(plan))
	var summary bytes.Buffer
	for _, planned := range plan {
		record, err := calibrateOnce(ctx, tacita, runner, out, planned)
		if err != nil {
			return summary.String(), err
		}
		records = append(records, record)
		fmt.Fprintf(&summary, "%s run %d: exit %d, %s, peak %d bytes, report %t\n",
			planned.repository.Name, planned.run, record.Operation.ProcessExitStatus,
			time.Duration(record.Operation.ElapsedDurationNS), record.Operation.PeakResidentBytes, record.Report != nil)
	}

	input := report.EvidenceInput{Runs: records}
	if selection, ok, err := selectFrom(records); err != nil {
		return summary.String(), err
	} else if ok {
		input.Selection = &selection
		if selection.Lock != nil {
			lock, err := report.EncodeLock(*selection.Lock)
			if err != nil {
				return summary.String(), err
			}
			input.Lock = lock
			if err := writeFile(out, "development-lock.json", lock); err != nil {
				return summary.String(), err
			}
			fmt.Fprintf(&summary, "lock %s\n", selection.Lock.ConfigurationID)
		} else {
			summary.WriteString(selection.Summary())
			if err := writeFile(out, "selection.txt", []byte(selection.Summary())); err != nil {
				return summary.String(), err
			}
		}
	}

	index, err := report.BuildEvidence(input)
	if err != nil {
		return summary.String(), err
	}
	encoded, err := report.EncodeEvidenceIndex(index)
	if err != nil {
		return summary.String(), err
	}
	if err := writeFile(out, "evidence.json", encoded); err != nil {
		return summary.String(), err
	}
	fmt.Fprintf(&summary, "inferred lane %s", index.Lanes[0].Outcome)
	if reason := index.Lanes[0].Reason; reason != nil {
		fmt.Fprintf(&summary, " (%s)", *reason)
	}
	fmt.Fprintf(&summary, "\nexperiment %s\nevidence %s\n", index.ExperimentOutcome, index.EvidenceDigest)

	return summary.String(), nil
}

// calibrateOnce runs one calibration as a child process and records its
// report and operational sidecar. The child's own elapsed budget fires first;
// the harness kills it only after a margin.
func calibrateOnce(
	ctx context.Context,
	tacita string,
	runner string,
	out string,
	planned repositoryRun,
) (report.RunRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, report.ElapsedLimit+killMargin)
	defer cancel()

	stdout := &limitedBuffer{limit: maxStdout}
	stderr := &limitedBuffer{limit: maxStderr}
	cmd := exec.CommandContext(ctx, tacita, "calibrate", "--revision", planned.repository.Commit, planned.path) //nolint:gosec // operator-named binary
	cmd.Stdout, cmd.Stderr = stdout, stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	if cmd.ProcessState == nil {
		return report.RunRecord{}, fmt.Errorf("starting %s: %w", tacita, runErr)
	}

	name := fmt.Sprintf("%s.%d", filepath.Base(planned.path), planned.run)
	if err := writeFile(out, name+".stderr.txt", stderr.Bytes()); err != nil {
		return report.RunRecord{}, err
	}
	record := report.RunRecord{Repository: planned.repository, Run: planned.run}
	fields := report.OperationFields{
		Schema:            report.OperationSchema,
		ElapsedDurationNS: elapsed.Nanoseconds(),
		PeakResidentBytes: peakResidentBytes(cmd.ProcessState),
		ProcessExitStatus: exitStatus(cmd.ProcessState),
		ReferenceRunner:   runner,
	}
	if runErr == nil && !stdout.overflowed && stdout.Len() > 0 {
		record.Report = bytes.Clone(stdout.Bytes())
		digest := report.Digest(record.Report)
		fields.ReportDigest = &digest
		if err := writeFile(out, name+".report.json", record.Report); err != nil {
			return report.RunRecord{}, err
		}
	}
	operation, err := report.NewOperation(fields)
	if err != nil {
		return report.RunRecord{}, err
	}
	sidecar, err := report.EncodeOperation(operation)
	if err != nil {
		return report.RunRecord{}, err
	}
	if err := writeFile(out, name+".operation.json", sidecar); err != nil {
		return report.RunRecord{}, err
	}
	record.Operation = operation

	return record, nil
}

// selectFrom applies the selector when every run wrote a report. It reports
// false when a report is missing, which fails the lane without selection. A
// repository passes operationally only if both of its runs exited cleanly
// within the elapsed-time and memory budgets.
func selectFrom(records []report.RunRecord) (report.Selection, bool, error) {
	byRepository := make(map[string][]report.RunRecord)
	for _, record := range records {
		if record.Report == nil {
			return report.Selection{}, false, nil
		}
		byRepository[record.Repository.Commit] = append(byRepository[record.Repository.Commit], record)
	}
	runs := make([]report.RepositoryRun, 0, len(report.CalibrationCorpus()))
	for _, repository := range report.CalibrationCorpus() {
		recorded := byRepository[repository.Commit]
		pass := true
		for _, run := range recorded {
			pass = pass && run.Operation.ProcessExitStatus == 0 &&
				run.Operation.WithinElapsedLimit() && run.Operation.WithinMemoryLimit()
		}
		runs = append(runs, report.RepositoryRun{Report: recorded[0].Report, Repeat: recorded[1].Report, OperationalPass: pass})
	}

	selection, err := report.Select(report.SelectionInput{
		Development: [2]report.RepositoryRun{runs[0], runs[1]},
		Controls:    [2]report.RepositoryRun{runs[2], runs[3]},
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		return report.Selection{}, false, fmt.Errorf("selecting the development configuration: %w", err)
	}

	return selection, true, nil
}

// peakResidentBytes reads the child's peak resident set size. Linux reports
// ru_maxrss in kibibytes.
func peakResidentBytes(state *os.ProcessState) int64 {
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	return usage.Maxrss * 1024
}

// exitStatus is the child's exit code, or 128 plus the signal that killed it.
func exitStatus(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}

func writeFile(directory string, name string, data []byte) error {
	if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return nil
}

// limitedBuffer keeps at most limit bytes and records an overflow instead of
// growing past it.
type limitedBuffer struct {
	bytes.Buffer
	limit      int
	overflowed bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	room := b.limit - b.Len()
	if len(data) > room {
		b.overflowed = true
		data = data[:max(room, 0)]
	}
	b.Buffer.Write(data)
	return len(data), nil
}
