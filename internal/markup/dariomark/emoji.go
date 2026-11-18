package dariomark

import "bytes"

// emojiText replaces emoji shortcodes, e.g., ":tada:", in the text of one
// HTML text node, which may be passed in parts, the way GitHub does. A
// shortcode is a name of ASCII letters, digits, "_", "+" and "-" between
// colons. The first shortcode of a text node decides for the whole node: when
// it follows a space, or only ASCII whitespace since the start of the node,
// every shortcode of the node with a name in names is replaced, wherever it
// is, e.g., the "b" in "a:b:c" after it. Otherwise none is, e.g., in
// "10:30:45 :tada:" or "x\t:tada:". Shortcodes do not overlap: a replaced
// one ends at its closing colon, and the closing colon of another can open
// the next one, e.g., ":zz:tada:" ends with an emoji.
type emojiText struct {
	names map[string]string
	state emojiState
	// blank reports whether the node so far is ASCII whitespace.
	blank bool
	// prev is the last byte of the node so far.
	prev byte
}

type emojiState int8

const (
	emojiUndecided emojiState = iota
	emojiOn
	emojiOff
)

// maxEmojiName is the length from which names are not looked up, longer than
// any name in Options.Emoji.
const maxEmojiName = 64

// startNode starts a new text node.
func (e *emojiText) startNode() {
	e.state = emojiUndecided
	e.blank = true
	e.prev = 0
}

// write writes text, the next part of the current text node, to out with
// shortcodes replaced.
func (e *emojiText) write(out *outBuf, text []byte) {
	if len(text) == 0 {
		return
	}
	if e.names == nil || e.state == emojiOff || bytes.IndexByte(text, ':') < 0 {
		e.wrote(text)
		out.Write(text)
		return
	}
	last := 0
	for i := 0; i < len(text); i++ {
		if text[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(text) && isEmojiNameChar(text[j]) {
			j++
		}
		if j == i+1 || j == len(text) || text[j] != ':' {
			// The next colon may open a shortcode.
			i = j - 1
			continue
		}
		if e.state == emojiUndecided {
			e.wrote(text[:i])
			if !e.blank && e.prev != ' ' {
				e.state = emojiOff
				break
			}
			e.state = emojiOn
		}
		name := text[i+1 : j]
		if len(name) <= maxEmojiName {
			if emoji, ok := e.names[string(name)]; ok {
				out.Write(text[last:i])
				out.WriteString(emoji)
				last = j + 1
				i = j
				continue
			}
		}
		// The closing colon may open the next shortcode.
		i = j - 1
	}
	e.wrote(text[last:])
	out.Write(text[last:])
}

// wrote updates the state of an undecided node with b, the next text in it.
func (e *emojiText) wrote(b []byte) {
	if len(b) == 0 || e.state != emojiUndecided {
		return
	}
	e.blank = e.blank && isBlank(b)
	e.prev = b[len(b)-1]
}

func isBlank(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\v', '\f', '\r':
		default:
			return false
		}
	}
	return true
}

func isEmojiNameChar(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '_' || c == '+' || c == '-'
}
