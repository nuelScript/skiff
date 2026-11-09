package config

import (
	"strings"
	"testing"
)

func TestStepWaves(t *testing.T) {
	steps := []Step{
		{Name: "load", Command: "x", After: []string{"transform"}},
		{Name: "extract", Command: "x"},
		{Name: "transform", Command: "x", After: []string{"extract"}},
		{Name: "report", Command: "x", After: []string{"extract"}},
	}
	waves, err := StepWaves(steps)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range waves {
		var names []string
		for _, s := range w {
			names = append(names, s.Name)
		}
		got = append(got, strings.Join(names, "+"))
	}
	if strings.Join(got, " | ") != "extract | transform+report | load" {
		t.Errorf("waves: %v", got)
	}
}

func TestValidateSteps(t *testing.T) {
	for want, steps := range map[string][]Step{
		"two steps are named": {{Name: "a", Command: "x"}, {Name: "a", Command: "y"}},
		"isn't a step":        {{Name: "a", Command: "x", After: []string{"missing"}}},
		"after itself":        {{Name: "a", Command: "x", After: []string{"a"}}},
		"in a loop":           {{Name: "a", Command: "x", After: []string{"b"}}, {Name: "b", Command: "x", After: []string{"a"}}},
		"has no command":      {{Name: "a"}},
		"lowercase letters":   {{Name: "Bad Name", Command: "x"}},
	} {
		if err := ValidateSteps(steps); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: %v, want %q", steps, err, want)
		}
	}
	if err := ValidateJobs([]Job{{Name: "nightly", Schedule: "0 2 * * *", Command: "x", Steps: []Step{{Name: "a", Command: "y"}}}}); err == nil {
		t.Error("a job with both a command and steps was accepted")
	}
	if err := ValidateJobs([]Job{{Name: "nightly", Schedule: "0 2 * * *", Steps: []Step{{Name: "a", Command: "y"}}}}); err != nil {
		t.Errorf("a job with only steps: %v", err)
	}
}
