package otlp

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// A minimal protobuf writer, enough to build an export the way an SDK would.
func tag(num, wire int) []byte { return binary.AppendUvarint(nil, uint64(num<<3|wire)) }

func lenField(num int, b []byte) []byte {
	out := tag(num, 2)
	out = binary.AppendUvarint(out, uint64(len(b)))
	return append(out, b...)
}

func varintField(num int, v uint64) []byte { return binary.AppendUvarint(tag(num, 0), v) }

func fixed64Field(num int, v uint64) []byte { return binary.LittleEndian.AppendUint64(tag(num, 1), v) }

func kv(key string, value []byte) []byte {
	return append(lenField(1, []byte(key)), lenField(2, value)...)
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestReadProtobuf(t *testing.T) {
	trace := bytes.Repeat([]byte{0xab}, 16)
	span := cat(
		lenField(1, trace),
		lenField(2, []byte{1, 2, 3, 4, 5, 6, 7, 8}),
		lenField(4, []byte{9, 9, 9, 9, 9, 9, 9, 9}),
		lenField(5, []byte("chat gpt-4o")),
		varintField(6, 3),
		fixed64Field(7, 1_000_000_000),
		fixed64Field(8, 3_500_000_000),
		lenField(9, kv("gen_ai.request.model", lenField(1, []byte("gpt-4o")))),
		lenField(9, kv("gen_ai.usage.input_tokens", varintField(3, 120))),
		lenField(9, kv("temperature", fixed64Field(4, math.Float64bits(0.5)))),
		lenField(9, kv("stream", varintField(2, 1))),
		lenField(15, cat(lenField(2, []byte("rate limited")), varintField(3, 2))),
	)
	resource := lenField(1, lenField(1, kv("service.name", lenField(1, []byte("my-agent")))))
	// The resource after its spans, which the schema allows.
	body := lenField(1, cat(lenField(2, lenField(2, span)), resource))

	spans, err := Read(bytes.NewReader(body), "application/x-protobuf", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.TraceID != strings.Repeat("ab", 16) || s.SpanID != "0102030405060708" || s.ParentID != "0909090909090909" {
		t.Errorf("ids: %+v", s)
	}
	if s.Name != "chat gpt-4o" || s.Kind != 3 || s.Start != 1e9 || s.End != 3.5e9 || s.StatusCode != 2 || s.StatusMessage != "rate limited" {
		t.Errorf("span: %+v", s)
	}
	if s.Attrs["gen_ai.request.model"] != "gpt-4o" || s.Attrs["gen_ai.usage.input_tokens"] != int64(120) ||
		s.Attrs["temperature"] != 0.5 || s.Attrs["stream"] != true || s.Attrs["service.name"] != "my-agent" {
		t.Errorf("attrs: %v", s.Attrs)
	}

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(body)
	zw.Close()
	if spans, err := Read(&gz, "application/x-protobuf", "gzip"); err != nil || len(spans) != 1 {
		t.Errorf("gzip: %v %d", err, len(spans))
	}
	if _, err := Read(bytes.NewReader([]byte{0x0a, 0xff}), "application/x-protobuf", ""); err == nil {
		t.Error("a cut-off export was accepted")
	}
}

func TestReadJSON(t *testing.T) {
	body := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"crew"}}]},
	"scopeSpans":[{"spans":[{"traceId":"5B8EFFF798038103D269B633813FC60C","spanId":"EEE19B7EC3C1B174","parentSpanId":"",
	"name":"tool web_search","kind":1,"startTimeUnixNano":"1544712660000000000","endTimeUnixNano":"1544712661000000000",
	"attributes":[{"key":"gen_ai.tool.name","value":{"stringValue":"web_search"}},{"key":"n","value":{"intValue":"7"}},
	{"key":"tags","value":{"arrayValue":{"values":[{"stringValue":"a"},{"stringValue":"b"}]}}}],
	"status":{"code":1}}]}]}]}`
	spans, err := Read(strings.NewReader(body), "application/json", "")
	if err != nil || len(spans) != 1 {
		t.Fatalf("%v %d", err, len(spans))
	}
	s := spans[0]
	if s.TraceID != "5b8efff798038103d269b633813fc60c" || s.SpanID != "eee19b7ec3c1b174" || s.ParentID != "" || s.Start != 1544712660000000000 {
		t.Errorf("span: %+v", s)
	}
	if s.Attrs["gen_ai.tool.name"] != "web_search" || s.Attrs["n"] != int64(7) || s.Attrs["service.name"] != "crew" || len(s.Attrs["tags"].([]any)) != 2 {
		t.Errorf("attrs: %v", s.Attrs)
	}
}

func TestReadProtobufRefusesDeepNesting(t *testing.T) {
	nested := func(depth int) []byte {
		v := lenField(1, []byte("leaf"))
		for range depth {
			v = lenField(5, lenField(1, v))
		}
		span := cat(lenField(1, bytes.Repeat([]byte{1}, 16)), lenField(2, bytes.Repeat([]byte{2}, 8)), lenField(9, kv("tags", v)))
		return lenField(1, lenField(2, lenField(2, span)))
	}
	if spans, err := Read(bytes.NewReader(nested(2)), "application/x-protobuf", ""); err != nil || len(spans) != 1 {
		t.Fatalf("a list of lists: %v %d", err, len(spans))
	}
	if _, err := Read(bytes.NewReader(nested(200)), "application/x-protobuf", ""); err == nil {
		t.Fatal("accepted attributes nested 200 deep")
	}
}
