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

// sniffInput is what --input auto takes data for: JSON Lines where at
// least half of its first 20 lines, blank ones aside, read each as a whole
// object, and CSV otherwise. Half, not the first: a log read from its
// middle, tail -c, begins with the end of a line, and some begin with a
// banner, which read as CSV gave columns named after JSON's fragments. A
// CSV holds no line that is a whole object.
//
// JSON that is not JSON Lines is refused, saying what to make of it, where
// it was read as CSV of brackets and quotes: one array, or objects written
// over several lines each, as jq writes them without -c.
func sniffInput(data []byte) (inputMode, error) {
	lines, objects := 0, 0
	for line := range bytes.Lines(data) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if _, ok := jsonObject(line); ok {
			objects++
		}
		if lines++; lines == 20 {
			break
		}
	}
	if objects > 0 && 2*objects >= lines {
		return inputJSONL, nil
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
// gap. Numbers in JSON have a decimal point, never a comma, so nothing is
// decided of the file's numbers as of a CSV's.
//
// A row holds cells only as far as its last key's column, and cell reads
// past that as empty: padded to every column, a log whose keys differ from
// line to line held a cell a column a line. Past maxJSONColumns, keys are
// left out.
//
// What is lost is said in the table's notes, as a cell that is not a number
// is a gap and said: a line that is not one object -- cut short where a log
// was rotated mid-write, say, or text among the objects -- which is left
// out; the keys past maxJSONColumns; and a column given two values in one
// line, the last kept -- a key written twice, or two paths named alike,
// {"a.b":1,"a":{"b":2}}. A file with no object is refused.
func readJSONLines(data []byte) (*table, error) {
	t := &table{}
	index := map[string]int{}
	var bad, twice []int
	dropped := map[string]bool{}
	var firstDropped string
	var stamp []int // the line each column last had a value in
	n := 0
	for line := range bytes.Lines(data) {
		n++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		fields, ok := jsonObject(line)
		if !ok {
			bad = append(bad, n)
			continue
		}
		var row []string
		again := false
		for _, f := range fields {
			j, seen := index[f.name]
			if !seen {
				if len(t.names) == maxJSONColumns {
					if !dropped[f.name] {
						if len(dropped) == 0 {
							firstDropped = f.name
						}
						dropped[f.name] = true
					}
					continue
				}
				j = len(t.names)
				index[f.name] = j
				t.names = append(t.names, f.name)
				stamp = append(stamp, 0)
			}
			if stamp[j] == n {
				again = true
			}
			stamp[j] = n
			for len(row) <= j {
				row = append(row, "")
			}
			row[j] = f.value
		}
		if again {
			twice = append(twice, n)
		}
		t.rows = append(t.rows, row)
	}
	if len(t.rows) == 0 {
		if len(bad) > 0 {
			return nil, fmt.Errorf("line %d: not a JSON object, and no line is one", bad[0])
		}
		return nil, errors.New("no rows to plot")
	}
	for j, name := range t.names {
		if name == "" {
			t.names[j] = fmt.Sprintf("column %d", j+1)
		}
	}
	switch {
	case len(bad) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: not a JSON object, left out", bad[0]))
	case len(bad) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines not a JSON object, left out, the first line %d", len(bad), bad[0]))
	}
	if len(dropped) > 0 {
		t.notes = append(t.notes, fmt.Sprintf("%d keys past the first %d left out, the first %q", len(dropped), maxJSONColumns, firstDropped))
	}
	switch {
	case len(twice) == 1:
		t.notes = append(t.notes, fmt.Sprintf("line %d: a column given two values, the last kept", twice[0]))
	case len(twice) > 1:
		t.notes = append(t.notes, fmt.Sprintf("%d lines giving a column two values, the last kept, the first line %d", len(twice), twice[0]))
	}
	return t, nil
}

// field is one value of a JSON object, named by its path in it.
type field struct{ name, value string }

// jsonObject is the fields of line, which must be one JSON object and
// nothing after it, in the order they are written. A key written twice is
// there twice, and readJSONLines, putting each name in one column, keeps
// the last, as encoding/json does.
func jsonObject(line []byte) ([]field, bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var fields []field
	if err := walkObject(dec, "", 1, &fields); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}

// walkObject reads an object's members up to its closing brace, the
// opening one read already, adding each value under prefix to fields.
func walkObject(dec *json.Decoder, prefix string, depth int, fields *[]field) error {
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		if err := walkValue(dec, name, depth, fields); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

// walkValue reads one value, adding it under name to fields: an object's
// or an array's members each under a name of their own.
func walkValue(dec *json.Decoder, name string, depth int, fields *[]field) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	var value string
	switch v := tok.(type) {
	case json.Delim:
		if depth == maxJSONDepth {
			return errors.New("nested too deep")
		}
		if v == '{' {
			return walkObject(dec, name, depth+1, fields)
		}
		for i := 0; dec.More(); i++ {
			if err := walkValue(dec, name+"["+strconv.Itoa(i)+"]", depth+1, fields); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case json.Number:
		value = v.String()
	case string:
		value = strings.TrimSpace(v)
	case bool:
		value = strconv.FormatBool(v)
	case nil:
		value = ""
	}
	*fields = append(*fields, field{name, value})
	return nil
}
