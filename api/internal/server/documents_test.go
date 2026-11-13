package server_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// saveDoc keeps rendered HTML for visual checks when DOCS_OUT is set.
func saveDoc(t *testing.T, name string, body []byte) {
	if dir := os.Getenv("DOCS_OUT"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDocuments(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	admin.do("POST", "/api/v1/entities/"+primary+"/accounts", map[string]string{"kind": "bank", "label": "Millennium BIM",
		"holder": "Escola Primária Nova Vida, SU, Lda.", "number": "1123380960", "nib": "000100000112338096057"}, nil)
	admin.do("POST", "/api/v1/entities/"+primary+"/accounts", map[string]string{"kind": "mpesa", "label": "M-Pesa", "holder": "Escola Nova Vida", "number": "84 000 0000"}, nil)
	pupil := enrolPrimary(t, admin, "Lúcia Mabunda")
	admin.do("PUT", "/api/v1/pupils/"+pupil+"/health", map[string]any{"allergies": "Amendoim"}, nil)
	admin.do("POST", "/api/v1/penalties/run", nil, nil)

	var p paymentOut
	admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 600000, "method": "mpesa",
		"receiving_account_id": accountID(t, admin, primary, "mpesa"), "reference": "QK7H2X9"}, &p)

	s, ct, body := admin.raw("GET", "/api/v1/payments/"+p.ID+"/receipt?format=html", nil)
	if s != 200 || !strings.HasPrefix(ct, "text/html") || !bytes.Contains(body, []byte("REC-2027-00001")) ||
		!bytes.Contains(body, []byte("seis mil meticais")) || !bytes.Contains(body, []byte("QK7H2X9")) {
		t.Fatalf("receipt html: %d %s", s, ct)
	}
	saveDoc(t, "receipt.html", body)
	s, ct, body = admin.raw("GET", "/api/v1/payments/"+p.ID+"/receipt", nil)
	if s != 200 || ct != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("receipt pdf: %d %s", s, ct)
	}

	var ref struct {
		ID     string `json:"id"`
		Number string `json:"number"`
	}
	if s := admin.do("POST", "/api/v1/statements", map[string]any{"pupil_id": pupil, "year": 2027}, &ref); s != 201 || ref.Number != "EXT-2027-00001" {
		t.Fatalf("statement: %d %+v", s, ref)
	}
	s, _, body = admin.raw("GET", "/api/v1/statements/"+ref.ID+"?format=html", nil)
	for _, want := range []string{"Lúcia Mabunda", "EXT-2027-00001", "53.000,00 MT", "50.350,00 MT", "Fevereiro", "Em atraso", "REC-2027-00001", "NIB: 000100000112338096057", "5% sobre o valor mensal em atraso"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("statement missing %q", want)
		}
	}
	if s != 200 {
		t.Fatalf("statement html: %d", s)
	}
	saveDoc(t, "statement.html", body)

	class := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = '2.ª A'`, primary)
	s, _, body = admin.raw("POST", "/api/v1/statements/class?format=html", map[string]string{"class_id": class})
	if s != 200 || !bytes.Contains(body, []byte("EXT-2027-00002")) {
		t.Fatalf("class statements: %d", s)
	}
	var sl list[struct {
		Number string `json:"number"`
	}]
	if admin.do("GET", "/api/v1/statements?pupil_id="+pupil, nil, &sl); sl.Total != 2 {
		t.Fatalf("statements list: %+v", sl)
	}

	enrolment := idOf(t, pool, `SELECT id FROM enrolment WHERE pupil_id = $1`, pupil)
	s, _, body = admin.raw("GET", "/api/v1/enrolments/"+enrolment+"/form?format=html", nil)
	if s != 200 || !bytes.Contains(body, []byte("Amendoim")) || !bytes.Contains(body, []byte("Responsável pelos pagamentos")) {
		t.Fatalf("form: %d", s)
	}
	saveDoc(t, "form.html", body)

	// Cancelled receipt shows ANULADO.
	admin.do("POST", "/api/v1/payments/"+p.ID+"/cancel", map[string]string{"reason": "Teste de anulação"}, nil)
	if _, _, body = admin.raw("GET", "/api/v1/payments/"+p.ID+"/receipt?format=html", nil); !bytes.Contains(body, []byte("ANULADO")) {
		t.Fatal("cancelled receipt not marked")
	}
}
