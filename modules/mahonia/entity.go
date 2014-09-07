package mahonia

// decoding HTML entities

import (
	"sort"
)

// EntityDecoder returns a Decoder that decodes HTML character entities.
// If there is no valid character entity at the current position, it returns INVALID_CHAR.
// So it needs to be combined with another Decoder via FallbackDecoder.
func EntityDecoder() Decoder {
	var leftover rune // leftover rune from two-rune entity
	return func(p []byte) (r rune, size int, status Status) {
		if leftover != 0 {
			r = leftover
			leftover = 0
			return r, 0, SUCCESS
		}

		if len(p) == 0 {
			return 0, 0, NO_ROOM
		}

		if p[0] != '&' {
			return 0xfffd, 1, INVALID_CHAR
		}

		if len(p) < 3 {
			return 0, 1, NO_ROOM
		}

		r, size, status = 0xfffd, 1, INVALID_CHAR
		n := 1 // number of bytes read so far

		if p[n] == '#' {
			n++
			c := p[n]
			hex := false
			if c == 'x' || c == 'X' {
				hex = true
				n++
			}

			var x rune
			for n < len(p) {
				c = p[n]
				n++
				if hex {
					if '0' <= c && c <= '9' {
						x = 16*x + rune(c) - '0'
						continue
					} else if 'a' <= c && c <= 'f' {
						x = 16*x + rune(c) - 'a' + 10
						continue
					} else if 'A' <= c && c <= 'F' {
						x = 16*x + rune(c) - 'A' + 10
						continue
					}
				} else if '0' <= c && c <= '9' {
					x = 10*x + rune(c) - '0'
					continue
				}
				if c != ';' {
					n--
				}
				break
			}

			if n == len(p) && p[n-1] != ';' {
				return 0, 0, NO_ROOM
			}

			size = n
			if p[n-1] == ';' {
				n--
			}
			if hex {
				n--
			}
			n--
			// Now n is the number of actual digits read.
			if n == 0 {
				return 0xfffd, 1, INVALID_CHAR
			}

			if 0x80 <= x && x <= 0x9F {
				// Replace characters from Windows-1252 with UTF-8 equivalents.
				x = replacementTable[x-0x80]
			} else if x == 0 || (0xD800 <= x && x <= 0xDFFF) || x > 0x10FFFF {
				// Replace invalid characters with the replacement character.
				return 0xfffd, size, INVALID_CHAR
			}

			r = x
			status = SUCCESS
			return
		}

		// Look for a named entity in EntityList.

		possible := entityList
		for len(possible) > 0 {
			if len(p) <= n {
				leftover = 0
				return 0, 0, NO_ROOM
			}

			c := p[n]

			// Narrow down the selection in possible to those items that have c in the
			// appropriate byte.
			first := sort.Search(len(possible), func(i int) bool {
				e := possible[i].name
				if len(e) < n {
					return false
				}
				return e[n-1] >= c
			})
			possible = possible[first:]
			last := sort.Search(len(possible), func(i int) bool {
				return possible[i].name[n-1] > c
			})
			possible = possible[:last]

			n++
			if len(possible) > 0 && len(possible[0].name) == n-1 {
				r, leftover = possible[0].r1, possible[0].r2
				size = n
				status = SUCCESS
				// but don't return yet, since we need the longest match
			}
		}

		return
	}
}

// This table is copied from /src/pkg/html/escape.go in the Go source
//
// These replacements permit compatibility with old numeric entities that
// assumed Windows-1252 encoding.
// http://www.whatwg.org/specs/web-apps/current-work/multipage/tokenization.html#consume-a-character-reference
var replacementTable = [...]rune{
	'\u20AC', // First entry is what 0x80 should be replaced with.
	'\u0081',
	'\u201A',
	'\u0192',
	'\u201E',
	'\u2026',
	'\u2020',
	'\u2021',
	'\u02C6',
	'\u2030',
	'\u0160',
	'\u2039',
	'\u0152',
	'\u008D',
	'\u017D',
	'\u008F',
	'\u0090',
	'\u2018',
	'\u2019',
	'\u201C',
	'\u201D',
	'\u2022',
	'\u2013',
	'\u2014',
	'\u02DC',
	'\u2122',
	'\u0161',
	'\u203A',
	'\u0153',
	'\u009D',
	'\u017E',
	'\u0178', // Last entry is 0x9F.
	// 0x00->'\uFFFD' is handled programmatically.
	// 0x0D->'\u000D' is a no-op.
}
