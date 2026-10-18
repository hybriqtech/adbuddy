package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

func (e *env) yoloCall(t *testing.T, argv ...string) (ipc.YoloStatus, ipc.Exit) {
	t.Helper()
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindYolo, Argv: argv, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	var st ipc.YoloStatus
	if len(s.data) > 0 {
		if err := json.Unmarshal(s.data[0], &st); err != nil {
			t.Fatal(err)
		}
	}
	return st, exit
}

func TestYoloStatusAndOff(t *testing.T) {
	e := newEnv(t)
	if st, exit := e.yoloCall(t, "status"); exit.Code != 0 || st.Active {
		t.Fatalf("%+v %+v", st, exit)
	}
	if err := e.d.Yolo().Enable(YoloShort); err != nil {
		t.Fatal(err)
	}
	st, exit := e.yoloCall(t, "status")
	if exit.Code != 0 || !st.Active || st.Session || st.Until != e.d.Yolo().State().Until.UnixMilli() {
		t.Fatalf("%+v %+v", st, exit)
	}
	st, exit = e.yoloCall(t, "off")
	if exit.Code != 0 || st.Active || e.d.Yolo().Active() {
		t.Fatalf("%+v %+v", st, exit)
	}
	if j := e.last(t); j.Rule != "yolo.off" || j.Argv[1] != "cli" {
		t.Fatalf("entry %+v", j)
	}
}

// The agent cannot turn YOLO on: not by the IPC command, whatever it writes,
// and not by any other request. Mutation: add `case "on": d.o.Yolo.EnableSession()`
// to yoloCmd.
func TestTheAgentCannotTurnYoloOn(t *testing.T) {
	e := newEnv(t)
	for _, argv := range [][]string{
		{"on"}, {"ON"}, {"enable"}, {"session"}, {"15"}, {"--on"}, {"on", "15"}, {"yolo", "on"}, {"status", "on"}, {""},
	} {
		if _, exit := e.yoloCall(t, argv...); exit.Code == 0 || e.d.Yolo().Active() {
			t.Fatalf("%q: %+v, active %v", argv, exit, e.d.Yolo().Active())
		}
	}
	if _, exit := e.yoloCall(t, "on"); exit.ErrorCode != "YOLO_ONLY_FROM_THE_PHONE" {
		t.Fatalf("%+v", exit)
	}
	if _, exit := e.yoloCall(t); exit.ErrorCode != "YOLO_INVALID" {
		t.Fatalf("%+v", exit)
	}
	// No other kind of request turns it on, whatever its words.
	for _, kind := range []string{ipc.KindExec, ipc.KindUndo, ipc.KindPing, ipc.KindUI, ipc.KindRemote, ipc.KindHook, ipc.KindSend, ipc.KindSession, ipc.KindAttach, "yolo.set", "unknown"} {
		for _, argv := range [][]string{{"yolo", "on"}, {"on"}, {"shell", "yolo", "on"}} {
			s := &stream{}
			e.d.Handle(context.Background(), ipc.Request{Kind: kind, Argv: argv, Target: "on", UIOp: "yolo", Env: map[string]string{"ADBUDDY_SOURCE": "agent", "ADBUDDY_YOLO": "1"}}, s)
			if e.d.Yolo().Active() {
				t.Fatalf("kind %q %q turned YOLO on", kind, argv)
			}
		}
	}
}
