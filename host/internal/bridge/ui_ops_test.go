package bridge

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// uiOpFixtures maps each ui.request and ui.response fixture to the type of its
// args or result (M5 spec §3); the error variants carry neither.
var uiOpFixtures = map[string]func() any{
	"host/ui.request":               func() any { return new(UIClickArgs) },
	"host/ui.request@dump":          func() any { return new(UIDumpArgs) },
	"host/ui.request@dumpall":       func() any { return new(UIDumpArgs) },
	"host/ui.request@clickid":       func() any { return new(UIClickArgs) },
	"host/ui.request@clickxy":       func() any { return new(UIClickArgs) },
	"host/ui.request@settext":       func() any { return new(UISetTextArgs) },
	"host/ui.request@foreground":    func() any { return new(Empty) },
	"host/ui.request@notifications": func() any { return new(Empty) },
	"app/ui.response":               func() any { return new(UIClicked) },
	"app/ui.response@dump":          func() any { return new(UIDump) },
	"app/ui.response@dumplocked":    func() any { return new(UIDump) },
	"app/ui.response@settext":       func() any { return new(UITextSet) },
	"app/ui.response@foreground":    func() any { return new(UIForeground) },
	"app/ui.response@notifications": func() any { return new(UINotifications) },
	"app/ui.response@error":         nil,
	"app/ui.response@ambiguous":     nil,
}

// uiOpPart returns the args of a ui.request fixture or the result of a
// ui.response one.
func uiOpPart(t *testing.T, key string) json.RawMessage {
	t.Helper()
	m := readFixture(t, filepath.Join(fixtureDir, key+".json"))
	if m.Type == TypeUIRequest {
		var r UIRequest
		if err := strictDecode(m.Payload, &r); err != nil {
			t.Fatal(err)
		}
		return r.Args
	}
	var r UIResponse
	if err := strictDecode(m.Payload, &r); err != nil {
		t.Fatal(err)
	}
	return r.Result
}

// Every ui.request and ui.response fixture is listed, and its args or result
// decode strictly into their type and come out the same when encoded again.
func TestUIOpFixtures(t *testing.T) {
	for _, dir := range []string{"host", "app"} {
		for name := range fixtures(t, dir) {
			typ, _, _ := strings.Cut(name, "@")
			if typ != TypeUIRequest && typ != TypeUIResponse {
				continue
			}
			if _, ok := uiOpFixtures[dir+"/"+name]; !ok {
				t.Errorf("%s/%s is not in uiOpFixtures", dir, name)
			}
		}
	}
	for key, newPart := range uiOpFixtures {
		t.Run(key, func(t *testing.T) {
			part := uiOpPart(t, key)
			if newPart == nil {
				if len(part) != 0 {
					t.Fatalf("an error variant with a result: %s", part)
				}
				return
			}
			v := newPart()
			if err := strictDecode(part, v); err != nil {
				t.Fatalf("decode: %v", err)
			}
			again, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, again, part) {
				t.Fatalf("encoded again:\n%s\nfixture:\n%s", again, part)
			}
		})
	}
}

// Every field of the args and results appears in some fixture of its type.
func TestUIOpFixturesHaveEveryField(t *testing.T) {
	present := map[reflect.Type]map[string]bool{}
	for key, newPart := range uiOpFixtures {
		if newPart == nil {
			continue
		}
		typ := reflect.TypeOf(newPart())
		if present[typ] == nil {
			present[typ] = map[string]bool{}
		}
		var v any
		if err := json.Unmarshal(uiOpPart(t, key), &v); err != nil {
			t.Fatal(err)
		}
		jsonPaths(v, "", present[typ])
	}
	for typ, got := range present {
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
