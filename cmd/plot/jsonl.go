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

// sniffInput is what --input auto takes data for: JSON Lines where at
// least half of its first 20 lines, blank ones aside, read each as a whole
// object, and CSV otherwise. Half, not the first: a log read from its
// middle, tail -c, begins with the end of a line, and some begin with a
// banner, which read as CSV gave columns named after JSON's fragments. A
// CSV holds no line that is a whole object.
//
// JSON that is not JSON Lines of objects is refused, saying what to make
// of it, where it was read as CSV of brackets and quotes: one array; an
// array a line; objects written over several lines each, as jq writes them
// without -c.
func sniffInput(data []byte) (inputMode, error) {
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
	case objects > 0 && 2*objects >= lines:
		return inputJSONL, nil
	case lines > 1 && 2*arrays >= lines:
		return "", errors.New("an array a line: plot reads JSON Lines of objects, a column a key")
	}
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.HasPrefix(trimmed, []byte("[")) && json.Valid(trimmed):
		var elems []json.RawMessage
		if json.Unmarshal(trimmed, &elems) == nil && len(elems) > 0 && allObjects(elems) {
			return "", errors.New("one JSON array: plot reads JSON Lines, an object a line, which jq -c '.[]' writes of it")
		}
		return "", errors.New("one JSON array: plot reads JSON Lines, an object a line")
	case bytes.HasPrefix(trimmed, []byte("{")):
		// The first few values say it, where the whole file decoded only
		// to choose a message held it all twice.
		var elems []json.RawMessage
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for range 3 {
			var v json.RawMessage
			if err := dec.Decode(&v); err == io.EOF {
				break
			} else if err != nil {
				return inputCSV, nil
			}
			elems = append(elems, v)
		}
		if allObjects(elems) {
			return "", errors.New("JSON objects over several lines each: plot reads JSON Lines, an object a line, which jq -c . writes of them")
		}
	}
	return inputCSV, nil
}

// allObjects says each of vs is a JSON object.
func allObjects(vs []json.RawMessage) bool {
	for _, v := range vs {
		if !bytes.HasPrefix(bytes.TrimSpace(v), []byte("{")) {
			return false
		}
	}
	return true
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
// a number as a person in any place writes one, "1,5", so the strings
// alone decide the file's mark, as decideDecimal decides a CSV's, or mark
// says it; the numbers JSON writes are left as they are.
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
	var bad, deep, twice, cut []int
	var firstDropped, firstCut string
	var stamp []int     // the line each column last had a value in
	var at []int        // where in its line's row each column's cell is
	var strs [][]string // each line's strings as its row holds them, for decideDecimal
	n := 0
	for line := range bytes.Lines(data) {
		n++
		strs = append(strs, nil)
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		w, err := walkLine(line)
		if errors.Is(err, errTooDeep) {
			deep = append(deep, n)
			continue
		} else if err != nil {
			bad = append(bad, n)
			continue
		}
		if w.cut != "" {
			if len(cut) == 0 {
				firstCut = w.cut
			}
			cut = append(cut, n)
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
			twice = append(twice, n)
		}
		slices.SortFunc(row, func(a, b sparseCell) int { return cmp.Compare(a.col, b.col) })
		for _, c := range row {
			if c.str {
				strs[n-1] = append(strs[n-1], c.value)
			}
		}
		t.sparse = append(t.sparse, row)
	}
	if len(t.sparse) == 0 {
		switch {
		case len(deep) > 0 && (len(bad) == 0 || deep[0] < bad[0]):
			return nil, fmt.Errorf("line %d: nested past %d deep, and no line is a JSON object to read", deep[0], maxJSONDepth)
		case len(bad) > 0:
			return nil, fmt.Errorf("line %d: not a JSON object, and no line is one", bad[0])
		}
		return nil, errors.New("no rows to plot")
	}
	// A JSON number has a point; only where nothing in the strings says,
	// is the mark a guess, and then a point, as JSON's own.
	decided, note, err := decideDecimal(strs, decimalPoint, mark, "line")
	if err != nil {
		return nil, err
	}
	if note != "" {
		t.notes = append(t.notes, note)
	}
	if decided == decimalComma {
		for _, row := range t.sparse {
			for k := range row {
				if row[k].str {
					row[k].value = commaToPoint(row[k].value)
				}
			}
		}
	}
	nameColumns(t.names)
	t.notes = appendLineNote(t.notes, bad, "not a JSON object, left out", "not a JSON object, left out")
	// Apart from the lines that are no object: a valid one said to be
	// none sends the reader looking for a fault that is not there.
	t.notes = appendLineNote(t.notes, deep, fmt.Sprintf("nested past %d deep, left out", maxJSONDepth), fmt.Sprintf("nested past %d deep, left out", maxJSONDepth))
	if len(t.dropped) > 0 {
		t.notes = append(t.notes, fmt.Sprintf("%d keys past the first %d left out, the first %q", len(t.dropped), maxJSONColumns, firstDropped))
	}
	t.notes = appendLineNote(t.notes, cut, fmt.Sprintf("%q cut to its first %d elements", firstCut, maxJSONArray), fmt.Sprintf("an array cut to its first %d elements, the first %q", maxJSONArray, firstCut))
	t.notes = appendLineNote(t.notes, twice, "a column given two values, the last kept", "a column given two values, the last kept")
	return t, nil
}

// appendLineNote is notes and, where lines holds any, a note of them, in
// one wording for all of plot's notes of lines: "line 3: one" for a line,
// "4 lines: many; the first line 3" for more.
func appendLineNote(notes []string, lines []int, one, many string) []string {
	switch len(lines) {
	case 0:
		return notes
	case 1:
		return append(notes, fmt.Sprintf("line %d: %s", lines[0], one))
	}
	return append(notes, fmt.Sprintf("%d lines: %s; the first line %d", len(lines), many, lines[0]))
}

// field is one value of a JSON object, named by its path in it; str says
// it was a string, whose decimal mark the file's strings decide.
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
