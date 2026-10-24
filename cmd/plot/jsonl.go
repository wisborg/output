package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// inputMode is --input: how the file is written.
type inputMode string

const (
	inputAuto  inputMode = "auto"
	inputCSV   inputMode = "csv"
	inputJSONL inputMode = "jsonl"
)

// maxJSONDepth is the deepest a JSON Lines value may nest, objects and
// arrays together, before its line is left out: deeper is no log's, and
// read by recursion, a line of a million brackets would take a frame
// each.
const maxJSONDepth = 64

// maxJSONColumns is the most columns JSON Lines is read into. A log whose
// keys differ from line to line -- a count per user, "counts.user123" --
// has a column a line, and without a limit its cells grew with the square
// of its lines: 4,000 of them took 640 MB. Keys past it are left out and
// said; a chart draws six columns unless told, and --list of a thousand is
// already more than a reader reads.
const maxJSONColumns = 1000

// maxJSONArray is the most elements of one array read as columns. Each
// element is a column, so a log holding a sample of a thousand readings a
// line took every column there was, and the keys after it -- the latency
// the log is read for -- were left out. Past it, the rest are left out and
// said.
const maxJSONArray = 100

// sniffInput is what --input auto takes data for: JSON Lines where any of
// its first 20 lines, blank ones aside, reads as a whole object, and CSV
// otherwise. Any, not the first: a log read from its middle, tail -c,
// begins with the end of a line, and some begin with a banner, which read
// as CSV gave columns named after JSON's fragments. Nor most: a log with a
// traceback of three lines after each object is a quarter objects, and was
// read so too. A CSV holds no line that is a whole object -- a cell holding
// one is quoted, its quotes doubled -- and the lines that are none are left
// out and said, as a cut line is.
//
// JSON that is not JSON Lines of objects is refused, saying what to make
// of it, where it was read as CSV of brackets and quotes: one array; an
// array a line; objects written over several lines each, as jq writes them
// without -c. These are looked for first, as an array of objects a line,
// or a pretty object holding one, has a line that is a whole object.
func sniffInput(data []byte) (inputMode, error) {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.HasPrefix(trimmed, []byte("[")):
		switch objects, ok := oneArray(trimmed); {
		case ok && objects:
			return "", errors.New("one JSON array: plot reads JSON Lines, an object a line, which jq -c '.[]' writes of it")
		case ok:
			return "", errors.New("one JSON array: plot reads JSON Lines, an object a line")
		}
	case bytes.HasPrefix(trimmed, []byte("{")) && overLines(trimmed):
		return "", errors.New("JSON objects over several lines each: plot reads JSON Lines, an object a line, which jq -c . writes of them")
	}
	lines, objects, arrays := 0, 0, 0
	for line := range bytes.Lines(data) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		// Too deep is an object still, which readJSONLines says of; not
		// counted, a file of them was refused as objects over several
		// lines.
		if _, err := walkLine(line); err == nil || errors.Is(err, errTooDeep) {
			objects++
		} else if bytes.HasPrefix(line, []byte("[")) && json.Valid(allowNonFinite(line)) {
			arrays++
		}
		if lines++; lines == 20 {
			break
		}
	}
	switch {
	case objects > 0:
		return inputJSONL, nil
	case lines > 1 && 2*arrays >= lines:
		return "", errors.New("an array a line: plot reads JSON Lines of objects, a column a key")
	}
	return inputCSV, nil
}

// overLines says data begins with JSON objects written over several lines
// each: its first values, up to three, objects, and the first of them over
// more than one line. The first alone, not that they are objects: a log of
// three whole lines and then cut ones decoded as three objects, and was
// refused as written over several lines, which none of it was. They are
// read one at a time, where the whole file decoded only to say it held it
// all twice.
func overLines(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	for i := range 3 {
		var v json.RawMessage
		// The first is no EOF, data beginning with "{".
		if err := dec.Decode(&v); err == io.EOF {
			return true
		} else if err != nil || !bytes.HasPrefix(v, []byte("{")) {
			return false
		}
		if i == 0 && !bytes.Contains(v, []byte("\n")) {
			return false
		}
	}
	return true
}

// oneArray says data is one JSON array and nothing after it, and whether
// its first elements, up to three, are objects. It reads a token at a
// time, holding none of it: validated, then decoded into elements to see
// the first, a file of one array was held twice to choose a message.
func oneArray(data []byte) (objects, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return false, false
	}
	objects = true
	for i := 0; dec.More(); i++ {
		first, err := skipValue(dec)
		if err != nil {
			return false, false
		}
		if i < 3 && first != json.Delim('{') {
			objects = false
		}
	}
	if _, err := dec.Token(); err != nil {
		return false, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return false, false
	}
	return objects, true
}

