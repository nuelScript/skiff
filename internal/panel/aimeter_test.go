package panel

import (
	"io"
	"strings"
	"testing"
)

func meter(body string, stream bool, sent int) aiUsage {
	var got aiUsage
	m := &aiMeter{body: io.NopCloser(strings.NewReader(body)), stream: stream, sent: sent, done: func(u aiUsage) { got = u }}
	_, _ = io.Copy(io.Discard, m)
	_ = m.Close()
	return got
}

func TestGatewayCountsWhatProvidersBill(t *testing.T) {
	// Anthropic counts cached input apart from the rest.
	if u := meter(`{"usage":{"input_tokens":10,"cache_creation_input_tokens":200,"cache_read_input_tokens":3000,"output_tokens":7}}`, false, 0); u.in != 3210 || u.out != 7 {
		t.Errorf("Anthropic with caching: %+v", u)
	}
	// The Responses API's stream puts the counts under response.
	stream := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":42,\"output_tokens\":5}}}\n\n"
	if u := meter(stream, true, 100); u.in != 42 || u.out != 5 {
		t.Errorf("Responses API stream: %+v", u)
	}
	// A body too big to read whole still gives its counts, from its end.
	big := `{"choices":[{"message":{"content":"` + strings.Repeat("x", aiMaxBody) + `"}}],"usage":{"prompt_tokens":11,"completion_tokens":2100000}}`
	if u := meter(big, false, 0); u.in != 11 || u.out != 2100000 {
		t.Errorf("a body over the limit: %+v", u)
	}
	// A stream cut off before its counts is estimated from what went each way.
	cut := "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("a", 400) + "\"}}]}\n\n"
	if u := meter(cut, true, 4000); u.in != 1001 || u.out != 101 {
		t.Errorf("a cut-off stream: %+v", u)
	}
	// A stream that finished uses its own counts, not the estimate.
	whole := cut + "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":900,\"completion_tokens\":95}}\n\n"
	if u := meter(whole, true, 4000); u.in != 900 || u.out != 95 {
		t.Errorf("a finished stream: %+v", u)
	}
	// An Anthropic text delta counts toward the estimate.
	if n := streamedText([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)); n != 5 {
		t.Errorf("Anthropic delta: %d", n)
	}
}
