// Package report encodes and decodes Tacita's canonical experiment artifacts.
package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"
)

// decimalPlaces is the frozen number of digits after the decimal point in
// every decimal metric string.
const decimalPlaces = 6

// Decimal is a finite float64 rendered as a decimal string with exactly six
// digits after the point. Negative zero, and any value that rounds to it,
// renders as "0.000000".
type Decimal float64

// MarshalJSON renders the canonical decimal string or rejects a non-finite
// value.
func (d Decimal) MarshalJSON() ([]byte, error) {
	text, err := formatDecimal(float64(d))
	if err != nil {
		return nil, err
	}

	return json.Marshal(text)
}

// UnmarshalJSON accepts only a canonical decimal string.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return &DecodeError{Detail: "decimal is not a string"}
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return &DecodeError{Detail: "decimal " + strconv.Quote(text) + " is not a number"}
	}
	canonical, err := formatDecimal(value)
	if err != nil || canonical != text {
		return &DecodeError{Detail: "decimal " + strconv.Quote(text) + " is not canonical"}
	}
	*d = Decimal(value)

	return nil
}

func formatDecimal(value float64) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", &EncodeError{Detail: "non-finite decimal " + strconv.FormatFloat(value, 'g', -1, 64)}
	}
	text := strconv.FormatFloat(value, 'f', decimalPlaces, 64)
	if text == "-0.000000" {
		text = "0.000000"
	}

	return text, nil
}

// Bytes is an arbitrary repository byte string, rendered as its base64url
// encoding without padding and, when the bytes are valid UTF-8, a display
// string. Consumers compare bytes, never the display string.
type Bytes string

type byteIdentity struct {
	BytesB64URL string  `json:"bytes_b64url"`
	Display     *string `json:"display"`
}

// MarshalJSON renders the frozen byte identity.
func (b Bytes) MarshalJSON() ([]byte, error) {
	identity := byteIdentity{BytesB64URL: base64.RawURLEncoding.EncodeToString([]byte(b))}
	if utf8.ValidString(string(b)) {
		display := string(b)
		identity.Display = &display
	}

	return canonicalJSON(identity)
}

// UnmarshalJSON decodes a byte identity and rejects a display string that
// disagrees with the bytes.
func (b *Bytes) UnmarshalJSON(data []byte) error {
	var identity byteIdentity
	if err := strictUnmarshal(data, &identity); err != nil {
		return err
	}
	decoded, err := base64.RawURLEncoding.DecodeString(identity.BytesB64URL)
	if err != nil {
		return &DecodeError{Detail: "bytes_b64url is not unpadded base64url"}
	}
	valid := utf8.Valid(decoded)
	if valid != (identity.Display != nil) || (valid && *identity.Display != string(decoded)) {
		return &DecodeError{Detail: "display does not match bytes_b64url"}
	}
	*b = Bytes(decoded)

	return nil
}

// b64url returns the frozen base64url form of repository bytes.
func b64url(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// CandidateID is the frozen inferred candidate ID: SHA-256 over
// tacita-candidate-v1, the origin, and the antecedent and consequent
// bytes_b64url, joined by single NUL bytes with no trailing NUL.
func CandidateID(origin string, antecedent string, consequent string) string {
	return hashFields("tacita-candidate-v1", origin, b64url(antecedent), b64url(consequent))
}

func hashFields(fields ...string) string {
	hash := sha256.New()
	for index, field := range fields {
		if index > 0 {
			hash.Write([]byte{0})
		}
		hash.Write([]byte(field))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// Digest is the lowercase hexadecimal SHA-256 of canonical artifact bytes.
func Digest(artifact []byte) string {
	sum := sha256.Sum256(artifact)

	return hex.EncodeToString(sum[:])
}

// canonicalJSON renders one minified object without a trailing LF. Struct
// field order is the canonical field order; HTML characters are not escaped.
func canonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

// strictUnmarshal decodes exactly one JSON value and rejects unknown fields.
func strictUnmarshal(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &DecodeError{Detail: err.Error()}
	}
	if decoder.More() {
		return &DecodeError{Detail: "trailing data after the JSON value"}
	}

	return nil
}
