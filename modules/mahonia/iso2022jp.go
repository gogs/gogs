package mahonia

import (
	"unicode/utf8"
)

// converters for ISO-2022-JP encoding

const esc = 27

func init() {
	type jpEncoding int
	const (
		ascii jpEncoding = iota
		jisX0201Roman
		jisX0208
	)

	RegisterCharset(&Charset{
		Name: "ISO-2022-JP",
		NewDecoder: func() Decoder {
			encoding := ascii
			return func(p []byte) (c rune, size int, status Status) {
				if len(p) == 0 {
					return 0, 0, NO_ROOM
				}

				b := p[0]
				if b == esc {
					if len(p) < 3 {
						return 0, 0, NO_ROOM
					}
					switch p[1] {
					case '(':
						switch p[2] {
						case 'B':
							encoding = ascii
							return 0, 3, STATE_ONLY

						case 'J':
							encoding = jisX0201Roman
							return 0, 3, STATE_ONLY
						}

					case '$':
						switch p[2] {
						case '@', 'B':
							encoding = jisX0208
							return 0, 3, STATE_ONLY
						}
					}
				}

				switch encoding {
				case ascii:
					if b > 127 {
						return utf8.RuneError, 1, INVALID_CHAR
					}
					return rune(b), 1, SUCCESS

				case jisX0201Roman:
					if b > 127 {
						return utf8.RuneError, 1, INVALID_CHAR
					}
					switch b {
					case '\\':
						return 0xA5, 1, SUCCESS
					case '~':
						return 0x203E, 1, SUCCESS
					}
					return rune(b), 1, SUCCESS

				case jisX0208:
					return jis0208Table.DecodeLow(p)
				}
				panic("unreachable")
			}
		},
		NewEncoder: func() Encoder {
			jis0208Table.Reverse()
			encoding := ascii
			return func(p []byte, c rune) (size int, status Status) {
				if len(p) == 0 {
					return 0, NO_ROOM
				}

				if c < 128 {
					if encoding != ascii {
						if len(p) < 4 {
							return 0, NO_ROOM
						}
						p[0], p[1], p[2] = esc, '(', 'B'
						p[3] = byte(c)
						encoding = ascii
						return 4, SUCCESS
					}
					p[0] = byte(c)
					return 1, SUCCESS
				}

				if c > 65535 {
					return 0, INVALID_CHAR
				}
				jis := jis0208Table.FromUnicode[c]
				if jis == [2]byte{0, 0} && c != rune(jis0208Table.Data[0][0]) {
					return 0, INVALID_CHAR
				}

				if encoding != jisX0208 {
					if len(p) < 3 {
						return 0, NO_ROOM
					}
					p[0], p[1], p[2] = esc, '$', 'B'
					encoding = jisX0208
					return 3, STATE_ONLY
				}

				p[0] = jis[0] + 0x21
				p[1] = jis[1] + 0x21
				return 2, SUCCESS
			}
		},
	})
}
