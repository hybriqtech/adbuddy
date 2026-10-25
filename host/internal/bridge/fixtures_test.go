package bridge

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// fixtureDir holds the protocol's shared fixtures (protocol spec §12), read
// by these tests and by the app's FixturesTest.
const fixtureDir = "../../../protocol/fixtures"

var directions = map[string]map[string]func() any{"host": HostPayloads, "app": AppPayloads}

// strictDecode decodes data into v, failing on any field v does not have.
func strictDecode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// sameJSON reports whether a and b are the same JSON value.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}

// fixtures returns the fixture files of a direction (host or app) by name:
// "<type>" for the base file, "<type>@<variant>" for the others.
func fixtures(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtureDir, dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no fixtures in %s", filepath.Join(fixtureDir, dir))
	}
	out := map[string]string{}
	for _, f := range files {
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = f
	}
	return out
}

// readFixture reads a fixture's envelope strictly.
func readFixture(t *testing.T, path string) Message {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m Message
	if err := strictDecode(data, &m); err != nil {
		t.Fatalf("%s: envelope: %v", path, err)
	}
	return m
}

// Each fixture decodes strictly into its type's payload, and the base file
// of each type comes out the same when encoded again: a field renamed or
// dropped on either side breaks this test or the app's.
func TestFixtures(t *testing.T) {
	for dir, payloads := range directions {
		for name, path := range fixtures(t, dir) {
			t.Run(dir+"/"+name, func(t *testing.T) {
				m := readFixture(t, path)
				typ, variant, _ := strings.Cut(name, "@")
				if m.Type != typ || m.V != ProtocolVersion {
					t.Fatalf("type %q v%d in %s", m.Type, m.V, name)
				}
				if m.RID != "" && !ValidRID(m.RID) {
					t.Fatalf("rid %q", m.RID)
				}
				newPayload, ok := payloads[typ]
				if !ok {
					t.Fatalf("%s is not a type the %s sends", typ, dir)
				}
				v := newPayload()
				if err := strictDecode(m.Payload, v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				if variant != "" {
					return
				}
				again, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				if !sameJSON(t, again, m.Payload) {
					t.Fatalf("encoded again:\n%s\nfixture:\n%s", again, m.Payload)
				}
			})
		}
	}
}

// typePaths adds the JSON paths of t's fields ("attachment.thumb"): lists are
// transparent, and time.Time, byte slices (base64, json.RawMessage) and maps
// are leaves.
func typePaths(t reflect.Type, prefix string, out map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8 {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeOf(time.Time{}) {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[prefix+name] = true
		typePaths(f.Type, prefix+name+".", out)
	}
}

// jsonPaths adds the paths of the keys in v, a decoded JSON value.
func jsonPaths(v any, prefix string, out map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			out[prefix+k] = true
			jsonPaths(x, prefix+k+".", out)
		}
	case []any:
		for _, x := range v {
			jsonPaths(x, prefix, out)
		}
	}
}

