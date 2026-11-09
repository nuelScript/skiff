package panel

import (
	"strings"
	"testing"
)

func TestCheckCommand(t *testing.T) {
	if checkCommand(nil) != "" || checkCommand([]string{"Qwen/Qwen3-0.6B", "--max-model-len", "8192"}) != "" {
		t.Error("a plain command was refused")
	}
	if checkCommand(make([]string, 65)) == "" || checkCommand([]string{"a\x00b"}) == "" || checkCommand([]string{strings.Repeat("x", 4097)}) == "" {
		t.Error("a command past the limits was accepted")
	}
}
