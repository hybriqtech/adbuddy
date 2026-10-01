package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// uiContract is protocol/fixtures/ui.json, which the app's tests read too:
// the operations, which operation each ui.response fixture answers, the
// error variants, the limits and package names to check ValidPackage with.
type uiContract struct {
	Ops       []string          `json:"ops"`
	Responses map[string]string `json:"responses"`
	Errors    []string          `json:"errors"`
	Limits    map[string]int    `json:"limits"`
	Packages  struct {
		Valid   []string `json:"valid"`
		Invalid []string `json:"invalid"`
	} `json:"packages"`
}

func readUIContract(t *testing.T) uiContract {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, "ui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c uiContract
	if err := strictDecode(data, &c); err != nil {
		t.Fatalf("ui.json: %v", err)
	}
	return c
}

// uiPart is the args or result of one ui.* fixture and the type they have.
type uiPart struct {
	name string
	raw  json.RawMessage
	typ  func() any
}

// uiParts reads every ui.request and ui.response fixture: a request's args
// have the type of its op, a response's result the type of the op ui.json
// gives it. An error variant has no part.
func uiParts(t *testing.T, c uiContract) []uiPart {
	t.Helper()
	isError := map[string]bool{}
	for _, e := range c.Errors {
		isError[e] = true
	}
	var parts []uiPart
	for _, dir := range []string{"host", "app"} {
		for name, path := range fixtures(t, dir) {
			typ, _, _ := strings.Cut(name, "@")
			m := readFixture(t, path)
			switch typ {
			case TypeUIRequest:
				var r UIRequest
				if err := strictDecode(m.Payload, &r); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				op, ok := UIOps[r.Op]
				if !ok {
					t.Fatalf("%s: unknown op %q", name, r.Op)
				}
				parts = append(parts, uiPart{name, r.Args, op.Args})
			case TypeUIResponse:
				var r UIResponse
				if err := strictDecode(m.Payload, &r); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if isError[name] {
					if r.OK || r.Result != nil || r.Error == nil || !knownCode(r.Error.Code) {
						t.Fatalf("%s: not an error with a known code: %s", name, m.Payload)
					}
					continue
				}
				op, ok := UIOps[c.Responses[name]]
				if !ok {
					t.Fatalf("%s is neither in ui.json's responses nor in its errors", name)
				}
				if !r.OK || r.Error != nil {
					t.Fatalf("%s: a result that is not ok", name)
				}
				parts = append(parts, uiPart{name, r.Result, op.Result})
			}
		}
	}
	return parts
}

// knownCode reports whether code is one of protocol.go's error codes.
func knownCode(code string) bool {
	switch code {
	case CodeUnsupported, CodeInvalid, CodeNotFound, CodeNoAgent, CodeBusy, CodeTooLarge,
		CodeChecksum, CodeFailed, CodeUnavailable, CodeProtected, CodeExpired, CodeAmbiguous:
		return true
	}
	return false
}

// The operations are the same as ui.json's, and every args and result
// decodes strictly into its type and comes out the same when encoded again.
func TestUIOps(t *testing.T) {
	c := readUIContract(t)
	var ops []string
	for op := range UIOps {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	want := append([]string{}, c.Ops...)
	sort.Strings(want)
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("UIOps %v, ui.json %v", ops, want)
	}
	for name := range c.Responses {
		if _, err := os.Stat(filepath.Join(fixtureDir, "app", name+".json")); err != nil {
			t.Errorf("ui.json names %s: %v", name, err)
		}
	}
	for _, p := range uiParts(t, c) {
		t.Run(p.name, func(t *testing.T) {
			v := p.typ()
			if err := strictDecode(p.raw, v); err != nil {
				t.Fatalf("decode: %v", err)
			}
			again, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, again, p.raw) {
				t.Fatalf("encoded again:\n%s\nfixture:\n%s", again, p.raw)
			}
		})
	}
}

// Every field of the args and results appears in some fixture of its type,
// and every operation has a request and a result fixture.
func TestUIOpsHaveEveryField(t *testing.T) {
	present := map[reflect.Type]map[string]bool{}
	for _, p := range uiParts(t, readUIContract(t)) {
		typ := reflect.TypeOf(p.typ())
		if present[typ] == nil {
			present[typ] = map[string]bool{}
		}
		var v any
		if err := json.Unmarshal(p.raw, &v); err != nil {
			t.Fatal(err)
		}
		jsonPaths(v, "", present[typ])
	}
	for op, o := range UIOps {
		for _, typ := range []reflect.Type{reflect.TypeOf(o.Args()), reflect.TypeOf(o.Result())} {
			got, ok := present[typ]
			if !ok {
				t.Errorf("%s: no fixture of %v", op, typ)
				continue
			}
			want := map[string]bool{}
			typePaths(typ, "", want)
			var missing []string
			for p := range want {
				if !got[p] {
					missing = append(missing, p)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%v: no fixture has %v", typ, missing)
			}
		}
	}
}

func TestUILimits(t *testing.T) {
	got := map[string]int{
		"maxUiResult":         MaxUIResult,
		"maxNotifications":    MaxNotifications,
		"maxNotificationText": MaxNotificationText,
		"maxSetText":          MaxSetText,
		"maxProtectedApps":    MaxProtectedApps,
		"maxPackage":          MaxPackage,
	}
	if c := readUIContract(t); !reflect.DeepEqual(got, c.Limits) {
		t.Fatalf("constants %v, ui.json %v", got, c.Limits)
	}
}

func TestValidPackage(t *testing.T) {
	c := readUIContract(t)
	for _, p := range c.Packages.Valid {
		if !ValidPackage(p) {
			t.Errorf("%q refused", p)
		}
	}
	for _, p := range c.Packages.Invalid {
		if ValidPackage(p) {
			t.Errorf("%q accepted", p)
		}
	}
}

// protectedApps absent is nil ("no change"); present and empty is a list.
func TestDeviceInfoProtectedApps(t *testing.T) {
	var absent, none DeviceInfo
	if err := strictDecode(readFixture(t, filepath.Join(fixtureDir, "app", "device.info@empty.json")).Payload, &absent); err != nil {
		t.Fatal(err)
	}
	if err := strictDecode(readFixture(t, filepath.Join(fixtureDir, "app", "device.info@protectednone.json")).Payload, &none); err != nil {
		t.Fatal(err)
	}
	if absent.ProtectedApps != nil || none.ProtectedApps == nil || len(none.ProtectedApps) != 0 {
		t.Fatalf("absent %#v, empty %#v", absent.ProtectedApps, none.ProtectedApps)
	}
}
