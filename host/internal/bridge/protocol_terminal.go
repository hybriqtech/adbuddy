package bridge

// Terminal payloads (protocol spec §7): M3. The PTY's output is a stream with
// its own position: Epoch names one PTY process and Offset counts its output
// bytes from 0. Data fields are bytes, base64 in JSON.

// TerminalWatch subscribes the app to the terminal, from what it already has.
type TerminalWatch struct {
	Epoch  string `json:"epoch"`  // empty when the app has nothing
	Offset int64  `json:"offset"` // 0 when the app has nothing
}

// TerminalSnapshot is the screen at Offset: Data, written to a clean
// terminal of Cols×Rows, draws it.
type TerminalSnapshot struct {
	Epoch  string `json:"epoch"`
	Offset int64  `json:"offset"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Data   []byte `json:"data"`
}

// TerminalOutput is output of the PTY; Offset is the position of its first byte.
type TerminalOutput struct {
	Epoch  string `json:"epoch"`
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"` // at most 64 KiB
}

// TerminalSize says the size changes from byte Offset on.
type TerminalSize struct {
	Epoch  string `json:"epoch"`
	Offset int64  `json:"offset"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
}

// TerminalInput is bytes for the PTY, as they are (quick keys included).
type TerminalInput struct {
	Data []byte `json:"data"` // at most 64 KiB
}

// TerminalResize is the size that fits the phone's view.
type TerminalResize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}
