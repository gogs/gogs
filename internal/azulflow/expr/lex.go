package expr

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenLeftParen
	tokenRightParen
	tokenLeftBracket
	tokenRightBracket
	tokenComma
	tokenDot
	tokenBang
	tokenBangEqual
	tokenEqualEqual
	tokenGreater
	tokenGreaterEqual
	tokenLess
	tokenLessEqual
	tokenAnd
	tokenOr
	tokenStar
	tokenNumber
	tokenString
	tokenIdentifier
	tokenTrue
	tokenFalse
	tokenNull
)

type token struct {
	typ    tokenType
	lexeme string
	str    string  // The value of a string token.
	num    float64 // The value of a number token.
	offset int     // Byte offset in the source.
}

// String returns the token as GitHub quotes it in error messages: its
// source, so a string keeps its quotes.
func (t token) String() string {
	if t.typ == tokenEOF {
		return "EOF"
	}
	return t.lexeme
}

type lexer struct {
	src    string
	offset int
	tokens []token
}

func lex(src string) ([]token, error) {
	if utf16Len(src) > maxLength {
		return nil, &Error{Msg: msgExceededMaxLength, Expr: src}
	}
	l := &lexer{src: src}
	for l.offset < len(src) {
		start := l.offset
		c := src[l.offset]
		l.offset++
		var err error
		switch c {
		case '(':
			l.add(tokenLeftParen, start)
		case ')':
			l.add(tokenRightParen, start)
		case '[':
			l.add(tokenLeftBracket, start)
		case ']':
			l.add(tokenRightBracket, start)
		case ',':
			l.add(tokenComma, start)
		case '.':
			switch l.previous() {
			case tokenIdentifier, tokenRightBracket, tokenRightParen, tokenStar:
				l.add(tokenDot, start)
			default:
				err = l.number(start)
			}
		case '-', '+':
			err = l.number(start)
		case '!':
			l.addEither('=', tokenBangEqual, tokenBang, start)
		case '<':
			l.addEither('=', tokenLessEqual, tokenLess, start)
		case '>':
			l.addEither('=', tokenGreaterEqual, tokenGreater, start)
		case '=':
			err = l.double(c, tokenEqualEqual, start)
		case '&':
			err = l.double(c, tokenAnd, start)
		case '|':
			err = l.double(c, tokenOr, start)
		case '*':
			l.add(tokenStar, start)
		case '\'':
			err = l.string(start)
		default:
			r, size := utf8.DecodeRuneInString(src[start:])
			l.offset = start + size
			switch {
			case isDigit(c):
				err = l.number(start)
			case unicode.IsSpace(r):
			default:
				err = l.identifier(start)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	l.tokens = append(l.tokens, token{typ: tokenEOF, offset: len(src)})
	return l.tokens, nil
}

func (l *lexer) previous() tokenType {
	if len(l.tokens) == 0 {
		return tokenEOF
	}
	return l.tokens[len(l.tokens)-1].typ
}

func (l *lexer) match(c byte) bool {
	if l.offset < len(l.src) && l.src[l.offset] == c {
		l.offset++
		return true
	}
	return false
}

func (l *lexer) add(typ tokenType, start int) {
	l.tokens = append(l.tokens, token{typ: typ, lexeme: l.src[start:l.offset], offset: start})
}

func (l *lexer) addEither(next byte, two, one tokenType, start int) {
	if l.match(next) {
		l.add(two, start)
	} else {
		l.add(one, start)
	}
}

// double adds an operator made of c twice. A single c is illegal, and so is
// the word it starts.
func (l *lexer) double(c byte, typ tokenType, start int) error {
	if l.match(c) {
		l.add(typ, start)
		return nil
	}
	return l.identifier(start)
}

// skipWord advances to the next boundary character. When dots is true, a
// dot does not end the word, as in numbers.
func (l *lexer) skipWord(dots bool) {
	for l.offset < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.offset:])
		if isBoundary(r) && (!dots || r != '.') {
			return
		}
		l.offset += size
	}
}

func (l *lexer) number(start int) error {
	l.skipWord(true)
	lexeme := l.src[start:l.offset]
	n := parseNumber(lexeme)
	if math.IsNaN(n) {
		return tokenError(l.src, msgUnexpectedSymbol, lexeme, start)
	}
	l.tokens = append(l.tokens, token{typ: tokenNumber, lexeme: lexeme, num: n, offset: start})
	return nil
}

func (l *lexer) string(start int) error {
	var b strings.Builder
	for {
		end := strings.IndexByte(l.src[l.offset:], '\'')
		if end < 0 {
			return tokenError(l.src, msgUnexpectedSymbol, l.src[start:], start)
		}
		b.WriteString(l.src[l.offset : l.offset+end])
		l.offset += end + 1
		if !l.match('\'') {
			break
		}
		b.WriteByte('\'')
	}
	l.tokens = append(l.tokens, token{typ: tokenString, lexeme: l.src[start:l.offset], str: b.String(), offset: start})
	return nil
}

func (l *lexer) identifier(start int) error {
	l.skipWord(false)
	lexeme := l.src[start:l.offset]
	t := token{typ: tokenIdentifier, lexeme: lexeme, offset: start}
	if l.previous() != tokenDot {
		switch lexeme {
		case "true":
			t.typ = tokenTrue
		case "false":
			t.typ = tokenFalse
		case "null":
			t.typ = tokenNull
		case "NaN":
			t.typ, t.num = tokenNumber, math.NaN()
		case "Infinity":
			t.typ, t.num = tokenNumber, math.Inf(1)
		}
	}
	if !isLegalIdentifier(lexeme) {
		return tokenError(l.src, msgUnexpectedSymbol, lexeme, start)
	}
	l.tokens = append(l.tokens, t)
	return nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isBoundary(r rune) bool {
	switch r {
	case '(', '[', ')', ']', ',', '.', '!', '>', '<', '=', '&', '|':
		return true
	}
	return unicode.IsSpace(r)
}

func isLegalIdentifier(s string) bool {
	if s == "" {
		return false
	}
	if s[0] != '_' && !isLetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c != '_' && c != '-' && !isDigit(c) && !isLetter(c) {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
