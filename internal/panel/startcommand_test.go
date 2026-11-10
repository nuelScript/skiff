package panel

import (
	"strings"
	"testing"
)

func TestClearingTheStartCommandRestoresTheRecipes(t *testing.T) {
	if got := storedCommand(Source{Catalog: "vllm"}, nil); !strings.Contains(got, "Qwen") {
		t.Errorf("cleared on vLLM: %q, want its recipe's command", got)
	}
	if got := storedCommand(Source{Catalog: "vllm"}, []string{"--model", "x"}); got != `["--model","x"]` {
		t.Errorf("a command of your own: %q", got)
	}
	if got := storedCommand(Source{Image: "nginx"}, nil); got != "" {
		t.Errorf("cleared on an image: %q, want the image's own", got)
	}
}
