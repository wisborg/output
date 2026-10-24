package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		if _, err := walkLine(line); err == nil {
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
		var elems []json.RawMessage
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for {
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
// A row holds cells only as far as its last key's column, and cell reads
// past that as empty: padded to every column, a log whose keys differ from
// line to line held a cell a column a line. Past maxJSONColumns, keys are
// left out, and past maxJSONArray, an array's elements.
//
// What is lost is said in the table's notes, as a cell that is not a number
// is a gap and said: a line that is not one object -- cut short where a log
// was rotated mid-write, say, or text among the objects -- which is left
// out; the keys past maxJSONColumns, which the table keeps to say so if
// one is asked for; the elements past maxJSONArray; and a column given two
// values in one line, the last kept -- a key written twice, or two paths
// named alike, {"a.b":1,"a":{"b":2}}. A file with no object is refused.
func readJSONLines(data []byte, mark decimalMark) (*table, error) {
	t := &table{dropped: map[string]bool{}}
	index := map[string]int{}
	var bad, deep, twice, cut []int
	var firstDropped, firstCut string
	var stamp []int     // the line each column last had a value in
	var strAt []int     // the line each column's value was last a string in
	var strCols [][]int // each row's columns that hold a string
	var rowLine []int   // each row's line
	n := 0
	for line := range bytes.Lines(data) {
		n++
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
		var row []string
		var touched []int
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
				stamp = append(stamp, 0)
				strAt = append(strAt, 0)
			}
			if stamp[j] == n {
				again = true
			} else {
				touched = append(touched, j)
			}
			stamp[j] = n
			for len(row) <= j {
				row = append(row, "")
			}
			// Whether the cell is a string is the last value's to say: a
			// number after a string, the key written twice, was taken for
			// a string and rewritten with a decimal comma, 1.500 as 1500.
			row[j], strAt[j] = f.value, 0
			if f.str {
				strAt[j] = n
			}
		}
		if again {
			twice = append(twice, n)
		}
		var cols []int
		for _, j := range touched {
			if strAt[j] == n {
				cols = append(cols, j)
			}
		}
		t.rows = append(t.rows, row)
		strCols = append(strCols, cols)
		rowLine = append(rowLine, n)
	}
	if len(t.rows) == 0 {
		switch {
		case len(deep) > 0 && (len(bad) == 0 || deep[0] < bad[0]):
			return nil, fmt.Errorf("line %d: nested past %d deep, and no line is a JSON object to read", deep[0], maxJSONDepth)
		case len(bad) > 0:
			return nil, fmt.Errorf("line %d: not a JSON object, and no line is one", bad[0])
		}
		return nil, errors.New("no rows to plot")
	}
	// The strings the rows hold, by the line they are on, for the message
	// that names one: not every string read, as a value a later one of the
	// same column replaced would decide a mark no cell is written in.
	strs := make([][]string, n)
	for i, cols := range strCols {
		for _, j := range cols {
			strs[rowLine[i]-1] = append(strs[rowLine[i]-1], t.rows[i][j])
		}
	}
	decided, note, err := decideDecimal(strs, ',', mark, "line")
	if err != nil {
		return nil, err
	}
	if note != "" {
		t.notes = append(t.notes, note)
	}
	if decided == decimalComma {
		for i, cols := range strCols {
			for _, j := range cols {
				t.rows[i][j] = commaToPoint(t.rows[i][j])
			}
		}
	}
	nameColumns(t.names)
	switch {
	case len(bad) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: not a JSON object, left out", bad[0]))
	case len(bad) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines not a JSON object, left out, the first line %d", len(bad), bad[0]))
	}
	// Apart from the lines that are no object: a valid one said to be
	// none sends the reader looking for a fault that is not there.
	switch {
	case len(deep) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: nested past %d deep, left out", deep[0], maxJSONDepth))
	case len(deep) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines nested past %d deep, left out, the first line %d", len(deep), maxJSONDepth, deep[0]))
	}
	if len(t.dropped) > 0 {
		t.notes = append(t.notes, fmt.Sprintf("%d keys past the first %d left out, the first %q", len(t.dropped), maxJSONColumns, firstDropped))
	}
	switch {
	case len(cut) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: %q's elements past the first %d left out", cut[0], firstCut, maxJSONArray))
	case len(cut) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines with an array's elements past the first %d left out, the first line %d, %q", len(cut), maxJSONArray, cut[0], firstCut))
	}
	switch {
	case len(twice) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: a column given two values, the last kept", twice[0]))
	case len(twice) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines giving a column two values, the last kept, the first line %d", len(twice), twice[0]))
	}
	return t, nil
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
// A decoder a line, read a token at a time, is about three times the time
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
