package strx

import (
	"crypto/rand"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ToUpperFirst returns s with only the first Unicode letter mapped to its upper case.
func ToUpperFirst(s string) string {
	for i, v := range s {
		return string(unicode.ToUpper(v)) + s[i+1:]
	}
	return ""
}

// RandomChars returns a generated string in given number of random characters.
func RandomChars(n int) (string, error) {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	randomInt := func(max *big.Int) (int, error) {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return 0, err
		}

		return int(r.Int64()), nil
	}

	buffer := make([]byte, n)
	max := big.NewInt(int64(len(alphanum)))
	for i := range n {
		index, err := randomInt(max)
		if err != nil {
			return "", err
		}

		buffer[i] = alphanum[index]
	}

	return string(buffer), nil
}

// Ellipsis returns a truncated string and appends "..." to the end of the
// string if the string has more characters than the threshold. Otherwise, the
// original string is returned.
func Ellipsis(str string, threshold int) string {
	if threshold < 0 || utf8.RuneCountInString(str) <= threshold {
		return str
	}
	return Truncate(str, threshold) + "..."
}

// Truncate returns the string cut to at most limit characters, splitting it
// only between characters so the result stays valid UTF-8, which database
// columns and HTML output require. Otherwise, the original string is returned.
func Truncate(str string, limit int) string {
	char := 0
	for i := range str {
		if char == limit {
			return str[:i]
		}
		char++
	}
	return str
}

// ContainsFold reports whether s is within the slice, ignoring case.
func ContainsFold(ss []string, s string) bool {
	for _, v := range ss {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
