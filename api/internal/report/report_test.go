package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

func sample() Table {
	cls := "2.ª A"
	return Table{
		Title: "Dívidas por aluno: 2027",
		Columns: []Column{
			{"pupil", "Aluno", Text}, {"class", "Turma", Text}, {"due", "Vencimento", Date},
			{"amount", "Em dívida", Money}, {"days", "Dias", Integer}, {"rate", "Taxa", Percent},
		},
		Rows: [][]any{
			{"Lúcia Mabunda & <Irmãos>", &cls, "2027-02-05", money.Amount(530000), 33, 0.25},
			{"Tiago Cossa", nil, time.Date(2027, 3, 5, 0, 0, 0, 0, time.UTC), money.Amount(13333), int64(5), 1.0},
		},
		Totals: []any{"Total", nil, nil, money.Amount(543333), nil, nil},
	}
}

func TestColName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := colName(i); got != want {
			t.Errorf("%d: %s want %s", i, got, want)
		}
	}
	if centsString(13333) != "133.33" || centsString(-5) != "-0.05" {
		t.Error("centsString")
	}
	if excelDate(time.Date(2027, 2, 5, 0, 0, 0, 0, time.UTC)) != 46423 {
		t.Errorf("excelDate %d", excelDate(time.Date(2027, 2, 5, 0, 0, 0, 0, time.UTC)))
	}
}

// TestWriteXLSX writes a workbook; set XLSX_OUT to keep it for inspection.
func TestWriteXLSX(t *testing.T) {
	path := os.Getenv("XLSX_OUT")
	if path == "" {
		path = filepath.Join(t.TempDir(), "sample.xlsx")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteXLSX(f, sample()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Size() < 1000 {
		t.Fatalf("file too small: %d", st.Size())
	}
}
