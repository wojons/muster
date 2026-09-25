package graphql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// GraphQL executable and type-system documents share one lexical grammar, so
// this file holds the single tokenizer every parser in the package uses: the
// SDL parser, the type-reference parser (introspection gives type references as
// nested objects, but SDL and default values give them as text), and the
// default-value literal parser.

// tokenKind classifies GraphQL tokens.
type tokenKind int

const (
	tokEOF    tokenKind = iota
	tokName             // Name token (field, type, enum value, `true`, `null`, ...)
	tokInt              // integer literal
	tokFloat            // float literal
	tokString           // string literal (already unescaped)
	tokPunct            // one of ! $ & ( ) @ [ ] { | } :
)

// token is one lexical unit with its source position (for error messages).
type token struct {
	kind tokenKind
	val  string
	line int
	col  int
}

// describe renders a token for error messages.
func (t token) describe() string {
	if t.kind == tokEOF {
		return "end of document"
	}
	return strconv.Quote(t.val)
}

// lexer tokenizes a GraphQL document.
type lexer struct {
	src  string
	pos  int
	line int
	col  int
}

// newLexer creates a tokenizer over src. Line/column tracking is 1-based.
func newLexer(src string) *lexer {
	return &lexer{src: src, line: 1, col: 1}
}

// parseError reports a syntax error with its position.
func (l *lexer) parseError(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s (line %d, column %d)", msg, l.line, l.col)
}

