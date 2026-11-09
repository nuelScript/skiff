// Package otlp reads OpenTelemetry trace exports, in either of OTLP/HTTP's encodings, into flat spans.
package otlp

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Span is one span, with its resource's attributes folded in under their own keys.
type Span struct {
	TraceID, SpanID, ParentID string // hex
	Name                      string
	Kind                      int
	Start, End                int64 // unix nanoseconds
	Attrs                     map[string]any
	StatusCode                int // 0 unset, 1 ok, 2 error
	StatusMessage             string
}

// MaxBody is the largest export read, after decompression.
const MaxBody = 16 << 20

// Read decodes a body sent to /v1/traces. contentType picks the encoding; encoding is the Content-Encoding.
func Read(body io.Reader, contentType, encoding string) ([]Span, error) {
	if strings.EqualFold(encoding, "gzip") {
		zr, err := gzip.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		body = zr
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBody {
		return nil, errors.New("the export is over 16 MB")
	}
	if strings.HasPrefix(contentType, "application/json") {
		return readJSON(raw)
	}
	return readProto(raw)
}

// The JSON encoding: ids are hex, 64-bit numbers are strings.
type jsonAnyValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	ArrayValue  *struct {
		Values []jsonAnyValue `json:"values"`
	} `json:"arrayValue"`
}

type jsonKeyValue struct {
	Key   string       `json:"key"`
	Value jsonAnyValue `json:"value"`
}

func (v jsonAnyValue) value() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.IntValue != nil:
		n, _ := strconv.ParseInt(strings.Trim(string(v.IntValue), `"`), 10, 64)
		return n
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, e := range v.ArrayValue.Values {
			out = append(out, e.value())
		}
		return out
	}
	return nil
}

func jsonAttrs(into map[string]any, kvs []jsonKeyValue) {
	for _, kv := range kvs {
		into[kv.Key] = kv.Value.value()
	}
}

func jsonNano(raw json.RawMessage) int64 {
	n, _ := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	return n
}

func readJSON(raw []byte) ([]Span, error) {
	var req struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []jsonKeyValue `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []struct {
					TraceID           string          `json:"traceId"`
					SpanID            string          `json:"spanId"`
					ParentSpanID      string          `json:"parentSpanId"`
					Name              string          `json:"name"`
					Kind              int             `json:"kind"`
					StartTimeUnixNano json.RawMessage `json:"startTimeUnixNano"`
					EndTimeUnixNano   json.RawMessage `json:"endTimeUnixNano"`
					Attributes        []jsonKeyValue  `json:"attributes"`
					Status            struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					} `json:"status"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("the export isn't OTLP JSON: %w", err)
	}
	var out []Span
	for _, rs := range req.ResourceSpans {
		resource := map[string]any{}
		jsonAttrs(resource, rs.Resource.Attributes)
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				sp := Span{TraceID: strings.ToLower(s.TraceID), SpanID: strings.ToLower(s.SpanID), ParentID: strings.ToLower(s.ParentSpanID),
					Name: s.Name, Kind: s.Kind, Start: jsonNano(s.StartTimeUnixNano), End: jsonNano(s.EndTimeUnixNano),
					Attrs: map[string]any{}, StatusCode: s.Status.Code, StatusMessage: s.Status.Message}
				for k, v := range resource {
					sp.Attrs[k] = v
				}
				jsonAttrs(sp.Attrs, s.Attributes)
				out = append(out, sp)
			}
		}
	}
	return out, nil
}

// The protobuf encoding, read field by field: only the parts of the trace schema a span view needs.
type field struct {
	num   int
	wire  int
	n     uint64 // varint and fixed values
	bytes []byte // length-delimited values
}

var errProto = errors.New("the export isn't OTLP protobuf")

