package dot

import (
	"strings"

	"github.com/wisborg/output/diagram"
)

// record reads a record shape's label into its fields, as Graphviz does:
// fields divided by "|", a field divided the other way between "{" and
// "}", and a field named to the edges by a port, "<name>", before its
// words. A backslash before "{", "}", "|", "<", ">" or a space writes that
// character; DOT's other escapes, \n, \l and \N among them, are read as a
// label's are, after, so a node "x|y" with no label of its own is one
// field, as in Graphviz. A field's words are taken without the spaces
// about them, an escaped one excepted. A label that is not one -- a brace
// or a "<" not closed, a "{" in a field's words, a port after them, braces
// nested more than maxDepth deep -- is no record, false, and the node is
// drawn as a box with its label as written, where Graphviz refuses to
// draw it.
func record(v, name, graph string) ([]diagram.Field, bool) {
	r := recordReader{s: v, name: name, graph: graph}
	fields, ok := r.list()
	if !ok || r.i != len(r.s) {
		return nil, false
	}
	return fields, true
}

type recordReader struct {
	s           string
	i           int
	name, graph string
	// depth is how many braces the reader is inside: a label of braces
	// nested deeper than maxDepth, as subgraphs may nest, is no record,
	// since reading and laying it out recurse once a brace and take time
	// growing with its square.
	depth int
}

// list reads fields divided by "|" up to a "}" or the end, and leaves the
// "}" to be read.
func (r *recordReader) list() ([]diagram.Field, bool) {
	var fields []diagram.Field
	for {
		f, ok := r.field()
		if !ok {
			return nil, false
		}
		fields = append(fields, f)
		if r.i < len(r.s) && r.s[r.i] == '|' {
			r.i++
			continue
		}
		return fields, true
	}
}

// field reads one field: "{" fields "}", or a port and words.
func (r *recordReader) field() (diagram.Field, bool) {
	for r.i < len(r.s) && r.s[r.i] == ' ' {
		r.i++
	}
	if r.i < len(r.s) && r.s[r.i] == '{' {
		if r.depth >= maxDepth {
			return diagram.Field{}, false
		}
		r.i++
		r.depth++
		fields, ok := r.list()
		r.depth--
		if !ok || r.i >= len(r.s) || r.s[r.i] != '}' {
			return diagram.Field{}, false
		}
		r.i++
		for r.i < len(r.s) && r.s[r.i] == ' ' {
			r.i++
		}
		return diagram.Field{Fields: fields}, true
	}
	var f diagram.Field
	// text is the field's words, and end how much of it is left once the
	// spaces after them are taken off: not an escaped one, which is how
	// Graphviz's labels keep a space at either end.
	var text strings.Builder
	end := 0
	for r.i < len(r.s) {
		c := r.s[r.i]
		switch c {
		case '|', '}':
			f.Label = label(text.String()[:end], r.name, r.graph, "")
			return f, true
		case '{', '>':
			return f, false
		case '<':
			gt := r.i + 1
			for gt < len(r.s) && r.s[gt] != '>' {
				if r.s[gt] == '\\' {
					gt++
				}
				gt++
			}
			// One port, before the words: one after them would take a
			// word out of the field's text, and Graphviz refuses both.
			if gt >= len(r.s) || f.Port != "" || text.Len() > 0 {
				return f, false
			}
			f.Port = strings.TrimSpace(unescape(r.s[r.i+1 : gt]))
			r.i = gt + 1
			continue
		case '\\':
			switch {
			case r.i+1 >= len(r.s):
				// A backslash at the end is itself.
				text.WriteByte(c)
				r.i++
			case strings.IndexByte("{}|<> ", r.s[r.i+1]) >= 0:
				text.WriteByte(r.s[r.i+1])
				r.i += 2
			default:
				// \n, \l, \N and the rest, kept for label to read.
				text.WriteString(r.s[r.i : r.i+2])
				r.i += 2
			}
			end = text.Len()
			continue
		case ' ', '\t', '\n', '\r':
			r.i++
			if text.Len() > 0 { // a space before the words is none of them
				text.WriteByte(c)
			}
			continue
		}
		text.WriteByte(c)
		end = text.Len()
		r.i++
	}
	f.Label = label(text.String()[:end], r.name, r.graph, "")
	return f, true
}

// fieldsText is the words of fields, in the order written, a space apart:
// a record's label for where its fields are not drawn.
func fieldsText(fields []diagram.Field) string {
	var words []string
	for _, f := range fields {
		w := f.Label
		if len(f.Fields) > 0 {
			w = fieldsText(f.Fields)
		}
		if w = strings.TrimSpace(w); w != "" {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// unescape is s with a backslash before any character taken off.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
