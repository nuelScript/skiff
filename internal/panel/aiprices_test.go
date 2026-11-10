package panel

import (
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/otlp"
)

func TestPriceForTakesTheLongestName(t *testing.T) {
	prices := []aiPrice{{Model: "gpt-4o", Input: 2.5, Output: 10}, {Model: "gpt-4o-mini", Input: 0.15, Output: 0.6}}
	for model, want := range map[string]string{"gpt-4o-2024-08-06": "gpt-4o", "GPT-4o-mini-2024-07-18": "gpt-4o-mini", "gpt-4o-mini": "gpt-4o-mini"} {
		if p, ok := priceFor(prices, model); !ok || p.Model != want {
			t.Errorf("%s: %+v %v", model, p, ok)
		}
	}
	if _, ok := priceFor(prices, "claude-sonnet-4-5"); ok {
		t.Error("an unlisted model was priced")
	}
	// 1M in at $2.50 and 500k out at $10 is $7.50.
	if got := costOf(prices[0], 1_000_000, 500_000); got != 7.5 {
		t.Errorf("cost %v", got)
	}
}

func TestPriceForSkipsOtherModels(t *testing.T) {
	prices := []aiPrice{{Model: "gpt-4", Input: 30, Output: 60}, {Model: "claude-sonnet-4", Input: 3, Output: 15}}
	for model, want := range map[string]bool{"gpt-4": true, "GPT-4": true, "gpt-4-0613": true, "gpt-4o": false, "gpt-4.1": false, "gpt-4-turbo": false, "claude-sonnet-4-20250514": true, "claude-sonnet-4-5": false} {
		if _, ok := priceFor(prices, model); ok != want {
			t.Errorf("%s: priced %v", model, ok)
		}
	}
}

func TestRunsShowTheirCost(t *testing.T) {
	openTestDB(t)
	now := time.Now().UnixNano()
	span := func(id, model string, in, out int64) otlp.Span {
		return otlp.Span{TraceID: traceID, SpanID: id, Name: "chat " + model, Start: now, End: now + 1e6,
			Attrs: map[string]any{"gen_ai.request.model": model, "gen_ai.usage.input_tokens": in, "gen_ai.usage.output_tokens": out}}
	}
	if err := storeSpans("bot", []otlp.Span{span("0000000000000001", "gpt-4o-mini-2024-07-18", 2_000_000, 1_000_000), span("0000000000000002", "local-llama", 500, 50)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	prices := []aiPrice{{Model: "gpt-4o-mini", Input: 0.15, Output: 0.6}}
	run := listTraces("bot", 5, prices)[0]
	if run.Cost == nil || *run.Cost != 0.9 || !run.Partial {
		t.Errorf("run cost %v partial %v", run.Cost, run.Partial)
	}
	if listTraces("bot", 5, nil)[0].Cost != nil {
		t.Error("a cost without prices")
	}
	var priced, unpriced int
	for _, s := range traceSpans("bot", traceID, prices) {
		if s.Cost != nil {
			priced++
		} else {
			unpriced++
		}
	}
	if priced != 1 || unpriced != 1 {
		t.Errorf("spans priced %d, unpriced %d", priced, unpriced)
	}
}
