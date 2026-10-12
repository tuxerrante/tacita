package report

import (
	"bytes"
	"time"
)

// Operational sidecar identifiers and frozen limits.
const (
	OperationSchema = "tacita.operation/v1"
	// maxSidecarBytes is the frozen operational sidecar byte budget.
	maxSidecarBytes = 1 << 20
	// ElapsedLimit and PeakMemoryLimit are the frozen per-run elapsed-time and
	// peak resident memory budgets the harness enforces.
	ElapsedLimit    = 20 * time.Minute
	PeakMemoryLimit = 2 << 30
	// maxExitStatus is the largest status a Linux process can report: an
	// 8-bit exit code or 128 plus a signal number.
	maxExitStatus = 255
)

// Operation is the canonical tacita.operation/v1 sidecar: the embedded
// OperationFields followed by the digest over them. It is not deterministic
// evidence and never changes the report it describes.
type Operation struct {
	OperationFields
	SidecarDigest string `json:"sidecar_digest"`
}

// OperationFields are the sidecar fields the digest covers, in order.
type OperationFields struct {
	Schema            string    `json:"schema"`
	ReportDigest      *string   `json:"report_digest"`
	ElapsedDurationNS int64     `json:"elapsed_duration_ns"`
	PeakResidentBytes int64     `json:"peak_resident_bytes"`
	ProcessExitStatus int       `json:"process_exit_status"`
	ReferenceRunner   string    `json:"reference_runner"`
	DiagnosticPaths   *[]string `json:"diagnostic_paths"`
}

// WithinElapsedLimit reports whether the run finished within the frozen
// elapsed-time budget.
func (o OperationFields) WithinElapsedLimit() bool {
	return o.ElapsedDurationNS <= int64(ElapsedLimit)
}

// WithinMemoryLimit reports whether the run stayed within the frozen peak
// resident memory budget.
func (o OperationFields) WithinMemoryLimit() bool {
	return o.PeakResidentBytes <= PeakMemoryLimit
}

// NewOperation seals fields with their digest.
func NewOperation(fields OperationFields) (Operation, error) {
	if err := validateOperation(fields); err != nil {
		return Operation{}, err
	}
	digest, err := fieldsDigest(fields)
	if err != nil {
		return Operation{}, err
	}

	return Operation{OperationFields: fields, SidecarDigest: digest}, nil
}

// EncodeOperation renders the canonical sidecar bytes after checking its
// contents, its digest, and the sidecar byte budget.
func EncodeOperation(operation Operation) ([]byte, error) {
	if err := validateOperation(operation.OperationFields); err != nil {
		return nil, err
	}
	digest, err := fieldsDigest(operation.OperationFields)
	if err != nil {
		return nil, err
	}
	if digest != operation.SidecarDigest {
		return nil, invalid("sidecar_digest does not cover the sidecar fields")
	}
	encoded, err := canonicalJSON(operation)
	if err != nil {
		return nil, &EncodeError{Detail: err.Error()}
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxSidecarBytes {
		return nil, invalid("sidecar holds %d bytes, budget %d", len(encoded), maxSidecarBytes)
	}

	return encoded, nil
}

// DecodeOperation parses canonical sidecar bytes and verifies the digest.
func DecodeOperation(data []byte) (Operation, error) {
	if len(data) > maxSidecarBytes {
		return Operation{}, &DecodeError{Detail: "sidecar exceeds its byte budget"}
	}
	var operation Operation
	if err := strictUnmarshal(data, &operation); err != nil {
		return Operation{}, err
	}
	canonical, err := EncodeOperation(operation)
	if err != nil {
		return Operation{}, asDecodeError(err)
	}
	if !bytes.Equal(canonical, data) {
		return Operation{}, &DecodeError{Detail: "sidecar bytes are not canonical"}
	}

	return operation, nil
}

// validateOperation requires the frozen schema, a well-formed report digest
// or null when the run wrote no report, non-negative observations (a run
// killed by a signal records 128 plus the signal number), and a runner
// description.
func validateOperation(fields OperationFields) error {
	switch {
	case fields.Schema != OperationSchema:
		return invalid("sidecar schema %q", fields.Schema)
	case fields.ReportDigest != nil && !isDigest(*fields.ReportDigest):
		return invalid("sidecar report_digest %q", *fields.ReportDigest)
	case fields.ElapsedDurationNS < 0 || fields.PeakResidentBytes < 0 ||
		fields.ProcessExitStatus < 0 || fields.ProcessExitStatus > maxExitStatus:
		return invalid("sidecar observations must be non-negative and the exit status at most %d", maxExitStatus)
	case fields.ReferenceRunner == "":
		return invalid("sidecar reference_runner is empty")
	}

	return nil
}
