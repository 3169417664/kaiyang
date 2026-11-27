package agent

import (
	"context"
	"testing"
)

// withSkillIndex installs a deterministic skill-index block for one test and
// restores whatever the package had before (other tests/init may set it).
func withSkillIndex(t *testing.T, block string) {
	t.Helper()
	prev := SkillIndexBlock
	SkillIndexBlock = func(string) string { return block }
	t.Cleanup(func() { SkillIndexBlock = prev })
}

func TestAssembleSystemBodyWithoutOverlayIsUnchanged(t *testing.T) {
	withSkillIndex(t, "[skills]")
	for _, body := range []string{"base", "base\n\n[skills]"} {
		got := assembleSystemBody("worker", body, ProfilePrompt{})
		want := body + "\n\n[skills]"
		if got != want {
			t.Fatalf("body=%q: got %q, want %q", body, got, want)
		}
	}
}

func TestAssembleSystemBodyOverlayModes(t *testing.T) {
	withSkillIndex(t, "SKILLS")
	const body = "BODY"
	cases := []struct {
		name string
		pp   ProfilePrompt
		want string
	}{
		{"prepend", ProfilePrompt{Text: "PRE", Mode: "prepend"}, "PRE\n\nBODY\n\nSKILLS"},
		{"append", ProfilePrompt{Text: "POST", Mode: "append"}, "BODY\n\nSKILLS\n\nPOST"},
		{"replace", ProfilePrompt{Text: "REPL", Mode: "replace"}, "REPL\n\nSKILLS"},
		{"empty text", ProfilePrompt{Text: "", Mode: "replace"}, "BODY\n\nSKILLS"},
		{"unknown mode", ProfilePrompt{Text: "X", Mode: "sideways"}, "BODY\n\nSKILLS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assembleSystemBody("worker", body, tc.pp); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssembleSystemBodyReplaceKeepsSkillIndexOnly(t *testing.T) {
	withSkillIndex(t, "SKILLS")
	got := assembleSystemBody("planner", "BODY", ProfilePrompt{Text: "REPL", Mode: "replace"})
	if got != "REPL\n\nSKILLS" {
		t.Fatalf("replace dropped the skill index or body: %q", got)
	}
}

func TestProfilePromptContextRoundTrip(t *testing.T) {
	pp := ProfilePrompt{Text: "P", Mode: "append", ProfileID: 7, Source: "task_chain"}
	got := ProfilePromptFrom(WithProfilePrompt(context.Background(), pp))
	if got != pp {
		t.Fatalf("got %+v, want %+v", got, pp)
	}
	if (ProfilePromptFrom(nil)).Active() {
		t.Fatal("nil context must yield an inactive overlay")
	}
}
