package emutest

import "testing"

func TestRecordFlags(t *testing.T) {
	const head = "NotificationRecord(0x1 pkg=x): Notification(channel=c shortcut=null contentView=null vibrate=null sound=null defaults=0x0 "
	cases := []struct {
		name string
		rec  string
		want int64
	}{
		{"hex with local only", head + "flags=0x1a2)", 0x1a2},
		{"hex without local only", head + "flags=0x10 color=0)", 0x10},
		{"names with local only", head + "flags=ONGOING_EVENT|LOCAL_ONLY|SILENT color=0)", 0x2 | 0x100 | 0x2000},
		{"names without local only", head + "flags=AUTO_CANCEL|CAN_COLORIZE)", 0x10 | 0x800},
		{"unknown name ignored", head + "flags=AUTO_CANCEL|WHATEVER|LOCAL_ONLY)", 0x10 | 0x100},
		{"only unknown names", head + "flags=WHATEVER)", 0},
		{"no field", head + "color=0)", -1},
		{"bad hex", head + "flags=0xzz)", -1},
	}
	for _, c := range cases {
		got := recordFlags(c.rec)
		if got != c.want {
			t.Errorf("%s: recordFlags = %#x, want %#x", c.name, got, c.want)
		}
		if (got >= 0 && got&flagLocalOnly != 0) != (c.want >= 0 && c.want&flagLocalOnly != 0) {
			t.Errorf("%s: local-only mismatch", c.name)
		}
	}
}
