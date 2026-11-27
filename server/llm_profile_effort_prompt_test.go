package server

import (
	"testing"

	"github.com/3169417664/kaiyang/db"
)

func TestResolveEffortPrecedence(t *testing.T) {
	prof := &db.LLMProfile{ID: 1, ReasoningEffort: "low"}
	other := &db.LLMProfile{ID: 2, ReasoningEffort: "max"}
	overrides := map[int64]string{1: "high"}

	if got, src := ResolveEffort("worker", prof, overrides); got != "high" || src != EffortSourceOverride {
		t.Fatalf("override: got %q/%q, want high/override", got, src)
	}
	if got, src := ResolveEffort("planner", prof, nil); got != "low" || src != EffortSourceProfile {
		t.Fatalf("profile: got %q/%q, want low/profile", got, src)
	}
	// A failover target resolves against itself; it must not inherit the previous
	// profile's override entry.
	if got, src := ResolveEffort("worker", other, overrides); got != "max" || src != EffortSourceProfile {
		t.Fatalf("target profile: got %q/%q, want max/profile", got, src)
	}
	// Empty agent key (pin/judge/env) ignores overrides entirely.
	if got, src := ResolveEffort("", prof, overrides); got != "low" || src != EffortSourceProfile {
		t.Fatalf("no agent: got %q/%q, want low/profile", got, src)
	}
	unset := &db.LLMProfile{ID: 3, ReasoningEffort: "bogus"}
	if got, src := ResolveEffort("worker", unset, nil); got != "" || src != EffortSourceUnset {
		t.Fatalf("invalid profile effort: got %q/%q, want empty/unset", got, src)
	}
	if got, src := ResolveEffort("worker", nil, overrides); got != "" || src != EffortSourceUnset {
		t.Fatalf("nil profile: got %q/%q, want empty/unset", got, src)
	}
}

func TestNormalizeProfilePromptRequestThreeState(t *testing.T) {
	old := db.PromptInfo{Text: "old", Mode: "append", ApplyInherited: true}
	str := func(s string) *string { return &s }
	boolean := func(b bool) *bool { return &b }

	// nil pointers keep the stored value.
	got, errMsg := normalizeProfilePromptRequest(old, nil, nil, nil)
	if errMsg != "" || got != old {
		t.Fatalf("keep: got %+v err=%q", got, errMsg)
	}
	// A stored mode survives a text-only update; when neither carries a mode the
	// save path defaults a non-empty prompt to prepend.
	got, errMsg = normalizeProfilePromptRequest(old, str("new text\r\n"), nil, nil)
	if errMsg != "" || got.Text != "new text" || got.Mode != "append" || !got.ApplyInherited {
		t.Fatalf("keep mode: got %+v err=%q", got, errMsg)
	}
	got, errMsg = normalizeProfilePromptRequest(db.PromptInfo{ApplyInherited: true}, str("fresh"), nil, nil)
	if errMsg != "" || got.Text != "fresh" || got.Mode != "prepend" || !got.ApplyInherited {
		t.Fatalf("default mode: got %+v err=%q", got, errMsg)
	}
	// Explicit empty text clears both text and mode.
	got, errMsg = normalizeProfilePromptRequest(old, str(""), nil, nil)
	if errMsg != "" || got.Text != "" || got.Mode != "" || !got.ApplyInherited {
		t.Fatalf("clear: got %+v err=%q", got, errMsg)
	}
	// Unknown mode is rejected.
	if _, errMsg = normalizeProfilePromptRequest(old, str("x"), str("sideways"), nil); errMsg == "" {
		t.Fatal("unknown mode must be rejected")
	}
	// Over-long plain text is rejected.
	long := make([]rune, maxProfilePromptRunes+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, errMsg = normalizeProfilePromptRequest(old, str(string(long)), nil, nil); errMsg == "" {
		t.Fatal("over-long prompt must be rejected")
	}
	// Apply flag can be cleared independently.
	got, errMsg = normalizeProfilePromptRequest(old, nil, nil, boolean(false))
	if errMsg != "" || got.ApplyInherited {
		t.Fatalf("apply clear: got %+v err=%q", got, errMsg)
	}
}

func TestStaticProfileAvailability(t *testing.T) {
	ok := &db.LLMProfile{Name: "p", Format: "openai", Model: "m"}
	if avail, _ := staticProfileAvailability(ok, true); avail != "ok" {
		t.Fatalf("complete profile: %q", avail)
	}
	if avail, _ := staticProfileAvailability(ok, false); avail != "missing_key" {
		t.Fatalf("keyless profile: %q", avail)
	}
	if avail, _ := staticProfileAvailability(&db.LLMProfile{Format: "openai", Model: ""}, true); avail != "invalid_config" {
		t.Fatalf("empty model: %q", avail)
	}
	if avail, _ := staticProfileAvailability(&db.LLMProfile{Format: "nope", Model: "m"}, true); avail != "invalid_config" {
		t.Fatalf("unknown format: %q", avail)
	}
	if avail, _ := staticProfileAvailability(&db.LLMProfile{Format: "openai", Model: "m", BaseURL: "http://[bad"}, true); avail != "invalid_config" {
		t.Fatalf("bad base url: %q", avail)
	}
}
