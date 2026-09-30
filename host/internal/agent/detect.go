package agent

// Found is an installed agent.
type Found struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Detect lists the known agents found in PATH, recommended first (spec §3.1
// step 9). lookPath is exec.LookPath outside tests.
func Detect(lookPath func(string) (string, error)) []Found {
	var found []Found
	for _, name := range Known {
		if p, err := lookPath(name); err == nil {
			found = append(found, Found{Name: name, Path: p})
		}
	}
	return found
}
