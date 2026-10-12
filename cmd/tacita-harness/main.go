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
	"slices"
	"strconv"
	"strings"
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
	entries, err := os.ReadDir(out)
	if err != nil {
		return "", fmt.Errorf("reading output directory: %w", err)
	}
	if len(entries) > 0 {
		return "", fmt.Errorf("output directory %s is not empty; a bundle must not mix runs", out)
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
// the harness kills its process group only after a margin, as soon as its
// output overflows a bound, or as soon as a peak resident set in the group
// exceeds the frozen memory budget.
func calibrateOnce(
	ctx context.Context,
	tacita string,
	runner string,
	out string,
	planned repositoryRun,
) (report.RunRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, report.ElapsedLimit+killMargin)
	defer cancel()

	stdout := &limitedBuffer{limit: maxStdout, stop: cancel}
	stderr := &limitedBuffer{limit: maxStderr, stop: cancel}
	cmd := groupCommand(ctx, tacita, "calibrate", "--revision", planned.repository.Commit, planned.path)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	start := time.Now()
	sampled, runErr := runWatchingMemory(cmd, cancel, report.PeakMemoryLimit)
	elapsed := time.Since(start)
	if cmd.ProcessState == nil {
		return report.RunRecord{}, fmt.Errorf("starting %s: %w", tacita, runErr)
	}
	if err := awaitGroup(cmd.Process.Pid); err != nil {
		return report.RunRecord{}, err
	}

	name := fmt.Sprintf("%s.%d", flagName(planned.repository), planned.run)
	record := report.RunRecord{Repository: planned.repository, Run: planned.run}
	fields := report.OperationFields{
		Schema:            report.OperationSchema,
		ElapsedDurationNS: elapsed.Nanoseconds(),
		PeakResidentBytes: max(peakResidentBytes(cmd.ProcessState), sampled),
		ProcessExitStatus: exitStatus(cmd.ProcessState),
		ReferenceRunner:   runner,
	}
	if stderr.Len() > 0 {
		diagnostic := name + ".stderr.txt"
		if err := writeFile(out, diagnostic, stderr.Bytes()); err != nil {
			return report.RunRecord{}, err
		}
		fields.DiagnosticPaths = &[]string{diagnostic}
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

// memoryPoll is how often the harness samples the run's peak resident set.
const memoryPoll = 50 * time.Millisecond

// groupExitWait bounds how long a killed process group may take to exit.
const groupExitWait = 30 * time.Second

// groupCommand runs name in a new process group whose leader is the child, so
// cancelling ctx kills the child and every descendant still in the group, not
// the child alone.
func groupCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // operator-named binary
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return cmd
}

// runWatchingMemory starts cmd and waits for it while sampling the peak
// resident set of the child and its descendants from /proc. When a peak
// exceeds limit, the frozen memory budget, it calls stop, which kills the
// child's process group through its context. It returns the largest sample,
// which covers a descendant killed before the child could wait for it and so
// missing from ru_maxrss. One goroutine waits for the child; it ends when the
// child exits, and its result is always received before returning.
func runWatchingMemory(cmd *exec.Cmd, stop context.CancelFunc, limit int64) (int64, error) {
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var sampled int64
	ticker := time.NewTicker(memoryPoll)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return sampled, err
		case <-ticker.C:
			peak, ok := treeHighWaterMark("/proc", cmd.Process.Pid)
			if !ok {
				continue
			}
			sampled = max(sampled, peak)
			if peak > limit {
				stop()
			}
		}
	}
}

// awaitGroup kills whatever remains of the process group led by a child that
// has been waited for, reaps members reparented to the harness, and waits
// until no member remains, so no descendant of one run outlives it.
func awaitGroup(group int) error {
	_ = syscall.Kill(-group, syscall.SIGKILL) // ESRCH: the group already exited
	deadline := time.Now().Add(groupExitWait)
	for {
		for {
			pid, err := syscall.Wait4(-group, nil, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
		}
		if err := syscall.Kill(-group, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process group %d outlived its run by %s", group, groupExitWait)
		}
		time.Sleep(memoryPoll)
	}
}

// treeHighWaterMark is the largest VmHWM among the process root and its
// descendants under a /proc tree. It reports false when root's own status
// cannot be read; a descendant that exits while the tree is read is skipped.
func treeHighWaterMark(proc string, root int) (int64, bool) {
	peak, ok := highWaterMark(filepath.Join(proc, strconv.Itoa(root), "status"))
	if !ok {
		return 0, false
	}
	entries, err := os.ReadDir(proc)
	if err != nil {
		return peak, true
	}
	children := make(map[int][]int)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if parent, found := statusField(filepath.Join(proc, entry.Name(), "status"), "PPid:"); found {
			children[int(parent)] = append(children[int(parent)], pid)
		}
	}
	pending := slices.Clone(children[root])
	seen := map[int]bool{root: true}
	for len(pending) > 0 {
		pid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		if value, found := highWaterMark(filepath.Join(proc, strconv.Itoa(pid), "status")); found {
			peak = max(peak, value)
		}
		pending = append(pending, children[pid]...)
	}

	return peak, true
}

// highWaterMark reads VmHWM, the peak resident set, from a /proc status file.
func highWaterMark(path string) (int64, bool) {
	kibibytes, found := statusField(path, "VmHWM:")
	return kibibytes * 1024, found
}

// statusField reads one integer field from a /proc status file, dropping a
// trailing kB unit.
func statusField(path, name string) (int64, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // a /proc path built from a PID
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		value, found := strings.CutPrefix(line, name)
		if !found {
			continue
		}
		parsed, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}

	return 0, false
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

// limitedBuffer keeps at most limit bytes. The first write past the limit
// records the overflow and calls stop, so the child is killed at once instead
// of running on with its output discarded.
type limitedBuffer struct {
	bytes.Buffer
	limit      int
	overflowed bool
	stop       func()
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	room := b.limit - b.Len()
	if len(data) > room {
		if !b.overflowed && b.stop != nil {
			b.stop()
		}
		b.overflowed = true
		data = data[:max(room, 0)]
	}
	b.Buffer.Write(data)
	return len(data), nil
}