func fields(b []byte, fn func(f field) error) error {
	for len(b) > 0 {
		key, n := uvarint(b)
		if n <= 0 {
			return errProto
		}
		b = b[n:]
		f := field{num: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := uvarint(b)
			if n <= 0 {
				return errProto
			}
			f.n, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return errProto
			}
			f.n, b = le64(b), b[8:]
		case 2:
			l, n := uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return errProto
			}
			f.bytes, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return errProto
			}
			f.n, b = uint64(b[0])|uint64(b[1])<<8|uint64(b[2])<<16|uint64(b[3])<<24, b[4:]
		default:
			return errProto
		}
		if err := fn(f); err != nil {
			return err
		}
	}
	return nil
}

func uvarint(b []byte) (uint64, int) {
	var x uint64
	for i := 0; i < len(b) && i < 10; i++ {
		x |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return x, i + 1
		}
	}
	return 0, 0
}

func le64(b []byte) uint64 {
	var x uint64
	for i := 7; i >= 0; i-- {
		x = x<<8 | uint64(b[i])
	}
	return x
}

func protoAnyValue(b []byte) (any, error) {
	var v any
	err := fields(b, func(f field) error {
		switch f.num {
		case 1:
			v = string(f.bytes)
		case 2:
			v = f.n != 0
		case 3:
			v = int64(f.n)
		case 4:
			v = math.Float64frombits(f.n)
		case 5:
			var list []any
			if err := fields(f.bytes, func(e field) error {
				if e.num == 1 {
					item, err := protoAnyValue(e.bytes)
					list = append(list, item)
					return err
				}
				return nil
			}); err != nil {
				return err
			}
			v = list
		case 7:
			v = hex.EncodeToString(f.bytes)
		}
		return nil
	})
	return v, err
}

func protoKeyValue(into map[string]any, b []byte) error {
	var key string
	var val any
	err := fields(b, func(f field) error {
		switch f.num {
		case 1:
			key = string(f.bytes)
		case 2:
			v, err := protoAnyValue(f.bytes)
			val = v
			return err
		}
		return nil
	})
	if err == nil && key != "" {
		into[key] = val
	}
	return err
}

func protoSpan(b []byte, resource map[string]any) (Span, error) {
	s := Span{Attrs: map[string]any{}}
	for k, v := range resource {
		s.Attrs[k] = v
	}
	err := fields(b, func(f field) error {
		switch f.num {
		case 1:
			s.TraceID = hex.EncodeToString(f.bytes)
		case 2:
			s.SpanID = hex.EncodeToString(f.bytes)
		case 4:
			s.ParentID = hex.EncodeToString(f.bytes)
		case 5:
			s.Name = string(f.bytes)
		case 6:
			s.Kind = int(f.n)
		case 7:
			s.Start = int64(f.n)
		case 8:
			s.End = int64(f.n)
		case 9:
			return protoKeyValue(s.Attrs, f.bytes)
		case 15:
			return fields(f.bytes, func(st field) error {
				switch st.num {
				case 2:
					s.StatusMessage = string(st.bytes)
				case 3:
					s.StatusCode = int(st.n)
				}
				return nil
			})
		}
		return nil
	})
	return s, err
}

func readProto(raw []byte) ([]Span, error) {
	var out []Span
	err := fields(raw, func(rs field) error {
		if rs.num != 1 {
			return nil
		}
		resource := map[string]any{}
		var scopes [][]byte
		if err := fields(rs.bytes, func(f field) error {
			switch f.num {
			case 1:
				return fields(f.bytes, func(a field) error {
					if a.num == 1 {
						return protoKeyValue(resource, a.bytes)
					}
					return nil
				})
			case 2:
				scopes = append(scopes, f.bytes)
			}
			return nil
		}); err != nil {
			return err
		}
		// The resource can come after its spans, so spans are read once it's known.
		for _, scope := range scopes {
			if err := fields(scope, func(f field) error {
				if f.num != 2 {
					return nil
				}
				s, err := protoSpan(f.bytes, resource)
				out = append(out, s)
				return err
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 && len(bytes.TrimSpace(raw)) > 0 && raw[0] == '{' {
		return readJSON(raw)
	}
	return out, nil
}
