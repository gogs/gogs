package dariomark

import "bytes"

var alertKinds = [...]struct {
	// marker is lowercase.
	marker, class, title string
}{
	{},
	{"[!note]", "markdown-alert markdown-alert-note", "Note"},
	{"[!tip]", "markdown-alert markdown-alert-tip", "Tip"},
	{"[!important]", "markdown-alert markdown-alert-important", "Important"},
	{"[!warning]", "markdown-alert markdown-alert-warning", "Warning"},
	{"[!caution]", "markdown-alert markdown-alert-caution", "Caution"},
}

// alertType returns the alert kind whose marker is line, ignoring case and
// trailing whitespace, or 0.
func alertType(line []byte) int {
	line = bytes.TrimRight(line, " \t\r")
	for i := 1; i < len(alertKinds); i++ {
		if len(line) == len(alertKinds[i].marker) && hasPrefixFold(line, alertKinds[i].marker) {
			return i
		}
	}
	return 0
}

// finalizeAlert removes the marker line of an alert. A blockquote without
// content after the marker line, or whose marker line is not the first line
// of a paragraph, stays a blockquote.
func (p *parser) finalizeAlert(b *node) {
	first := b.first
	if first == nil || first.kind != kindParagraph || first.segEnd == first.segStart ||
		int(p.segs[first.segStart].start) != b.markerOffset {
		b.alert = 0
		return
	}
	if i := bytes.IndexByte(first.text, '\n'); i >= 0 {
		first.text = first.text[i+1:]
		first.segStart++
		return
	}
	if first.next == nil {
		b.alert = 0
		return
	}
	first.unlink()
}

func (r *renderer) alert(n *node) {
	s := r.s
	k := &alertKinds[n.alert]
	s.start("div", attr{"class", []byte(k.class), true})
	r.nl()
	s.start("p", attr{"class", []byte("markdown-alert-title"), true})
	s.text([]byte(k.title))
	s.end("p")
	r.nl()
	r.blocks(n)
	s.end("div")
	r.nl()
}
