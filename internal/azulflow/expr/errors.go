package expr

import (
	"strconv"
	"unicode/utf8"
)

// Error is a parse or evaluation error with GitHub's message.
type Error struct {
	// Msg is GitHub's message without location, e.g. "Unrecognized named-value: 'env'".
	Msg string
	// Pos is the 1-based position of the offending token, counted in UTF-16
	// code units from the start of the expression as GitHub does, or 0 when
	// the error has no position.
	Pos int
	// Expr is the source of the expression.
	Expr string

	cause error
}

// Error returns the message the way GitHub formats it. Only errors about a
// token of the expression have a location.
func (e *Error) Error() string {
	if e.Pos > 0 && e.Msg != msgExceededMaxDepth {
		return e.Msg + ". Located at position " + strconv.Itoa(e.Pos) + " within expression: " + e.Expr
	}
	return e.Msg
}

// Unwrap returns the underlying error of an evaluation error, if any.
func (e *Error) Unwrap() error {
	return e.cause
}

const (
	maxDepth  = 50
	maxLength = 21000
)

const (
	msgUnexpectedSymbol      = "Unexpected symbol"
	msgUnexpectedEnd         = "Unexpected end of expression"
	msgUnrecognizedNamed     = "Unrecognized named-value"
	msgUnrecognizedFunction  = "Unrecognized function"
	msgTooFewParameters      = "Too few parameters supplied"
	msgTooManyParameters     = "Too many parameters supplied"
	msgEvenParameters        = "Even number of parameters supplied, requires an odd number of parameters"
	msgExceededMaxDepth      = "Exceeded max expression depth 50"
	msgExceededMaxLength     = "Exceeded max expression length 21000"
	msgExceededAllowedMemory = "The maximum allowed memory size was exceeded while evaluating the following expression: "
)

// tokenError returns an error that names a token, as in "Unexpected symbol: 'x'".
func tokenError(src, msg, token string, offset int) *Error {
	return &Error{Msg: msg + ": '" + token + "'", Pos: utf16Len(src[:offset]) + 1, Expr: src}
}

// utf16Len returns the length of s in UTF-16 code units, which is how GitHub
// counts positions and lengths.
func utf16Len(s string) int {
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}
