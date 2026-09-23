package tabular

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// readCSV reads a CSV the way spreadsheets write one: with or without the
// UTF-8 BOM, separated by a comma, a semicolon (Excel in a locale whose
// decimal separator is the comma) or a tab, and — from Excel's "CSV" rather
// than "CSV UTF-8" — in Windows-1252 instead of UTF-8.
func readCSV(b []byte, o Options) (*Sheet, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(b) {
		dec, err := charmap.Windows1252.NewDecoder().Bytes(b)
		if err != nil {
			return nil, err
		}
		b = dec
	}
	sep := o.Sep
	if sep == 0 {
		sep = sniffSep(b)
	}
	r := csv.NewReader(bytes.NewReader(b))
	r.Comma = sep
	r.FieldsPerRecord = -1
	s := &Sheet{Format: "csv", Sep: sep}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := r.FieldPos(0)
		if s.Headers == nil {
			if isBlank(rec) {
				continue
			}
			s.Headers = rec
			continue
		}
		cells := make([]Cell, len(rec))
		for i, v := range rec {
			cells[i] = Cell{Text: v}
		}
		if err := s.collect(line, cells, o.MaxRows); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// sniffSep picks the separator the header line uses most, counting only
// outside quotes. A header is the one line guaranteed to hold every column,
// and a label rarely holds a separator.
func sniffSep(b []byte) rune {
	counts := map[rune]int{}
	quoted := false
	for _, r := range string(b) {
		if r == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if r == '\n' {
			if counts[','] > 0 || counts[';'] > 0 || counts['\t'] > 0 {
				break
			}
			continue // a blank first line
		}
		if r == ',' || r == ';' || r == '\t' {
			counts[r]++
		}
	}
	best := ','
	for _, r := range []rune{';', '\t'} {
		if counts[r] > counts[best] {
			best = r
		}
	}
	return best
}

func isBlank(rec []string) bool {
	for _, v := range rec {
		if v != "" {
			return false
		}
	}
	return true
}
