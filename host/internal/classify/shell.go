package classify

// classifyScript will analyse the phone's shell command line (next task).
// Until then every `adb shell` command is 🔴: unknown means dangerous.
func (c *Classifier) classifyScript(src string, depth int) []Finding {
	return []Finding{redFinding("shell.parse", "shell commands are not analysed yet", []string{src}, textParse)}
}
