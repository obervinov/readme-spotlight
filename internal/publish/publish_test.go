package publish

import "testing"

func TestMergeReplacesBetweenMarkers(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	content := "intro\n" + start + "\nold body\n" + end + "\noutro\n"
	got := Merge(content, start, end, "NEW")

	want := "intro\n" + start + "\nNEW\n" + end + "\noutro\n"
	if got != want {
		t.Fatalf("merge replaced wrong region:\n got: %q\nwant: %q", got, want)
	}
}

func TestMergeAppendsWhenMarkersMissing(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	got := Merge("hello\n", start, end, "BODY")

	want := "hello\n\n" + start + "\nBODY\n" + end + "\n"
	if got != want {
		t.Fatalf("merge did not append cleanly:\n got: %q\nwant: %q", got, want)
	}
}

func TestMergeIsIdempotent(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	once := Merge("readme\n", start, end, "BODY")
	twice := Merge(once, start, end, "BODY")
	if once != twice {
		t.Fatalf("merge not idempotent:\n once:  %q\n twice: %q", once, twice)
	}
}
