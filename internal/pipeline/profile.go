package pipeline

import "fmt"

// Profile selects a named subset of a shared pipeline configuration.
type Profile struct {
	Name        string
	Steps       []string
	Description string
}

// SelectProfile resolves a profile and returns its steps in graph order.
func SelectProfile(g *Graph, profiles []Profile, name string) ([]Step, error) {
	if name == "" {
		name = "fast"
	}
	var selected *Profile
	for i := range profiles {
		if profiles[i].Name == name {
			selected = &profiles[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("missing profile %q", name)
	}
	if len(selected.Steps) == 0 {
		return nil, fmt.Errorf("profile %q must include at least one step", name)
	}

	allowed := make(map[string]struct{}, len(selected.Steps))
	for _, id := range selected.Steps {
		if _, ok := g.Step(id); !ok {
			return nil, fmt.Errorf("profile %q references unknown step %q", name, id)
		}
		allowed[id] = struct{}{}
	}

	out := make([]Step, 0, len(allowed))
	for _, step := range g.Steps() {
		if _, ok := allowed[step.ID]; ok {
			for _, dep := range step.DependsOn {
				if _, included := allowed[dep]; !included {
					return nil, fmt.Errorf("profile %q step %q omits dependency %q", name, step.ID, dep)
				}
			}
			out = append(out, step)
		}
	}
	return out, nil
}
