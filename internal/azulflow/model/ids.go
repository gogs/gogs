package model

import (
	"strconv"
	"strings"
)

// maxIDLength is the length job and step IDs must stay under.
const maxIDLength = 100

// idBuilder checks IDs and generates unique ones, as GitHub does for steps
// without an ID and for matrix jobs. IDs are compared case-insensitively.
type idBuilder struct {
	name     strings.Builder
	distinct map[string]bool
}

func isIDLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIDDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func (b *idBuilder) add(id string) bool {
	if b.distinct == nil {
		b.distinct = map[string]bool{}
	}
	key := strings.ToLower(id)
	if b.distinct[key] {
		return false
	}
	b.distinct[key] = true
	return true
}

// appendSegment adds a segment to the ID being built, replacing characters
// that IDs may not contain with '_'.
func (b *idBuilder) appendSegment(value string) {
	if value == "" {
		return
	}
	if b.name.Len() == 0 {
		first := []rune(value)[0]
		if isIDDigit(first) || first == '-' {
			b.name.WriteByte('_')
		}
	} else {
		b.name.WriteByte('_')
	}
	for _, r := range value {
		switch {
		case isIDLetter(r), isIDDigit(r), r == '_', r == '-':
			b.name.WriteRune(r)
		case r >= 0x10000:
			// GitHub replaces each UTF-16 code unit.
			b.name.WriteString("__")
		default:
			b.name.WriteByte('_')
		}
	}
}

// build returns the ID built, made unique with a suffix such as "_2", and
// starts a new one.
func (b *idBuilder) build(allowReservedPrefix bool, maxLength int) string {
	original := b.name.String()
	if original == "" {
		original = "job"
	}
	b.name.Reset()
	if !allowReservedPrefix && strings.HasPrefix(original, "__") {
		original = "_" + strings.TrimLeft(original, "_")
	}
	for attempt := 1; attempt < 1000; attempt++ {
		suffix := ""
		if attempt > 1 {
			suffix = "_" + strconv.Itoa(attempt)
			if !allowReservedPrefix && original == "_" {
				suffix = strconv.Itoa(attempt)
			}
		}
		candidate := original
		if n := maxLength - len(suffix); len(candidate) > n {
			candidate = candidate[:n]
		}
		candidate += suffix
		if b.add(candidate) {
			return candidate
		}
	}
	throw("Unable to create a unique name")
	return ""
}

// addKnownID adds an ID from the workflow, and returns the error GitHub
// reports when it is not valid or not unique, or "".
func (b *idBuilder) addKnownID(id string) string {
	if !validID(id) || utf16Len(id) >= maxIDLength {
		return "The identifier '" + id + "' is invalid. IDs may only contain alphanumeric characters, '_', and '-'. IDs must start with a letter or '_' and and must be less than " + strconv.Itoa(maxIDLength) + " characters."
	}
	if strings.HasPrefix(id, "__") {
		return "The identifier '" + id + "' is invalid. IDs starting with '__' are reserved."
	}
	if !b.add(id) {
		return "The identifier '" + id + "' may not be used more than once within the same scope."
	}
	return ""
}

func validID(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range id {
		if !isIDLetter(r) && r != '_' && (i == 0 || (!isIDDigit(r) && r != '-')) {
			return false
		}
	}
	return true
}
