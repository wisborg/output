package main

import (
	"bufio"
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

// sniffInput is what --input auto takes data for: JSON Lines where it
// begins with an object and one of its first lines reads as a whole one,
// CSV otherwise. A CSV beginning with "{" is rare; a header line that is a
// JSON object, rarer still. A file of one JSON array is neither, and is
// said to be, as plot read it as CSV of brackets and quotes.
func sniffInput(data []byte) (inputMode, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	switch {
	case bytes.HasPrefix(trimmed, []byte("[")) && json.Valid(trimmed):
		return "", errors.New("one JSON array: plot reads JSON Lines, an object a line, which jq -c '.[]' writes")
	case !bytes.HasPrefix(trimmed, []byte("{")):
		return inputCSV, nil
	}
	n := 0
	for line := range bytes.SplitSeq(trimmed, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if _, ok := jsonObject(line); ok {
			return inputJSONL, nil
		}
		if n++; n == 20 {
			break
		}
	}
	return inputCSV, nil
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
// A line that is not one object -- cut short where a log was rotated
// mid-write, say, or a line of text among the objects -- is left out and
// said in the table's notes with the first of its number, as a cell that
// is not a number is a gap and said. A file with none is refused.
func readJSONLines(data []byte) (*table, error) {
	t := &table{}
	index := map[string]int{}
	var bad []int
	br := bufio.NewReader(bytes.NewReader(data))
	for n := 1; ; n++ {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			fields, ok := jsonObject(bytes.TrimSpace(line))
			if !ok {
				bad = append(bad, n)
			} else {
				row := make([]string, len(t.names))
				for _, f := range fields {
					j, seen := index[f.name]
					if !seen {
						j = len(t.names)
						index[f.name] = j
						t.names = append(t.names, f.name)
						row = append(row, "")
					}
					row[j] = f.value
				}
				t.rows = append(t.rows, row)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if len(t.rows) == 0 {
		if len(bad) > 0 {
			return nil, fmt.Errorf("line %d: not a JSON object, and no line is one", bad[0])
		}
		return nil, errors.New("no rows to plot")
	}
	for i, row := range t.rows {
		for len(row) < len(t.names) {
			row = append(row, "")
		}
		t.rows[i] = row
	}
	for j, name := range t.names {
		if name == "" {
			t.names[j] = fmt.Sprintf("column %d", j+1)
		}
	}
	if len(bad) == 1 {
		t.notes = append(t.notes, fmt.Sprintf("line %d: not a JSON object, left out", bad[0]))
	} else if len(bad) > 1 {
		t.notes = append(t.notes, fmt.Sprintf("%d lines not a JSON object, left out, the first line %d", len(bad), bad[0]))
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
