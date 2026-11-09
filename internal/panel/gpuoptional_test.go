package panel

import "testing"

func TestOllamaRunsWithoutAGPU(t *testing.T) {
	if !gpusOptional(Source{Catalog: "ollama", GPUs: 1}) {
		t.Error("Ollama should deploy without a GPU when none is free")
	}
	if gpusOptional(Source{Catalog: "vllm", GPUs: 1}) || gpusOptional(Source{GPUs: 1}) {
		t.Error("vLLM and an app of your own need the GPUs they ask for")
	}
}