// Every field of every payload appears in at least one fixture of its type:
// a field added to one side only would otherwise go unseen, since an absent
// optional field is encoded again as absent.
func TestFixturesHaveEveryField(t *testing.T) {
	for dir, payloads := range directions {
		present := map[string]map[string]bool{}
		for name, path := range fixtures(t, dir) {
			typ, _, _ := strings.Cut(name, "@")
			if present[typ] == nil {
				present[typ] = map[string]bool{}
			}
			var v any
			if err := json.Unmarshal(readFixture(t, path).Payload, &v); err != nil {
				t.Fatal(err)
			}
			jsonPaths(v, "", present[typ])
		}
		for typ, newPayload := range payloads {
			want := map[string]bool{}
			typePaths(reflect.TypeOf(newPayload()), "", want)
			var missing []string
			for p := range want {
				if !present[typ][p] {
					missing = append(missing, p)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%s/%s: no fixture has %v", dir, typ, missing)
			}
		}
	}
}

// Every type has a base fixture, and every fixture a type.
func TestEveryTypeHasAFixture(t *testing.T) {
	for dir, payloads := range directions {
		files := fixtures(t, dir)
		var missing []string
		for typ := range payloads {
			if _, ok := files[typ]; !ok {
				missing = append(missing, typ)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s types without a fixture: %v", dir, missing)
		}
		for name := range files {
			typ, _, _ := strings.Cut(name, "@")
			if _, ok := payloads[typ]; !ok {
				t.Errorf("fixture %s/%s has no type", dir, name)
			}
		}
	}
}

// The host and the app send different types: a type in both lists would be
// ambiguous in the fixtures.
func TestDirectionsDoNotShareTypes(t *testing.T) {
	for typ := range HostPayloads {
		if _, ok := AppPayloads[typ]; ok {
			t.Errorf("%s is in both directions", typ)
		}
	}
}

// Each host fixture is what its class sends (protocol spec §3): events are
// numbered, replies carry a rid and no number, stream messages neither.
func TestFixtureClasses(t *testing.T) {
	for typ := range HostPayloads {
		if HostClasses[typ] == 0 {
			t.Errorf("%s has no class", typ)
		}
	}
	for name, path := range fixtures(t, "host") {
		m := readFixture(t, path)
		switch HostClasses[m.Type] {
		case ClassEvent:
			if m.Seq == 0 {
				t.Errorf("%s: an event without seq", name)
			}
		case ClassReply:
			if m.Seq != 0 || m.RID == "" {
				t.Errorf("%s: a reply has a rid and no seq", name)
			}
		case ClassStream, ClassHandshake:
			if m.Seq != 0 || m.RID != "" {
				t.Errorf("%s: neither seq nor rid", name)
			}
		}
	}
	for name, path := range fixtures(t, "app") {
		if m := readFixture(t, path); m.Seq != 0 {
			t.Errorf("app/%s: the app does not number its messages", name)
		}
	}
}

// Every Type* constant of protocol.go is in a registry: one added without a
// payload would have no fixture.
func TestEveryTypeConstantIsRegistered(t *testing.T) {
	f, err := parser.ParseFile(gotoken.NewFileSet(), "protocol.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, id := range spec.Names {
			if !strings.HasPrefix(id.Name, "Type") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != gotoken.STRING {
				continue
			}
			typ, _ := strconv.Unquote(lit.Value)
			n++
			_, host := HostPayloads[typ]
			_, app := AppPayloads[typ]
			if !host && !app {
				t.Errorf("%s (%q) is in no registry", id.Name, typ)
			}
		}
		return true
	})
	if n != len(HostPayloads)+len(AppPayloads) {
		t.Errorf("%d Type constants for %d registered types", n, len(HostPayloads)+len(AppPayloads))
	}
}

type frameVectors struct {
	Valid []struct {
		ID     string `json:"id"`
		Offset int64  `json:"offset"`
		Data   string `json:"data"` // base64
		Frame  string `json:"frame"`
	} `json:"valid"`
	Invalid []struct {
		Why   string `json:"why"`
		Frame string `json:"frame"`
	} `json:"invalid"`
	BadIDs []string `json:"badIds"`
}

func TestFrameVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "frames.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v frameVectors
	if err := strictDecode(data, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Valid {
		id, err := ParseFrameID(c.ID)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := base64.StdEncoding.DecodeString(c.Data)
		frame, err := EncodeFrame(id, c.Offset, payload)
		if err != nil || hex.EncodeToString(frame) != c.Frame {
			t.Errorf("EncodeFrame(%s, %d, %q) = %x, %v; want %s", c.ID, c.Offset, payload, frame, err, c.Frame)
		}
		raw, _ := hex.DecodeString(c.Frame)
		gotID, gotOff, gotData, err := DecodeFrame(raw)
		if err != nil || gotID.String() != c.ID || gotOff != c.Offset || !bytes.Equal(gotData, payload) {
			t.Errorf("DecodeFrame(%s) = %s %d %q %v", c.Frame, gotID, gotOff, gotData, err)
		}
	}
	for _, c := range v.Invalid {
		raw, _ := hex.DecodeString(c.Frame)
		if _, _, _, err := DecodeFrame(raw); err == nil {
			t.Errorf("DecodeFrame accepted a frame with %s", c.Why)
		}
	}
	for _, s := range v.BadIDs {
		if _, err := ParseFrameID(s); err == nil {
			t.Errorf("ParseFrameID(%q) accepted it", s)
		}
	}
}

