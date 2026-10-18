package dot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokKind is what a token is.
type tokKind int

const (
	tEOF   tokKind = iota
	tID            // an identifier, a number, a quoted string or an HTML string
	tPunct         // { } [ ] = ; , :
	tEdge          // -> or --
)

type token struct {
	kind tokKind
	text string // an ID's value, a quoted one unquoted; the punctuation itself
	// quoted says the ID was written in quotes or angle brackets, so it is
	// never a keyword: "node" is a node called node.
	quoted bool
	line   int
}

// lexer splits DOT source into tokens.
type lexer struct {
	src  string
	pos  int
	line int
}

// Error is a mistake in a DOT file, and the line it is on. Err, where it is
// not nil, is a sentinel the mistake is one of, such as ErrTooManyEdges.
type Error struct {
	Line int
	Msg  string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// Unwrap is Err, so errors.Is reaches it.
func (e *Error) Unwrap() error { return e.Err }

func (l *lexer) errorf(format string, args ...any) error {
	return &Error{Line: l.line, Msg: fmt.Sprintf(format, args...)}
}

// next is the next token. Comments -- // and /* */ as in C, and a line
// beginning with #, which DOT leaves to a C preprocessor -- are spaces.
func (l *lexer) next() (token, error) {
	for {
		l.skipSpace()
		if l.pos >= len(l.src) {
			return token{kind: tEOF, line: l.line}, nil
		}
		rest := l.src[l.pos:]
		switch {
		case strings.HasPrefix(rest, "//"):
			l.skipLine()
			continue
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				return token{}, l.errorf("a /* comment is never closed")
			}
			l.advance(end + 4)
			continue
		case rest[0] == '#' && l.atLineStart():
			l.skipLine()
			continue
		}
		break
	}
	rest := l.src[l.pos:]
	line := l.line
	switch c := rest[0]; {
	case strings.HasPrefix(rest, "->") || strings.HasPrefix(rest, "--"):
		l.advance(2)
		return token{kind: tEdge, text: rest[:2], line: line}, nil
	case strings.ContainsRune("{}[]=;,:", rune(c)):
		l.advance(1)
		return token{kind: tPunct, text: string(c), line: line}, nil
	case c == '"':
		s, err := l.quoted()
		return token{kind: tID, text: s, quoted: true, line: line}, err
	case c == '<':
		s, err := l.html()
		return token{kind: tID, text: s, quoted: true, line: line}, err
	case c == '-' || c == '.' || c >= '0' && c <= '9':
		n := l.number()
		if n == "" {
			return token{}, l.errorf("%q is not a number, an identifier or an edge", rest[:1])
		}
		return token{kind: tID, text: n, line: line}, nil
	}
	r, _ := utf8.DecodeRuneInString(rest)
	if !idStart(r) {
		return token{}, l.errorf("unexpected %q", r)
	}
	start := l.pos
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !idStart(r) && !unicode.IsDigit(r) {
			break
		}
		l.pos += size
	}
	return token{kind: tID, text: l.src[start:l.pos], line: line}, nil
}

// idStart says r may begin an unquoted identifier: a letter, an underscore,
// or any character past ASCII, as DOT's \200-\377 bytes are.
func idStart(r rune) bool {
	return r == '_' || r >= 0x80 || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func (l *lexer) advance(n int) {
	l.line += strings.Count(l.src[l.pos:l.pos+n], "\n")
	l.pos += n
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.src) {
		switch l.src[l.pos] {
		case '\n':
			l.line++
			fallthrough
		case ' ', '\t', '\r', '\f', '\v':
			l.pos++
		default:
			return
		}
	}
}

func (l *lexer) skipLine() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
}

func (l *lexer) atLineStart() bool {
	i := strings.LastIndexByte(l.src[:l.pos], '\n')
	return strings.TrimSpace(l.src[i+1:l.pos]) == ""
}

// number reads a numeral: [-]?(.[0-9]+ | [0-9]+(.[0-9]*)?).
func (l *lexer) number() string {
	start := l.pos
	if l.pos < len(l.src) && l.src[l.pos] == '-' {
		l.pos++
	}
	digits, dot := 0, false
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '.' && !dot {
			dot = true
		} else if c >= '0' && c <= '9' {
			digits++
		} else {
			break
		}
		l.pos++
	}
	if digits == 0 {
		l.pos = start
		return ""
	}
	return l.src[start:l.pos]
}

