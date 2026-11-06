package strx

import (
	"crypto/rand"
	"math/big"
	"strings"
	"unicode"
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
// string if the string length is larger than the threshold. Otherwise, the
// original string is returned.
func Ellipsis(str string, threshold int) string {
	if len(str) <= threshold || threshold < 0 {
		return str
	}
	return str[:threshold] + "..."
}

// Truncate returns a truncated string if its length is over the limit.
// Otherwise, it returns the original string.
func Truncate(str string, limit int) string {
	if len(str) < limit {
		return str
	}
	return str[:limit]
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
