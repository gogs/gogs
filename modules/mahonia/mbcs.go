package mahonia

// Generic converters for multibyte character sets.

// An mbcsTrie contains the data to convert from the character set to Unicode.
// If a character would be encoded as "\x01\x02\x03", its unicode value would be found at t.children[1].children[2].children[3].rune
// children either is nil or has 256 elements.
type mbcsTrie struct {
	// For leaf nodes, the Unicode character that is represented.
	char rune

	// For non-leaf nodes, the trie to decode the remainder of the character.
	children []mbcsTrie
}

// A MBCSTable holds the data to convert to and from Unicode.
type MBCSTable struct {
	toUnicode   mbcsTrie
	fromUnicode map[rune]string
}

// AddCharacter adds a character to the table. rune is its Unicode code point,
// and bytes contains the bytes used to encode it in the character set.
func (table *MBCSTable) AddCharacter(c rune, bytes string) {
	if table.fromUnicode == nil {
		table.fromUnicode = make(map[rune]string)
	}

	table.fromUnicode[c] = bytes

	trie := &table.toUnicode
	for i := 0; i < len(bytes); i++ {
		if trie.children == nil {
			trie.children = make([]mbcsTrie, 256)
		}

		b := bytes[i]
		trie = &trie.children[b]
	}

	trie.char = c
}

func (table *MBCSTable) Decoder() Decoder {
	return func(p []byte) (c rune, size int, status Status) {
		if len(p) == 0 {
			status = NO_ROOM
			return
		}

		if p[0] == 0 {
			return 0, 1, SUCCESS
		}

		trie := &table.toUnicode
		for trie.char == 0 {
			if trie.children == nil {
				return 0xfffd, 1, INVALID_CHAR
			}
			if len(p) < size+1 {
				return 0, 0, NO_ROOM
			}

			trie = &trie.children[p[size]]
			size++
		}

		c = trie.char
		status = SUCCESS
		return
	}
}

func (table *MBCSTable) Encoder() Encoder {
	return func(p []byte, c rune) (size int, status Status) {
		bytes := table.fromUnicode[c]
		if bytes == "" {
			if len(p) > 0 {
				p[0] = '?'
				return 1, INVALID_CHAR
			} else {
				return 0, NO_ROOM
			}
		}

		if len(p) < len(bytes) {
			return 0, NO_ROOM
		}

		return copy(p, bytes), SUCCESS
	}
}
