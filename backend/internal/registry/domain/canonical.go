package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// CanonicalJSON generates RFC 8785 (JSON Canonicalization Scheme - JCS) output for any JSON-serializable value.
func CanonicalJSON(v any) ([]byte, error) {
	// First marshal to standard JSON to respect struct tags, omitempty, custom marshaler, etc.
	var intermediate []byte
	switch val := v.(type) {
	case []byte:
		intermediate = val
	case json.RawMessage:
		intermediate = []byte(val)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("canonical json: marshal intermediate: %w", err)
		}
		intermediate = data
	}

	decoder := json.NewDecoder(bytes.NewReader(intermediate))
	decoder.UseNumber()

	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, fmt.Errorf("canonical json: decode intermediate: %w", err)
	}

	var buf bytes.Buffer
	if err := writeCanonical(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	if v == nil {
		buf.WriteString("null")
		return nil
	}

	switch val := v.(type) {
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil

	case string:
		writeCanonicalString(buf, val)
		return nil

	case json.Number:
		return writeCanonicalNumber(buf, val)

	case float64:
		return writeCanonicalFloat(buf, val)

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		fmt.Fprintf(buf, "%d", val)
		return nil

	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil

	case map[string]any:
		buf.WriteByte('{')
		// RFC 8785 section 3.2.3: keys sorted lexicographically by UTF-16 code units.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return compareUTF16(keys[i], keys[j]) < 0
		})

		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(buf, k)
			buf.WriteByte(':')
			if err := writeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil

	default:
		// Fallback: re-marshal through json
		data, err := json.Marshal(val)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var gen any
		if err := decoder.Decode(&gen); err != nil {
			return err
		}
		return writeCanonical(buf, gen)
	}
}

// compareUTF16 compares two strings lexicographically by their UTF-16 code units per RFC 8785 Section 3.2.3.
func compareUTF16(a, b string) int {
	u1 := utf16.Encode([]rune(a))
	u2 := utf16.Encode([]rune(b))
	n := len(u1)
	if len(u2) < n {
		n = len(u2)
	}
	for i := 0; i < n; i++ {
		if u1[i] < u2[i] {
			return -1
		}
		if u1[i] > u2[i] {
			return 1
		}
	}
	if len(u1) < len(u2) {
		return -1
	}
	if len(u1) > len(u2) {
		return 1
	}
	return 0
}

// writeCanonicalString serializes a string according to RFC 8785 Section 3.2.2.2.
func writeCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]

		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

func writeCanonicalNumber(buf *bytes.Buffer, n json.Number) error {
	str := string(n)
	// If it's a simple integer, write directly
	if !strings.ContainsAny(str, ".eE") {
		buf.WriteString(str)
		return nil
	}
	f, err := n.Float64()
	if err != nil {
		return fmt.Errorf("canonical number %q: %w", str, err)
	}
	return writeCanonicalFloat(buf, f)
}

func writeCanonicalFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonical json: invalid float %v", f)
	}
	// Check for negative zero
	if f == 0 {
		buf.WriteByte('0')
		return nil
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		fmt.Fprintf(buf, "%.0f", f)
		return nil
	}
	// ECMAScript-compatible float formatting:
	formatted := strconv.FormatFloat(f, 'g', -1, 64)
	// Replace uppercase 'E' with lowercase 'e' and strip redundant '+' in exponent
	formatted = strings.ReplaceAll(formatted, "E+", "e+")
	formatted = strings.ReplaceAll(formatted, "E-", "e-")
	formatted = strings.ReplaceAll(formatted, "e+", "e")
	buf.WriteString(formatted)
	return nil
}