// skipValue reads one value from dec, of whatever depth, and is its first
// token.
func skipValue(dec *json.Decoder) (json.Token, error) {
	first, err := dec.Token()
	if err != nil {
		return nil, err
	}
	depth := 0
	if d, ok := first.(json.Delim); ok && (d == '{' || d == '[') {
		depth = 1
	}
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch tok {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
	return first, nil
}

// readJSONLines reads JSON Lines -- an object a line -- into a table: a
// column for each key, in the order the keys are first met, and a row for
// each object, a key it lacks an empty cell. A key inside an object is
// named by its path, "cpu.user"; an element of an array by its index,
// "load[0]". A number is kept as written, a string as it is but for the
// space about it, true and false as words, and null as an empty cell: a
// gap. NaN and Infinity, which JSON has no word for and Python writes
// anyway, are read as those words, as a CSV's cell is.
//
// A JSON number has a decimal point, never a comma, but a string may hold
// a number as a person in any place writes one, "1,5". The strings are
// kept as written, and what each column's say of the mark is kept in
// t.seen: the columns plotted decide it, through settleMarks, or mark
// says it. The numbers JSON writes are left as they are.
//
// The rows are sparse, t.sparse: each holds only the cells its line has
// keys for, as table says why. Past maxJSONColumns, keys are left out, and
// past maxJSONArray, an array's elements.
//
// What is lost is said in the table's notes, as a cell that is not a number
// is a gap and said: a line that is not one object -- cut short where a log
// was rotated mid-write, say, or text among the objects -- which is left
// out; the keys past maxJSONColumns, which the table keeps to say so if
// one is asked for; the elements past maxJSONArray; and a column given two
// values in one line, the last kept -- a key written twice, or two paths
// named alike, {"a.b":1,"a":{"b":2}}. A file with no object is refused.
func readJSONLines(data []byte, mark decimalMark) (*table, error) {
	t := &table{sparse: [][]sparseCell{}, dropped: map[string]bool{}}
	index := map[string]int{}
	var bad, deep, twice, cut lineTally
	var firstDropped, firstCut string
	var stamp []int // the line each column last had a value in
	var at []int    // where in its line's row each column's cell is
	n := 0
	for line := range bytes.Lines(data) {
		n++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		w, err := walkLine(line)
		if errors.Is(err, errTooDeep) {
			deep.add(n)
			continue
		} else if err != nil {
			bad.add(n)
			continue
		}
		if w.cut != "" {
			if cut.n == 0 {
				firstCut = w.cut
			}
			cut.add(n)
		}
		// Made the size of the line's fields, which it is unless keys
		// repeat or are left out: grown by append, a row of six cells
		// held room for eight.
		row := make([]sparseCell, 0, len(w.fields))
		again := false
		for _, f := range w.fields {
			j, seen := index[f.name]
			if !seen {
				if len(t.names) == maxJSONColumns {
					if !t.dropped[f.name] {
						if len(t.dropped) == 0 {
							firstDropped = f.name
						}
						t.dropped[f.name] = true
					}
					continue
				}
				j = len(t.names)
				index[f.name] = j
				t.names = append(t.names, f.name)
				t.seen = append(t.seen, marks{unit: "line"})
				stamp, at = append(stamp, 0), append(at, 0)
			}
			// Written twice in the line, the last value wins, and with it
			// whether the cell is a string: a number after a string was
			// taken for one and rewritten with a decimal comma, 1.500 as
			// 1500.
			c := sparseCell{col: int32(j), value: f.value, str: f.str}
			if stamp[j] == n {
				row[at[j]], again = c, true
				continue
			}
			stamp[j], at[j] = n, len(row)
			row = append(row, c)
		}
		if again {
			twice.add(n)
		}
		slices.SortFunc(row, func(a, b sparseCell) int { return cmp.Compare(a.col, b.col) })
		// What the strings the row keeps say of the decimal mark, each
		// column's apart, for settleMarks; JSON's own numbers say
		// nothing, having a point whatever the file's strings do. Only
		// where --decimal leaves it to be decided, and only of a string
		// with a mark in it, as one without reads the same either way:
		// every string of every line, messages and routes, was parsed
		// twice to say nothing.
		if mark == decimalAuto {
			for _, c := range row {
				if c.str && strings.ContainsAny(c.value, ".,") {
					t.seen[c.col].see(c.value, n, int(c.col))
				}
			}
		}
		t.sparse = append(t.sparse, row)
	}
	if len(t.sparse) == 0 {
		switch {
		case deep.n > 0 && (bad.n == 0 || deep.first < bad.first):
			return nil, fmt.Errorf("line %d: nested past %d deep, and no line is a JSON object to read", deep.first, maxJSONDepth)
		case bad.n > 0:
			return nil, fmt.Errorf("line %d: not a JSON object, and no line is one", bad.first)
		}
		return nil, errors.New("no rows to plot")
	}
	// Until settleMarks decides it for the columns plotted, each column is
	// read by what its own strings say -- with a point where they say
	// nothing, or both -- which is how --list shows it, and how the
	// columns plotted by default are chosen.
	t.readMark = make([]decimalMark, len(t.names))
	for j := range t.readMark {
		t.readMark[j] = mark
		if mark == decimalAuto {
			if d, _, err := t.seen[j].decide(decimalPoint); err == nil {
				t.readMark[j] = d
			} else {
				t.readMark[j] = decimalPoint
			}
		}
	}
	nameColumns(t.names)
	t.notes = bad.note(t.notes, "not a JSON object, left out", "")
	// Apart from the lines that are no object: a valid one said to be
	// none sends the reader looking for a fault that is not there.
	t.notes = deep.note(t.notes, fmt.Sprintf("nested past %d deep, left out", maxJSONDepth), "")
	switch len(t.dropped) {
	case 0:
	case 1:
		t.notes = append(t.notes, fmt.Sprintf("1 key past the first %d left out, %q", maxJSONColumns, firstDropped))
	default:
		t.notes = append(t.notes, fmt.Sprintf("%d keys past the first %d left out, the first %q", len(t.dropped), maxJSONColumns, firstDropped))
	}
	t.notes = cut.note(t.notes, fmt.Sprintf("%q cut to its first %d elements", firstCut, maxJSONArray), fmt.Sprintf("an array cut to its first %d elements, the first %q", maxJSONArray, firstCut))
	t.notes = twice.note(t.notes, "a column given two values, the last kept", "")
	return t, nil
}

// lineTally is how many lines something was said of, and the first: all a
// note of them says, where a list of every one grew with a log of
// millions of such lines.
type lineTally struct{ n, first int }

// add counts line.
func (l *lineTally) add(line int) {
	if l.n == 0 {
		l.first = line
	}
	l.n++
}

// note is notes and, where l counted any line, a note of them, in one
// wording for all of plot's notes of lines: "line 3: one" for a line, "4
// lines: many; the first line 3" for more -- many being one where it is
// empty.
func (l lineTally) note(notes []string, one, many string) []string {
	if many == "" {
		many = one
	}
	switch l.n {
	case 0:
		return notes
	case 1:
		return append(notes, fmt.Sprintf("line %d: %s", l.first, one))
	}
	return append(notes, fmt.Sprintf("%d lines: %s; the first line %d", l.n, many, l.first))
}

// field is one value of a JSON object, named by its path in it; str says
// it was a string, whose decimal mark the columns plotted decide.
type field struct {
	name, value string
	str         bool
}

// walker is what reading one line found: its fields, in the order they
// are written, and the first array cut short at maxJSONArray, if any. A
// key written twice is there twice, and readJSONLines, putting each name
// in one column, keeps the last, as encoding/json does.
type walker struct {
	dec    *json.Decoder
	fields []field
	cut    string
}

// errTooDeep is a line nested past maxJSONDepth: a JSON object, but none
// read, and said so apart from a line that is no object.
var errTooDeep = errors.New("nested too deep")

// errNotObject is a line that is not one JSON object and nothing after it.
var errNotObject = errors.New("not a JSON object")

// walkLine reads line, which must be one JSON object and nothing after it.
//
// A decoder a line, read a token at a time, is two to three times the time
// and memory of the same data as CSV: a known limit, kept. encoding/json's
// Decoder cannot be reset onto a new line, Unmarshal into a map loses the
// order of the keys, which is the order of the columns, and one decoder
// over the whole file cannot leave out a broken line and go on. What would
// do better is a tokenizer of plot's own, which was judged not worth its
// code for seconds at half a million lines.
func walkLine(line []byte) (*walker, error) {
	w := &walker{dec: json.NewDecoder(bytes.NewReader(allowNonFinite(line)))}
	w.dec.UseNumber()
	if tok, err := w.dec.Token(); err != nil || tok != json.Delim('{') {
		return w, errNotObject
	}
	if err := w.object("", true, 1, true); errors.Is(err, errTooDeep) {
		return w, err
	} else if err != nil {
		return w, errNotObject
	}
	if _, err := w.dec.Token(); err != io.EOF {
		return w, errNotObject
	}
	return w, nil
}

// object reads an object's members up to its closing brace, the opening
// one read already, adding each value under prefix where keep says to.
// top says it is the line's own object, whose keys are named alone; an
// object's under the key "" are named ".a", not "a", which is another
// key's.
func (w *walker) object(prefix string, top bool, depth int, keep bool) error {
	for w.dec.More() {
		tok, err := w.dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		name := key
		if !top {
			name = prefix + "." + key
		}
		if err := w.value(name, depth, keep); err != nil {
			return err
		}
	}
	_, err := w.dec.Token()
	return err
}

// value reads one value, adding it under name where keep says to: an
// object's or an array's members each under a name of their own. An
// array's elements past maxJSONArray are read, to reach what follows, and
// not kept.
func (w *walker) value(name string, depth int, keep bool) error {
	tok, err := w.dec.Token()
	if err != nil {
		return err
	}
	f := field{name: name}
	switch v := tok.(type) {
	case json.Delim:
		if depth == maxJSONDepth {
			return errTooDeep
		}
		if v == '{' {
			return w.object(name, false, depth+1, keep)
		}
		for i := 0; w.dec.More(); i++ {
			if i == maxJSONArray && keep && w.cut == "" {
				w.cut = name
			}
			if err := w.value(name+"["+strconv.Itoa(i)+"]", depth+1, keep && i < maxJSONArray); err != nil {
				return err
			}
		}
		_, err := w.dec.Token()
		return err
	case json.Number:
		f.value = v.String()
	case string:
		f.value, f.str = strings.TrimSpace(v), true
	case bool:
		f.value = strconv.FormatBool(v)
	case nil:
	}
	if keep {
		w.fields = append(w.fields, f)
	}
	return nil
}

// nonFinite are the words Python's json writes for numbers JSON has none
// for, which encoding/json refuses: a log holding one lost its line.
var nonFinite = [][]byte{[]byte("-Infinity"), []byte("Infinity"), []byte("NaN")}

// allowNonFinite is line with each of nonFinite quoted where a value
// stands, so it reads as a string, as the word in a CSV's cell does: NaN a
// gap, Infinity a cell not a number. Where a key stands it is left alone,
// as is a word inside a string or within a longer one: quoted there, a
// broken line, {NaN:1}, read as an object with a column "NaN". A line
// holding none is line itself.
//
// A value stands after a colon, after an array's opening bracket, and
// after a comma in an array; in an object a comma is followed by a key.
func allowNonFinite(line []byte) []byte {
	if !bytes.Contains(line, []byte("NaN")) && !bytes.Contains(line, []byte("Infinity")) {
		return line
	}
	out := make([]byte, 0, len(line)+8)
	var open []byte // the brackets and braces about i
	quoted, escaped, value := false, false, true
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				quoted = false
			}
		case c == '"':
			quoted, value = true, false
		case c == '{' || c == '[':
			open = append(open, c)
			value = c == '['
		case c == '}' || c == ']':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
			value = false
		case c == ':':
			value = true
		case c == ',':
			value = len(open) > 0 && open[len(open)-1] == '['
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
		default:
			if word := nonFiniteAt(line, i); value && word != nil {
				out = append(out, '"')
				out = append(out, word...)
				out = append(out, '"')
				i += len(word) - 1
				value = false
				continue
			}
			value = false
		}
		out = append(out, c)
	}
	return out
}

// nonFiniteAt is the word of nonFinite at line[i], standing alone, or nil.
func nonFiniteAt(line []byte, i int) []byte {
	if i > 0 && isWordByte(line[i-1]) {
		return nil
	}
	for _, word := range nonFinite {
		if bytes.HasPrefix(line[i:], word) {
			if end := i + len(word); end < len(line) && isWordByte(line[end]) {
				return nil
			}
			return word
		}
	}
	return nil
}

// isWordByte says c may be part of a bare JSON word or number.
func isWordByte(c byte) bool {
	return c == '-' || c == '+' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
