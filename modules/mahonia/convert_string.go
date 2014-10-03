package mahonia

import (
	"unicode/utf8"
)

// ConvertString converts a  string from UTF-8 to e's encoding.
func (e Encoder) ConvertString(s string) string {
	dest := make([]byte, len(s)+10)
	destPos := 0

	for _, rune := range s {
	retry:
		size, status := e(dest[destPos:], rune)

		if status == NO_ROOM {
			newDest := make([]byte, len(dest)*2)
			copy(newDest, dest)
			dest = newDest
			goto retry
		}

		if status == STATE_ONLY {
			destPos += size
			goto retry
		}

		destPos += size
	}

	return string(dest[:destPos])
}

// ConvertString converts a string from d's encoding to UTF-8.
func (d Decoder) ConvertString(s string) string {
	bytes := []byte(s)
	runes := make([]rune, len(s))
	destPos := 0

	for len(bytes) > 0 {
		c, size, status := d(bytes)

		if status == STATE_ONLY {
			bytes = bytes[size:]
			continue
		}

		if status == NO_ROOM {
			c = 0xfffd
			size = len(bytes)
			status = INVALID_CHAR
		}

		bytes = bytes[size:]
		runes[destPos] = c
		destPos++
	}

	return string(runes[:destPos])
}

// ConvertStringOK converts a  string from UTF-8 to e's encoding. It also
// returns a boolean indicating whether every character was converted
// successfully.
func (e Encoder) ConvertStringOK(s string) (result string, ok bool) {
	dest := make([]byte, len(s)+10)
	destPos := 0
	ok = true

	for i, r := range s {
		// The following test is copied from utf8.ValidString.
		if r == utf8.RuneError && ok {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				ok = false
			}
		}

	retry:
		size, status := e(dest[destPos:], r)

		switch status {
		case NO_ROOM:
			newDest := make([]byte, len(dest)*2)
			copy(newDest, dest)
			dest = newDest
			goto retry

		case STATE_ONLY:
			destPos += size
			goto retry

		case INVALID_CHAR:
			ok = false
		}

		destPos += size
	}

	return string(dest[:destPos]), ok
}

// ConvertStringOK converts a string from d's encoding to UTF-8.
// It also returns a boolean indicating whether every character was converted
// successfully.
func (d Decoder) ConvertStringOK(s string) (result string, ok bool) {
	bytes := []byte(s)
	runes := make([]rune, len(s))
	destPos := 0
	ok = true

	for len(bytes) > 0 {
		c, size, status := d(bytes)

		switch status {
		case STATE_ONLY:
			bytes = bytes[size:]
			continue

		case NO_ROOM:
			c = 0xfffd
			size = len(bytes)
			ok = false

		case INVALID_CHAR:
			ok = false
		}

		bytes = bytes[size:]
		runes[destPos] = c
		destPos++
	}

	return string(runes[:destPos]), ok
}