func TestFrameLimits(t *testing.T) {
	var id FrameID
	if _, err := EncodeFrame(id, 0, nil); err == nil {
		t.Error("a frame without data was encoded")
	}
	if _, err := EncodeFrame(id, -1, []byte("x")); err == nil {
		t.Error("a frame with a negative offset was encoded")
	}
	if _, err := EncodeFrame(id, 0, make([]byte, MaxFrameData+1)); err == nil {
		t.Error("a frame over 256 KiB was encoded")
	}
	frame, err := EncodeFrame(id, 0, make([]byte, MaxFrameData))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := DecodeFrame(frame); err != nil {
		t.Errorf("a 256 KiB frame was refused: %v", err)
	}
	if _, _, _, err := DecodeFrame(append(frame, 0)); err == nil {
		t.Error("a frame over 256 KiB was decoded")
	}
}

func TestValidRID(t *testing.T) {
	for _, rid := range []string{"r", "r_1", "rA9-x", strings.Repeat("r", MaxRID)} {
		if !ValidRID(rid) {
			t.Errorf("ValidRID(%q) = false", rid)
		}
	}
	for _, rid := range []string{"", strings.Repeat("r", MaxRID+1), "r 1", "r/1", "ré", "r\n"} {
		if ValidRID(rid) {
			t.Errorf("ValidRID(%q) = true", rid)
		}
	}
}

// HostPayloads maps each message type the host sends to a new value of its
// payload. Every type has a fixture in protocol/fixtures/host/, checked by
// TestFixtures (protocol spec §12).
var HostPayloads = map[string]func() any{
	TypeWelcome:          func() any { return new(Welcome) },
	TypeSnapshot:         func() any { return new(Snapshot) },
	TypeSessionState:     func() any { return new(SessionState) },
	TypeConfirmReq:       func() any { return new(ConfirmRequest) },
	TypeJournalEntry:     func() any { return new(journal.Entry) },
	TypeNotice:           func() any { return new(Notice) },
	TypeError:            func() any { return new(ErrorReply) },
	TypeChatMessage:      func() any { return new(ChatMessage) },
	TypeToolActivity:     func() any { return new(ToolActivity) },
	TypePermRequest:      func() any { return new(PermissionRequest) },
	TypePermResolved:     func() any { return new(PermissionResolved) },
	TypeAttachReady:      func() any { return new(AttachReady) },
	TypeAttachDone:       func() any { return new(AttachDone) },
	TypeFileEnd:          func() any { return new(FileEnd) },
	TypeCaptureReady:     func() any { return new(CaptureReady) },
	TypeTerminalSnapshot: func() any { return new(TerminalSnapshot) },
	TypeTerminalOutput:   func() any { return new(TerminalOutput) },
	TypeTerminalSize:     func() any { return new(TerminalSize) },
	TypeUIRequest:        func() any { return new(UIRequest) },
	TypeRemoteState:      func() any { return new(RemoteState) },
	TypeYoloState:        func() any { return new(YoloState) },
}

// AppPayloads maps each message type the app sends to a new value of its
// payload. Every type has a fixture in protocol/fixtures/app/.
var AppPayloads = map[string]func() any{
	TypeHello:           func() any { return new(Hello) },
	TypeConfirmReply:    func() any { return new(ConfirmReply) },
	TypePause:           func() any { return new(Empty) },
	TypeResume:          func() any { return new(Empty) },
	TypeChatSend:        func() any { return new(ChatSend) },
	TypeSessionNew:      func() any { return new(SessionNew) },
	TypeSessionStop:     func() any { return new(Empty) },
	TypeInterrupt:       func() any { return new(Empty) },
	TypePermReply:       func() any { return new(PermissionReply) },
	TypeAttachBegin:     func() any { return new(AttachBegin) },
	TypeAttachEnd:       func() any { return new(AttachEnd) },
	TypeFileGet:         func() any { return new(FileGet) },
	TypeFileCancel:      func() any { return new(FileCancel) },
	TypeCaptureReq:      func() any { return new(Empty) },
	TypeTerminalWatch:   func() any { return new(TerminalWatch) },
	TypeTerminalUnwatch: func() any { return new(Empty) },
	TypeTerminalInput:   func() any { return new(TerminalInput) },
	TypeTerminalResize:  func() any { return new(TerminalResize) },
	TypeUIResponse:      func() any { return new(UIResponse) },
	TypeDeviceInfo:      func() any { return new(DeviceInfo) },
	TypeRemoteSet:       func() any { return new(RemoteSet) },
	TypeYoloSet:         func() any { return new(YoloSet) },
}
