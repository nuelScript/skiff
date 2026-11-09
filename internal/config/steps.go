package config

import (
	"fmt"
	"slices"
)

// A Step is one stage of a job: its command runs once every step in After has succeeded.
type Step struct {
	Name    string   `toml:"name" json:"name"`
	Command string   `toml:"command" json:"command"`
	After   []string `toml:"after" json:"after,omitempty"`
}

// MaxSteps bounds a job's pipeline.
const MaxSteps = 20

// ValidateSteps checks names are unique, every step it comes after exists, and the steps can be put in an order.
func ValidateSteps(steps []Step) error {
	if len(steps) > MaxSteps {
		return fmt.Errorf("a job can have at most %d steps", MaxSteps)
	}
	names := map[string]bool{}
	for _, s := range steps {
		if !validName(s.Name) {
			return fmt.Errorf("step name %q: use 1-32 lowercase letters, digits or dashes", s.Name)
		}
		if names[s.Name] {
			return fmt.Errorf("two steps are named %s", s.Name)
		}
		names[s.Name] = true
		if s.Command == "" {
			return fmt.Errorf("step %s has no command", s.Name)
		}
	}
	for _, s := range steps {
		for _, a := range s.After {
			if !names[a] {
				return fmt.Errorf("step %s comes after %s, which isn't a step", s.Name, a)
			}
			if a == s.Name {
				return fmt.Errorf("step %s comes after itself", s.Name)
			}
		}
	}
	if _, err := StepWaves(steps); err != nil {
		return err
	}
	return nil
}

// StepWaves orders steps into waves: each wave's steps come only after steps in earlier waves, so a wave's steps can run side by side.
func StepWaves(steps []Step) ([][]Step, error) {
	done := map[string]bool{}
	var waves [][]Step
	left := slices.Clone(steps)
	for len(left) > 0 {
		var wave, rest []Step
		for _, s := range left {
			ready := true
			for _, a := range s.After {
				ready = ready && done[a]
			}
			if ready {
				wave = append(wave, s)
			} else {
				rest = append(rest, s)
			}
		}
		if len(wave) == 0 {
			return nil, fmt.Errorf("steps %s come after each other in a loop", stepNames(rest))
		}
		for _, s := range wave {
			done[s.Name] = true
		}
		waves, left = append(waves, wave), rest
	}
	return waves, nil
}

func stepNames(steps []Step) string {
	out := ""
	for i, s := range steps {
		if i > 0 {
			out += ", "
		}
		out += s.Name
	}
	return out
}
