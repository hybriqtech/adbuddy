package bridge

// YOLO mode payloads (spec docs/superpowers/specs/2026-10-18-adbuddy-modo-yolo-design.md,
// protocol spec §8.3). The app turns it on and off with yolo.set; the host
// answers with yolo.state and sends it on every change, in the snapshot and
// after the welcome.

// YoloSet turns YOLO on for Minutes (15 or 60) or until the session ends
// (Session), or off (Off). Exactly one of the three is set; the host refuses
// anything else with CodeInvalid. All three are always in the JSON.
type YoloSet struct {
	Minutes int  `json:"minutes"`
	Session bool `json:"session"`
	Off     bool `json:"off"`
}

// YoloState is YOLO as the host has it. Until is epoch milliseconds of the
// host's clock (0 when off or until the session ends).
type YoloState struct {
	Active  bool  `json:"active"`
	Until   int64 `json:"until"`
	Session bool  `json:"session"`
}
