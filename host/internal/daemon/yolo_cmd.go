package daemon

import "github.com/hybriqtech/adbuddy/host/internal/ipc"

// yoloCmd answers `adbuddy yolo off|status`. It has no "on": the IPC is what
// the agent reaches (the shim and the CLI), and a prompt injection must not be
// able to take its own brakes off. Only the phone app turns YOLO on, by the
// bridge's authenticated channel (cli/phone_yolo.go).
func (d *Daemon) yoloCmd(req ipc.Request, s Stream) ipc.Exit {
	if len(req.Argv) != 1 {
		return ipc.Exit{Code: 2, ErrorCode: "YOLO_INVALID", Error: "use yolo off or yolo status"}
	}
	switch req.Argv[0] {
	case "off":
		d.o.Yolo.Off(YoloOffCLI)
	case "status":
	default:
		return ipc.Exit{Code: ExitDenied, ErrorCode: "YOLO_ONLY_FROM_THE_PHONE", Error: "YOLO can only be turned on from the phone app"}
	}
	st := d.o.Yolo.State()
	out := ipc.YoloStatus{Active: st.Active, Session: st.Session}
	if st.Active && !st.Session {
		out.Until = st.Until.UnixMilli()
	}
	_ = s.Data(out)
	return ipc.Exit{}
}
