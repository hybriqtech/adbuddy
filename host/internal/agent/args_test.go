package agent

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidModel(t *testing.T) {
	for _, s := range []string{"opus", "claude-sonnet-5-5", "opus[1m]", "claude-opus-5-5"} {
		if !ValidModel(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "-x", "--model", "a b", "a;b", "a\nb", strings.Repeat("a", 65), "$(x)"} {
		if ValidModel(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestValidEffort(t *testing.T) {
	for _, s := range []string{"low", "medium", "high", "xhigh", "max"} {
		if !ValidEffort(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"LOW", "", "ultra"} {
		if ValidEffort(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestArgs(t *testing.T) {
	if a, u := Args("claude", "", ""); a != nil || u != nil {
		t.Fatalf("%v %v", a, u)
	}
	if a, _ := Args("claude", "opus", "low"); !reflect.DeepEqual(a, []string{"--model", "opus", "--effort", "low"}) {
		t.Fatalf("%v", a)
	}
	if a, u := Args("codex", "x", ""); a != nil || !reflect.DeepEqual(u, []string{"model"}) {
		t.Fatalf("%v %v", a, u)
	}
}

func TestDescribe(t *testing.T) {
	if Describe("a", "low") != "model a, effort low" || Describe("a", "") != "model a" || Describe("", "low") != "effort low" || Describe("", "") != "" {
		t.Fatal("Describe")
	}
}