// advance consumes one rune, keeping line/column in step.
func (l *lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += size
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// peek returns the rune at the current position without consuming it.
func (l *lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

// peekAt returns the rune n runes ahead without consuming anything.
func (l *lexer) peekAt(n int) rune {
	pos := l.pos
	for i := 0; i < n; i++ {
		if pos >= len(l.src) {
			return 0
		}
		_, size := utf8.DecodeRuneInString(l.src[pos:])
		pos += size
	}
	if pos >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[pos:])
	return r
}

// skipIgnored consumes whitespace, commas, comments and the byte-order mark
// (GraphQL treats commas as insignificant whitespace).
func (l *lexer) skipIgnored() {
	for l.pos < len(l.src) {
		switch r := l.peek(); {
		case r == '\ufeff' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',':
			l.advance()
		case r == '#':
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

// isNameStart reports whether r may start a GraphQL Name.
func isNameStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// isNameContinue reports whether r may continue a GraphQL Name.
func isNameContinue(r rune) bool {
	return isNameStart(r) || (r >= '0' && r <= '9')
}

// isDigit reports whether r is a decimal digit.
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// punctChars are the single-character punctuators GraphQL defines.
const punctChars = "!$&()@[]{|}:="

// next returns the next token.
func (l *lexer) next() (token, error) {
	l.skipIgnored()
	tok := token{line: l.line, col: l.col}
	if l.pos >= len(l.src) {
		tok.kind = tokEOF
		return tok, nil
	}

	r := l.peek()
	switch {
	case isNameStart(r):
		tok.kind = tokName
		tok.val = l.readName()
		return tok, nil
	case isDigit(r) || r == '-':
		return l.readNumber(tok)
	case r == '"':
		val, err := l.readString()
		if err != nil {
			return tok, err
		}
		tok.kind = tokString
		tok.val = val
		return tok, nil
	case strings.ContainsRune(punctChars, r):
		tok.kind = tokPunct
		tok.val = string(l.advance())
		return tok, nil
	default:
		l.advance()
		return tok, l.parseError("unexpected character %q", string(r))
	}
}

// readName consumes a Name token.
func (l *lexer) readName() string {
	start := l.pos
	for l.pos < len(l.src) && isNameContinue(l.peek()) {
		l.advance()
	}
	return l.src[start:l.pos]
}

// readNumber consumes an Int or Float literal.
func (l *lexer) readNumber(tok token) (token, error) {
	start := l.pos
	if l.peek() == '-' {
		l.advance()
	}
	if !isDigit(l.peek()) {
		return tok, l.parseError("invalid number literal")
	}
	for isDigit(l.peek()) {
		l.advance()
	}
	tok.kind = tokInt
	if l.peek() == '.' || l.peek() == 'e' || l.peek() == 'E' {
		tok.kind = tokFloat
		if l.peek() == '.' {
			l.advance()
			for isDigit(l.peek()) {
				l.advance()
			}
		}
		if l.peek() == 'e' || l.peek() == 'E' {
			l.advance()
			if l.peek() == '+' || l.peek() == '-' {
				l.advance()
			}
			for isDigit(l.peek()) {
				l.advance()
			}
		}
	}
	tok.val = l.src[start:l.pos]
	return tok, nil
}

// readString consumes a string literal (block or single-quoted) and returns its
// unescaped value.
func (l *lexer) readString() (string, error) {
	if l.peekAt(1) == '"' && l.peekAt(2) == '"' {
		return l.readBlockString()
	}
	l.advance() // opening quote
	var sb strings.Builder
	for {
		if l.pos >= len(l.src) {
			return "", l.parseError("unterminated string literal")
		}
		r := l.advance()
		switch r {
		case '"':
			return sb.String(), nil
		case '\n':
			return "", l.parseError("unterminated string literal")
		case '\\':
			esc, err := l.readEscape()
			if err != nil {
				return "", err
			}
			sb.WriteString(esc)
		default:
			sb.WriteRune(r)
		}
	}
}

// readEscape consumes the character(s) after a backslash in a single-quoted
// string literal.
func (l *lexer) readEscape() (string, error) {
	if l.pos >= len(l.src) {
		return "", l.parseError("unterminated escape sequence")
	}
	r := l.advance()
	switch r {
	case '"':
		return `"`, nil
	case '\\':
		return `\`, nil
	case '/':
		return "/", nil
	case 'b':
		return "\b", nil
	case 'f':
		return "\f", nil
	case 'n':
		return "\n", nil
	case 'r':
		return "\r", nil
	case 't':
		return "\t", nil
	case 'u':
		hex := ""
		for i := 0; i < 4; i++ {
			if l.pos >= len(l.src) {
				return "", l.parseError("truncated \\u escape")
			}
			hex += string(l.advance())
		}
		code, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return "", l.parseError("invalid \\u escape %q", hex)
		}
		return string(rune(code)), nil
	default:
		return "", l.parseError("invalid escape sequence \\%s", string(r))
	}
}

// readBlockString consumes a """block string""" and returns its value with the
// GraphQL block-string indentation rules applied.
func (l *lexer) readBlockString() (string, error) {
	l.advance()
	l.advance()
	l.advance() // opening """
	start := l.pos
	for {
		if l.pos >= len(l.src) {
			return "", l.parseError("unterminated block string")
		}
		if l.peek() == '"' && l.peekAt(1) == '"' && l.peekAt(2) == '"' {
			raw := l.src[start:l.pos]
			l.advance()
			l.advance()
			l.advance()
			return dedentBlockString(raw), nil
		}
		l.advance()
	}
}

// dedentBlockString applies GraphQL's block-string semantics: strip the common
// indentation of all lines after the first, and trim leading/trailing blank
// lines.
func dedentBlockString(raw string) string {
	lines := strings.Split(raw, "\n")
	if len(lines) > 1 {
		common := -1
		for _, line := range lines[1:] {
			indent := 0
			for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
				indent++
			}
			if indent == len(line) { // blank line: ignore for the indent count
				continue
			}
			if common < 0 || indent < common {
				common = indent
			}
		}
		if common > 0 {
			for i := 1; i < len(lines); i++ {
				if len(lines[i]) >= common {
					lines[i] = lines[i][common:]
				}
			}
		}
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// parserTokenStream turns a lexer into a token cursor with one token of
// lookahead plus the parser helpers SDL, type references and value literals
// all need.
type parserTokenStream struct {
	lex  *lexer
	cur  token
	err  error
	init bool
}

// newTokenStream creates a token stream positioned on the first token.
func newTokenStream(src string) (*parserTokenStream, error) {
	ts := &parserTokenStream{lex: newLexer(src)}
	if err := ts.advance(); err != nil {
		return nil, err
	}
	return ts, nil
}

// advance moves to the next token.
func (ts *parserTokenStream) advance() error {
	if ts.err != nil {
		return ts.err
	}
	tok, err := ts.lex.next()
	if err != nil {
		ts.err = err
		return err
	}
	ts.cur = tok
	ts.init = true
	return nil
}

// atPunct reports whether the current token is the given punctuator.
func (ts *parserTokenStream) atPunct(p string) bool {
	return ts.init && ts.cur.kind == tokPunct && ts.cur.val == p
}

// atName reports whether the current token is the given Name.
func (ts *parserTokenStream) atName(n string) bool {
	return ts.init && ts.cur.kind == tokName && ts.cur.val == n
}

// expectPunct consumes the given punctuator or fails.
func (ts *parserTokenStream) expectPunct(p string) error {
	if !ts.atPunct(p) {
		return ts.errorf("expected %q but found %s", p, ts.curToken().describe())
	}
	return ts.advance()
}

// expectName consumes a Name token and returns its value.
func (ts *parserTokenStream) expectName(what string) (string, error) {
	if !ts.init || ts.cur.kind != tokName {
		return "", ts.errorf("expected %s but found %s", what, ts.curToken().describe())
	}
	name := ts.cur.val
	if err := ts.advance(); err != nil {
		return "", err
	}
	return name, nil
}

// errorf reports a syntax error at the current token.
func (ts *parserTokenStream) errorf(format string, args ...interface{}) error {
	line, col := 0, 0
	if ts.init {
		line, col = ts.cur.line, ts.cur.col
	}
	return fmt.Errorf("%s (line %d, column %d)", fmt.Sprintf(format, args...), line, col)
}

// curToken returns the current token (an EOF token before initialization).
func (ts *parserTokenStream) curToken() token {
	if !ts.init {
		return token{kind: tokEOF}
	}
	return ts.cur
}

// atEOF reports whether the stream is exhausted.
func (ts *parserTokenStream) atEOF() bool {
	return !ts.init || ts.cur.kind == tokEOF
}

// parseTypeRefFromTokens parses a GraphQL type reference ("[User!]!").
func parseTypeRefFromTokens(ts *parserTokenStream) (*TypeRef, error) {
	if ts.atPunct("[") {
		if err := ts.advance(); err != nil {
			return nil, err
		}
		elem, err := parseTypeRefFromTokens(ts)
		if err != nil {
			return nil, err
		}
		if err := ts.expectPunct("]"); err != nil {
			return nil, err
		}
		ref := &TypeRef{Kind: KindList, OfType: elem}
		return maybeNonNull(ts, ref)
	}
	name, err := ts.expectName("type name")
	if err != nil {
		return nil, err
	}
	return maybeNonNull(ts, &TypeRef{Kind: KindObject, Name: name})
}

// maybeNonNull wraps ref in a NON_NULL reference when the next token is "!".
func maybeNonNull(ts *parserTokenStream, ref *TypeRef) (*TypeRef, error) {
	if ts.atPunct("!") {
		if err := ts.advance(); err != nil {
			return nil, err
		}
		return &TypeRef{Kind: KindNonNull, OfType: ref}, nil
	}
	return ref, nil
}

// enumLiteral is a bare Name in a value literal (an enum value, e.g. `ADMIN`).
// It is a distinct type from string so rendering the literal back keeps the
// unquoted spelling GraphQL uses for enums (introspection reports enum defaults
// bare too, so both ingestion paths agree about a default's spelling).
type enumLiteral string

// parseValueLiteral parses a GraphQL value literal (used for argument and
// input-field defaults). Supported: null/true/false, Int, Float, String,
// enum-name, List and InputObject literals. Variables are not valid in
// defaults and are rejected.
func parseValueLiteral(ts *parserTokenStream) (interface{}, error) {
	switch {
	case ts.curToken().kind == tokString:
		v := ts.cur.val
		return v, ts.advance()
	case ts.curToken().kind == tokInt:
		v := ts.cur.val
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			// Very large integers fall back to float64 rather than failing the
			// whole ingestion.
			f, ferr := strconv.ParseFloat(v, 64)
			if ferr != nil {
				return nil, ts.errorf("invalid integer literal %q", v)
			}
			return f, ts.advance()
		}
		return n, ts.advance()
	case ts.curToken().kind == tokFloat:
		v := ts.cur.val
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, ts.errorf("invalid float literal %q", v)
		}
		return f, ts.advance()
	case ts.atPunct("["):
		if err := ts.advance(); err != nil {
			return nil, err
		}
		list := []interface{}{}
		for !ts.atPunct("]") {
			if ts.atEOF() {
				return nil, ts.errorf("unterminated list literal")
			}
			item, err := parseValueLiteral(ts)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		return list, ts.expectPunct("]")
	case ts.atPunct("{"):
		if err := ts.advance(); err != nil {
			return nil, err
		}
		obj := map[string]interface{}{}
		for !ts.atPunct("}") {
			if ts.atEOF() {
				return nil, ts.errorf("unterminated object literal")
			}
			name, err := ts.expectName("input field name")
			if err != nil {
				return nil, err
			}
			if err := ts.expectPunct(":"); err != nil {
				return nil, err
			}
			val, err := parseValueLiteral(ts)
			if err != nil {
				return nil, err
			}
			obj[name] = val
		}
		return obj, ts.expectPunct("}")
	case ts.curToken().kind == tokName:
		switch ts.cur.val {
		case "null":
			return nil, ts.advance()
		case "true":
			return true, ts.advance()
		case "false":
			return false, ts.advance()
		default:
			// Bare name: an enum value (or an unresolvable literal). Keep the
			// unquoted spelling the document used.
			v := ts.cur.val
			return enumLiteral(v), ts.advance()
		}
	case ts.atPunct("$"):
		return nil, ts.errorf("variable references are not valid in default values")
	default:
		return nil, ts.errorf("unexpected %s in value literal", ts.curToken().describe())
	}
}

// parseValueLiteralString parses a default-value literal string as
// introspection reports it ("" is no default; "null" is an explicit null).
func parseValueLiteralString(src string) (interface{}, error) {
	ts, err := newTokenStream(src)
	if err != nil {
		return nil, err
	}
	if ts.atEOF() {
		return nil, fmt.Errorf("empty default value literal")
	}
	val, err := parseValueLiteral(ts)
	if err != nil {
		return nil, err
	}
	if !ts.atEOF() {
		return nil, ts.errorf("unexpected trailing token %s in default value", ts.curToken().describe())
	}
	return val, nil
}
