package agent

import (
	"fmt"
	"regexp"
	"slices"
)

// Efforts are the values Claude Code takes for --effort.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// modelRE is a model alias or id as Claude Code takes it for --model:
// letters, digits and . _ : - [ ] (so opus, claude-sonnet-5-5, opus[1m]).
// It never starts with "-": it cannot be read as another option.
var modelRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\[\]-]{0,63}$`)

// ValidModel reports whether s is acceptable as a model name for the agent.
func ValidModel(s string) bool { return modelRE.MatchString(s) }

// ValidEffort reports whether s is one of Efforts.
func ValidEffort(s string) bool { return slices.Contains(Efforts, s) }

// Args returns the command-line arguments that make agent use model and
// effort, and the names of the settings that agent does not take (so serve can
// say they were not applied). Empty values add nothing: the agent keeps what
// the user configured in it. Only Claude Code is known to take both.
func Args(name, model, effort string) (args []string, unused []string) {
	if name == "claude" {
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" {
			args = append(args, "--effort", effort)
		}
		return args, nil
	}
	if model != "" {
		unused = append(unused, "model")
	}
	if effort != "" {
		unused = append(unused, "effort")
	}
	return nil, unused
}

// Describe is what serve tells the person for a setting it applies.
func Describe(model, effort string) string {
	switch {
	case model != "" && effort != "":
		return fmt.Sprintf("model %s, effort %s", model, effort)
	case model != "":
		return "model " + model
	case effort != "":
		return "effort " + effort
	}
	return ""
}
