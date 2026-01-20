package markup

import (
	"strings"
	"testing"
)

func TestPostProcessHTML_ClosesUnclosedTags(t *testing.T) {
	input := []byte("<div><p>hello")
	out := postProcessHTML(input, "/", nil)
	s := string(out)

	if !strings.HasSuffix(s, "</p></div>") {
		t.Fatalf("expected output to end with closing tags </p></div>, got: %q", s)
	}
}

func TestPostProcessHTML_MisnestedEndTagClosesInterveningTags(t *testing.T) {
	// Start div, start span, then end div -> should close span before div
	input := []byte("<div><span>foo</div>")
	out := postProcessHTML(input, "/", nil)
	s := string(out)

	if !strings.Contains(s, "</span></div>") {
		t.Fatalf("expected intervening <span> to be closed before </div>, got: %q", s)
	}

	// ensure we did not produce an incorrect order like </div></span>
	if strings.Contains(s, "</div></span>") {
		t.Fatalf("unexpected incorrect closing order found in output: %q", s)
	}
}

func TestPostProcessHTML_NoEndTags_AreNotClosed(t *testing.T) {
	// 'sup' was added to noEndTags: it should not be pushed to stack and therefore not closed.
	input := []byte("<div><sup>foo</div>")
	out := postProcessHTML(input, "/", nil)
	s := string(out)

	// Expect only the div to be closed (sup being in noEndTags should not produce a closing </sup>)
	if !strings.HasSuffix(s, "</div>") {
		t.Fatalf("expected output to end with </div>, got: %q", s)
	}
	if strings.Contains(s, "</sup>") {
		t.Fatalf("did not expect </sup> to be emitted for a tag in noEndTags, got: %q", s)
	}
}