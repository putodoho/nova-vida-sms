// Package report defines the tabular report format shared by the JSON API and
// the Excel export, and a small dependency-free XLSX writer.
package report

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// Column types.
const (
	Text    = "text"
	Integer = "int"
	Money   = "money" // money.Amount (centavos)
	Date    = "date"  // time.Time or "YYYY-MM-DD"
	Percent = "percent"
)

type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"` // Portuguese, shown in the UI and the spreadsheet
	Type  string `json:"type"`
}

// Table is a report. Rows hold values in column order; Totals (optional)
// has the same length, nil where no total applies.
type Table struct {
	Title   string   `json:"title"`
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Totals  []any    `json:"totals,omitempty"`
}

// Write serves the table as JSON, or as an .xlsx download when ?format=xlsx.
func Write(w http.ResponseWriter, r *http.Request, t Table, filename string) error {
	if t.Rows == nil {
		t.Rows = [][]any{}
	}
	if r.URL.Query().Get("format") != "xlsx" {
		httpx.JSON(w, http.StatusOK, t)
		return nil
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, filename))
	return WriteXLSX(w, t)
}

// ---------------------------------------------------------------------------
// XLSX
// ---------------------------------------------------------------------------

const (
	styleDefault = 0
	styleHeader  = 1
	styleMoney   = 2
	styleDate    = 3
	styleTotal   = 4
	styleTitle   = 5
	stylePercent = 6
)

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="3">
<numFmt numFmtId="164" formatCode="#,##0.00&quot; MT&quot;"/>
<numFmt numFmtId="165" formatCode="dd/mm/yyyy"/>
<numFmt numFmtId="166" formatCode="0.0%"/>
</numFmts>
<fonts count="3">
<font><sz val="11"/><name val="Calibri"/></font>
<font><b/><sz val="11"/><name val="Calibri"/></font>
<font><b/><sz val="14"/><name val="Calibri"/></font>
</fonts>
<fills count="3">
<fill><patternFill patternType="none"/></fill>
<fill><patternFill patternType="gray125"/></fill>
<fill><patternFill patternType="solid"><fgColor rgb="FFEAF2FB"/><bgColor indexed="64"/></patternFill></fill>
</fills>
<borders count="2">
<border><left/><right/><top/><bottom/><diagonal/></border>
<border><left/><right/><top style="thin"><color rgb="FF173B67"/></top><bottom/><diagonal/></border>
</borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="7">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/>
<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="164" fontId="1" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyBorder="1"/>
<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

// WriteXLSX writes the table as a one-sheet workbook: title on row 1, header
// on row 3 (frozen, with filter), data, then a bold totals row.
func WriteXLSX(out io.Writer, t Table) error {
	z := zip.NewWriter(out)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="` + esc(sheetName(t.Title)) + `" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/styles.xml":            stylesXML,
		"xl/worksheets/sheet1.xml": sheetXML(t),
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, files[name]); err != nil {
			return err
		}
	}
	return z.Close()
}

func sheetXML(t Table) string {
	var b strings.Builder
	n := len(t.Columns)
	lastCol := colName(max(n-1, 0))
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetViews><sheetView workbookViewId="0"><pane ySplit="3" topLeftCell="A4" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>
<cols>`)
	for i, c := range t.Columns {
		width := max(len([]rune(c.Label))+2, 12)
		switch c.Type {
		case Text:
			width = max(width, 28)
		case Money:
			width = max(width, 18)
		}
		fmt.Fprintf(&b, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, width)
	}
	b.WriteString(`</cols><sheetData>`)
	fmt.Fprintf(&b, `<row r="1">%s</row>`, strCell("A1", t.Title, styleTitle))
	b.WriteString(`<row r="3">`)
	for i, c := range t.Columns {
		b.WriteString(strCell(fmt.Sprintf("%s3", colName(i)), c.Label, styleHeader))
	}
	b.WriteString(`</row>`)
	rowNum := 4
	for _, row := range t.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, rowNum)
		for i, v := range row {
			if i < n {
				b.WriteString(cell(fmt.Sprintf("%s%d", colName(i), rowNum), t.Columns[i].Type, v, false))
			}
		}
		b.WriteString(`</row>`)
		rowNum++
	}
	if len(t.Totals) > 0 {
		fmt.Fprintf(&b, `<row r="%d">`, rowNum)
		for i, v := range t.Totals {
			if i < n && v != nil {
				b.WriteString(cell(fmt.Sprintf("%s%d", colName(i), rowNum), t.Columns[i].Type, v, true))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
	if len(t.Rows) > 0 {
		fmt.Fprintf(&b, `<autoFilter ref="A3:%s%d"/>`, lastCol, 3+len(t.Rows))
	}
	b.WriteString(`<pageSetup orientation="landscape" paperSize="9" fitToWidth="1"/></worksheet>`)
	return b.String()
}

func cell(ref, typ string, v any, total bool) string {
	switch x := v.(type) {
	case nil:
		return ""
	case money.Amount:
		style := styleMoney
		if total {
			style = styleTotal
		}
		return numCell(ref, centsString(int64(x)), style)
	case *money.Amount:
		if x == nil {
			return ""
		}
		return cell(ref, typ, *x, total)
	case time.Time:
		return numCell(ref, fmt.Sprintf("%d", excelDate(x)), styleDate)
	case string:
		if typ == Date {
			if d, err := time.Parse(time.DateOnly, x); err == nil {
				return numCell(ref, fmt.Sprintf("%d", excelDate(d)), styleDate)
			}
		}
		style := styleDefault
		if total {
			style = styleHeader
		}
		return strCell(ref, x, style)
	case *string:
		if x == nil {
			return ""
		}
		return cell(ref, typ, *x, total)
	case int:
		return numCell(ref, fmt.Sprintf("%d", x), styleDefault)
	case int64:
		return numCell(ref, fmt.Sprintf("%d", x), styleDefault)
	case float64:
		if typ == Percent {
			return numCell(ref, fmt.Sprintf("%.4f", x), stylePercent)
		}
		return numCell(ref, fmt.Sprintf("%g", x), styleDefault)
	case bool:
		s := "Não"
		if x {
			s = "Sim"
		}
		return strCell(ref, s, styleDefault)
	default:
		return strCell(ref, fmt.Sprint(x), styleDefault)
	}
}

func strCell(ref, s string, style int) string {
	return fmt.Sprintf(`<c r="%s" t="inlineStr" s="%d"><is><t xml:space="preserve">%s</t></is></c>`, ref, style, esc(s))
}

func numCell(ref, v string, style int) string {
	return fmt.Sprintf(`<c r="%s" s="%d"><v>%s</v></c>`, ref, style, v)
}

// centsString renders centavos as an exact decimal ("5035.00"), no floats.
func centsString(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

func excelDate(t time.Time) int {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Sub(base).Hours() / 24)
}

func colName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

func sheetName(title string) string {
	s := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, title)
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		s = "Relatório"
	}
	return s
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
