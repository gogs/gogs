// This package is a character-set conversion library for Go.
//
// (DEPRECATED: use code.google.com/p/go.text/encoding, perhaps along with
// code.google.com/p/go.net/html/charset)
package mahonia

import (
	"bytes"
	"unicode"
)

// Status is the type for the status return value from a Decoder or Encoder.
type Status int

const (
	// SUCCESS means that the character was converted with no problems.
	SUCCESS = Status(iota)

	// INVALID_CHAR means that the source contained invalid bytes, or that the character
	// could not be represented in the destination encoding.
	// The Encoder or Decoder should have output a substitute character.
	INVALID_CHAR

	// NO_ROOM means there were not enough input bytes to form a complete character,
	// or there was not enough room in the output buffer to write a complete character.
	// No bytes were written, and no internal state was changed in the Encoder or Decoder.
	NO_ROOM

	// STATE_ONLY means that bytes were read or written indicating a state transition,
	// but no actual character was processed. (Examples: byte order marks, ISO-2022 escape sequences)
	STATE_ONLY
)

// A Decoder is a function that decodes a character set, one character at a time.
// It works much like utf8.DecodeRune, but has an aditional status return value.
type Decoder func(p []byte) (c rune, size int, status Status)

// An Encoder is a function that encodes a character set, one character at a time.
// It works much like utf8.EncodeRune, but has an additional status return value.
type Encoder func(p []byte, c rune) (size int, status Status)

// A Charset represents a character set that can be converted, and contains functions
// to create Converters to encode and decode strings in that character set.
type Charset struct {
	// Name is the character set's canonical name.
	Name string

	// Aliases returns a list of alternate names.
	Aliases []string

	// NewDecoder returns a Decoder to convert from the charset to Unicode.
	NewDecoder func() Decoder

	// NewEncoder returns an Encoder to convert from Unicode to the charset.
	NewEncoder func() Encoder
}

// The charsets are stored in charsets under their canonical names.
var charsets = make(map[string]*Charset)

// aliases maps their aliases to their canonical names.
var aliases = make(map[string]string)

// simplifyName converts a name to lower case and removes non-alphanumeric characters.
// This is how the names are used as keys to the maps.
func simplifyName(name string) string {
	var buf bytes.Buffer
	for _, c := range name {
		switch {
		case unicode.IsDigit(c):
			buf.WriteRune(c)
		case unicode.IsLetter(c):
			buf.WriteRune(unicode.ToLower(c))
		default:

		}
	}

	return buf.String()
}

// RegisterCharset adds a charset to the charsetMap.
func RegisterCharset(cs *Charset) {
	name := cs.Name
	charsets[name] = cs
	aliases[simplifyName(name)] = name
	for _, alias := range cs.Aliases {
		aliases[simplifyName(alias)] = name
	}
}

// GetCharset fetches a charset by name.
// If the name is not found, it returns nil.
func GetCharset(name string) *Charset {
	return charsets[aliases[simplifyName(name)]]
}

// NewDecoder returns a Decoder to decode the named charset.
// If the name is not found, it returns nil.
func NewDecoder(name string) Decoder {
	cs := GetCharset(name)
	if cs == nil {
		return nil
	}
	return cs.NewDecoder()
}

// NewEncoder returns an Encoder to encode the named charset.
func NewEncoder(name string) Encoder {
	cs := GetCharset(name)
	if cs == nil {
		return nil
	}
	return cs.NewEncoder()
}
