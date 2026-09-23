package tabular

// A deliberately small XLSX reader: the first worksheet's cell values and
// nothing else. Styles are not read — a date in a workbook is a number with a
// date style, and the field the column lands in already says whether the
// number is a date — and neither are formulas, only the value Excel cached for
// them when the file was saved.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// maxEntryBytes caps each decompressed part of the archive: a few kilobytes
// of zip can expand into gigabytes of XML.
const maxEntryBytes = 64 << 20

// maxColumns is Excel's own limit (XFD).
const maxColumns = 16384

var errNotWorkbook = errors.New("the file is a zip archive but not an XLSX workbook")

func readXLSX(b []byte, o Options) (*Sheet, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, errNotWorkbook
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}

	var wb struct {
		Pr struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := readXML(files, "xl/workbook.xml", &wb); err != nil {
		return nil, err
	}
	if len(wb.Sheets) == 0 {
		return nil, ErrEmpty
	}
	var rels struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := readXML(files, "xl/_rels/workbook.xml.rels", &rels); err != nil {
		return nil, err
	}
	sheetPath := ""
	for _, r := range rels.Rel {
		if r.ID == wb.Sheets[0].RID {
			if strings.HasPrefix(r.Target, "/") {
				sheetPath = strings.TrimPrefix(r.Target, "/")
			} else {
				sheetPath = path.Join("xl", r.Target)
			}
		}
	}
	if sheetPath == "" {
		return nil, errNotWorkbook
	}

	var shared []string
	if _, ok := files["xl/sharedStrings.xml"]; ok {
		var sst struct {
			SI []richText `xml:"si"`
		}
		if err := readXML(files, "xl/sharedStrings.xml", &sst); err != nil {
			return nil, err
		}
		shared = make([]string, len(sst.SI))
		for i, si := range sst.SI {
			shared[i] = si.String()
		}
	}

	var ws struct {
		Rows []struct {
			R     int `xml:"r,attr"`
			Cells []struct {
				Ref    string    `xml:"r,attr"`
				Type   string    `xml:"t,attr"`
				Value  *string   `xml:"v"`
				Inline *richText `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := readXML(files, sheetPath, &ws); err != nil {
		return nil, err
	}

	s := &Sheet{Format: "xlsx", Date1904: wb.Pr.Date1904 == "1" || wb.Pr.Date1904 == "true"}
	line := 0
	for _, row := range ws.Rows {
		if row.R > 0 {
			line = row.R
		} else {
			line++
		}
		var cells []Cell
		next := 0
		for _, c := range row.Cells {
			col := next
			if c.Ref != "" {
				if col, err = columnIndex(c.Ref); err != nil {
					return nil, err
				}
			}
			next = col + 1
			if col >= maxColumns {
				return nil, fmt.Errorf("cell %s is beyond the last column", c.Ref)
			}
			cell, err := xlsxCell(c.Type, c.Value, c.Inline, shared)
			if err != nil {
				return nil, fmt.Errorf("cell %s: %w", c.Ref, err)
			}
			for len(cells) <= col {
				cells = append(cells, Cell{})
			}
			cells[col] = cell
		}
		if s.Headers == nil {
			texts := Row{Cells: cells}.Texts(len(cells))
			if isBlank(texts) {
				continue
			}
			s.Headers = texts
			continue
		}
		if err := s.collect(line, cells, o.MaxRows); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func xlsxCell(typ string, v *string, inline *richText, shared []string) (Cell, error) {
	val := ""
	if v != nil {
		val = *v
	}
	switch typ {
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || i < 0 || i >= len(shared) {
			return Cell{}, fmt.Errorf("unknown shared string %q", val)
		}
		return Cell{Text: shared[i]}, nil
	case "inlineStr":
		if inline == nil {
			return Cell{}, nil
		}
		return Cell{Text: inline.String()}, nil
	case "b":
		b := strings.TrimSpace(val) == "1"
		return Cell{Text: strconv.FormatBool(b), Bool: &b}, nil
	case "str", "e", "d":
		// a formula's text result, an error value like #N/A, an ISO date
		return Cell{Text: val}, nil
	}
	if strings.TrimSpace(val) == "" {
		return Cell{}, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return Cell{Text: val}, nil
	}
	return Cell{Text: formatNum(f), Num: &f}, nil
}

// richText is a shared or inline string: either one <t>, or runs of <r><t>
// when parts of it are formatted. Phonetic runs (<rPh>) are not the text.
type richText struct {
	T    *string `xml:"t"`
	Runs []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (r richText) String() string {
	if r.T != nil {
		return *r.T
	}
	var b strings.Builder
	for _, run := range r.Runs {
		b.WriteString(run.T)
	}
	return b.String()
}

// columnIndex turns "C12" into 2.
func columnIndex(ref string) (int, error) {
	n := 0
	i := 0
	for ; i < len(ref); i++ {
		ch := ref[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch < 'A' || ch > 'Z' {
			break
		}
		n = n*26 + int(ch-'A'+1)
		if n > maxColumns {
			return 0, fmt.Errorf("cell %s is beyond the last column", ref)
		}
	}
	if i == 0 {
		return 0, fmt.Errorf("invalid cell reference %q", ref)
	}
	return n - 1, nil
}

func readXML(files map[string]*zip.File, name string, v any) error {
	f, ok := files[name]
	if !ok {
		return errNotWorkbook
	}
	if f.UncompressedSize64 > maxEntryBytes {
		return fmt.Errorf("%s is too large", name)
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	// the header's size can lie; the reader cannot
	b, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
	if err != nil {
		return err
	}
	if len(b) > maxEntryBytes {
		return fmt.Errorf("%s is too large", name)
	}
	if err := xml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