// quoted reads a double-quoted string, and any joined to it with +, as DOT
// joins them. Only \" is an escape here; the rest -- \n, \l, \N, \\ -- are
// left for the attribute that reads them, since what they mean depends on
// it, but are read as pairs, as Graphviz reads them: in "C:\\" the second
// backslash is the first's, and the quote after them closes the string.
// Taken alone, that backslash escaped the quote, and the string ran on into
// the statements after it. A backslash before a line break -- "\n" or
// "\r\n" -- continues the line.
func (l *lexer) quoted() (string, error) {
	var b strings.Builder
	start := l.line
	for {
		l.advance(1) // the opening quote
		for {
			if l.pos >= len(l.src) {
				return "", &Error{Line: start, Msg: "a quoted string is never closed"}
			}
			c := l.src[l.pos]
			switch {
			case c == '"':
				l.advance(1)
			case c == '\\' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '"':
				b.WriteByte('"')
				l.advance(2)
				continue
			case c == '\\' && strings.HasPrefix(l.src[l.pos+1:], "\n"):
				l.advance(2)
				continue
			case c == '\\' && strings.HasPrefix(l.src[l.pos+1:], "\r\n"):
				l.advance(3)
				continue
			case c == '\\' && l.pos+1 < len(l.src):
				b.WriteString(l.src[l.pos : l.pos+2])
				l.advance(2)
				continue
			default:
				b.WriteByte(c)
				l.advance(1)
				continue
			}
			break
		}
		// A + joins the next quoted string to this one.
		save, saveLine := l.pos, l.line
		l.skipSpace()
		if l.pos < len(l.src) && l.src[l.pos] == '+' {
			l.advance(1)
			l.skipSpace()
			if l.pos < len(l.src) && l.src[l.pos] == '"' {
				continue
			}
		}
		l.pos, l.line = save, saveLine
		return b.String(), nil
	}
}

// html reads an HTML string, <...> with its angle brackets balanced, as its
// text alone: the markup Graphviz draws tables with is not drawn here, and
// the words in it are a better label than the tags.
func (l *lexer) html() (string, error) {
	depth := 0
	start, line := l.pos, l.line
	for l.pos < len(l.src) {
		switch l.src[l.pos] {
		case '<':
			depth++
		case '>':
			depth--
		}
		l.advance(1)
		if depth == 0 {
			return htmlText(l.src[start+1 : l.pos-1]), nil
		}
	}
	// Where it opens, as an unclosed quoted string is reported: the end of
	// the file is no help in finding it.
	return "", &Error{Line: line, Msg: "an HTML string is never closed"}
}

// entity is an HTML character reference: a name, or a number in decimal
// or hexadecimal.
var entity = regexp.MustCompile(`&(lt|gt|quot|amp|apos|nbsp|#[0-9]+|#[xX][0-9a-fA-F]+);`)

// entities is s with its character references read: the named few, and
// any by number -- "&#60;", "&#x3C;" -- which were drawn as written.
func entities(s string) string {
	return entity.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		switch name {
		case "lt":
			return "<"
		case "gt":
			return ">"
		case "quot":
			return `"`
		case "amp":
			return "&"
		case "apos":
			return "'"
		case "nbsp":
			return " "
		}
		base, digits := 10, name[1:]
		if digits[0] == 'x' || digits[0] == 'X' {
			base, digits = 16, digits[1:]
		}
		n, err := strconv.ParseUint(digits, base, 32)
		if err != nil || !utf8.ValidRune(rune(n)) {
			return m
		}
		return string(rune(n))
	})
}

// htmlComment is an HTML comment, <!-- ... -->, over lines as well.
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// htmlText is the words of HTML markup: tags dropped, <br/> a line break,
// a few entities read.
func htmlText(s string) string {
	// Comments first: one may hold a ">", which the tag scan below takes
	// for the comment's end, and its words are no part of the label.
	s = htmlComment.ReplaceAllString(s, "")
	var b strings.Builder
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			break
		}
		tag := strings.ToLower(strings.Trim(s[i+1:i+j], "/ "))
		if tag == "br" || strings.HasPrefix(tag, "br ") || tag == "tr" {
			b.WriteByte('\n')
		}
		s = s[i+j+1:]
	}
	out := entities(b.String())
	lines := strings.Split(out, "\n")
	var kept []string
	for _, line := range lines {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
