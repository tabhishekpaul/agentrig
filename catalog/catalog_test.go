package catalog

import "testing"

func TestClassify(t *testing.T) {
	for in, want := range map[string]string{
		"ffmpeg helpers for trimming video clips": "media/video",
		"React and Svelte frontend patterns":      "coding/frontend",
		"Fill and merge PDF forms":                "documents/pdf",
		"nothing matches here":                    "other",
	} {
		if got := Classify("", in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
	if Classify("build-tool", "") == "design/ui" {
		t.Error(`"ui" must not match inside "build"`)
	}
}

func TestSearch(t *testing.T) {
	es := []Entry{
		{ID: "pdf", Name: "pdf", Description: "read and write PDF files"},
		{ID: "video-editor", Name: "video-editor", Description: "cut and trim video"},
		{ID: "react", Name: "react", Description: "React patterns"},
	}
	got := Search(es, "trim a vid", 0)
	if len(got) == 0 || got[0].ID != "video-editor" {
		t.Fatalf("got %+v", got)
	}
	if len(Search(es, "", 0)) != 0 {
		t.Error("empty query should return nothing")
	}
}

func TestValid(t *testing.T) {
	if !Valid("media") || !Valid("media/video") || Valid("media/nope") || Valid("") {
		t.Error("Valid wrong")
	}
}
