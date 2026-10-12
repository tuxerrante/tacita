package report

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestDecimalRendersSixDigits(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 0.8125, want: `"0.812500"`},
		{value: 1, want: `"1.000000"`},
		{value: 4674.25, want: `"4674.250000"`},
		{value: 1.0 / 3.0, want: `"0.333333"`},
		{value: 2.0 / 3.0, want: `"0.666667"`},
		{value: math.Copysign(0, -1), want: `"0.000000"`},
		{value: -1e-9, want: `"0.000000"`},
		{value: -0.25, want: `"-0.250000"`},
	}

	for _, tt := range tests {
		got, err := json.Marshal(Decimal(tt.value))
		if err != nil {
			t.Fatalf("Marshal(%v) error = %v", tt.value, err)
		}
		if string(got) != tt.want {
			t.Errorf("Marshal(%v) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestDecimalRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := json.Marshal(Decimal(value)); err == nil {
			t.Errorf("Marshal(%v) succeeded, want an error", value)
		}
	}
}

func TestDecimalDecodesOnlyCanonicalStrings(t *testing.T) {
	var decimal Decimal
	if err := json.Unmarshal([]byte(`"0.812500"`), &decimal); err != nil || decimal != 0.8125 {
		t.Fatalf("Unmarshal(canonical) = %v, %v", decimal, err)
	}
	for _, input := range []string{`"0.8125"`, `0.812500`, `"-0.000000"`, `"NaN"`, `"x"`, `"1e3"`} {
		err := json.Unmarshal([]byte(input), &decimal)
		if !errors.Is(err, ErrDecode) {
			t.Errorf("Unmarshal(%s) error = %v, want ErrDecode", input, err)
		}
	}
}

func TestBytesIdentity(t *testing.T) {
	tests := []struct {
		value Bytes
		want  string
	}{
		{value: "internal/report", want: `{"bytes_b64url":"aW50ZXJuYWwvcmVwb3J0","display":"internal/report"}`},
		{value: "odd\xff", want: `{"bytes_b64url":"b2Rk_w","display":null}`},
		{value: "a<b>&", want: `{"bytes_b64url":"YTxiPiY","display":"a<b>&"}`},
		{value: "", want: `{"bytes_b64url":"","display":""}`},
	}

	for _, tt := range tests {
		got, err := canonicalJSON(tt.value)
		if err != nil {
			t.Fatalf("canonicalJSON(%q) error = %v", tt.value, err)
		}
		if string(got) != tt.want {
			t.Errorf("canonicalJSON(%q) = %s, want %s", tt.value, got, tt.want)
		}
		var decoded Bytes
		if err := json.Unmarshal(got, &decoded); err != nil || decoded != tt.value {
			t.Errorf("round trip of %q = %q, %v", tt.value, decoded, err)
		}
	}
}

func TestBytesRejectsInconsistentIdentities(t *testing.T) {
	for _, input := range []string{
		`{"bytes_b64url":"YQ","display":"b"}`,
		`{"bytes_b64url":"YQ","display":null}`,
		`{"bytes_b64url":"b2Rk_w","display":"odd"}`,
		`{"bytes_b64url":"YQ==","display":"a"}`,
		`{"bytes_b64url":"YQ","display":"a","extra":1}`,
	} {
		var decoded Bytes
		if err := json.Unmarshal([]byte(input), &decoded); !errors.Is(err, ErrDecode) {
			t.Errorf("Unmarshal(%s) error = %v, want ErrDecode", input, err)
		}
	}
}

func TestCandidateIDMatchesTheWorkedVector(t *testing.T) {
	got := CandidateID("inferred", "config", "charts/kapparmor")

	const want = "dd2fbb9a369995a25ef008e95a8b6027ebc4b72f44acd0689d933600f1150dbb"
	if got != want {
		t.Errorf("CandidateID() = %s, want %s", got, want)
	}
}

func TestDigest(t *testing.T) {
	const want = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := Digest(nil); got != want {
		t.Errorf("Digest(empty) = %s, want %s", got, want)
	}
}

func TestStrictUnmarshalRejectsTrailingData(t *testing.T) {
	var value struct{ A int }
	if err := strictUnmarshal([]byte(`{"A":1}{"A":2}`), &value); !errors.Is(err, ErrDecode) {
		t.Errorf("strictUnmarshal(two values) error = %v, want ErrDecode", err)
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: &EncodeError{Detail: "x"}, want: "encoding canonical artifact: x"},
		{err: &DecodeError{Detail: "y"}, want: "decoding canonical artifact: y"},
		{err: &LimitError{Code: "budget_report_rows", Observed: 3, Limit: 2}, want: "budget_report_rows: observed 3, limit 2"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}
