package server_test

import (
	"bytes"
	"testing"
)

type tableOut struct {
	Title   string  `json:"title"`
	Rows    [][]any `json:"rows"`
	Totals  []any   `json:"totals"`
	Columns []struct {
		Key string `json:"key"`
	} `json:"columns"`
}

func TestDashboardAndReports(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	pupil := enrolPrimary(t, admin, "Lúcia Mabunda")
	admin.do("POST", "/api/v1/penalties/run", nil, nil)
	admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 600000, "method": "cash",
		"receiving_account_id": accountID(t, admin, primary, "cash")}, nil)

	var d struct {
		ExpectedMonth  int64            `json:"expected_month"`
		ReceivedMonth  int64            `json:"received_month"`
		CollectionRate float64          `json:"collection_rate"`
		Overdue        int64            `json:"overdue"`
		PupilsOverdue  int              `json:"pupils_overdue"`
		ActivePupils   int              `json:"active_pupils"`
		ByMethodMonth  map[string]int64 `json:"by_method_month"`
		Monthly        []struct {
			Label    string `json:"label"`
			Expected int64  `json:"expected"`
			Received int64  `json:"received"`
		} `json:"monthly"`
		TopOverdue     []any `json:"top_overdue"`
		RecentPayments []any `json:"recent_payments"`
	}
	if s := admin.do("GET", "/api/v1/reports/dashboard", nil, &d); s != 200 {
		t.Fatalf("dashboard: %d", s)
	}
	if d.ExpectedMonth != 530000 || d.ReceivedMonth != 600000 || d.ByMethodMonth["cash"] != 600000 ||
		d.Overdue != 1113000-600000 || d.PupilsOverdue != 1 || d.ActivePupils != 1 ||
		len(d.Monthly) != 10 || d.Monthly[1].Label != "Março" || d.Monthly[1].Received != 600000 || d.Monthly[0].Expected != 530000 ||
		len(d.TopOverdue) != 1 || len(d.RecentPayments) != 1 {
		t.Fatalf("dashboard: %+v", d)
	}
	if d.CollectionRate < 0.53 || d.CollectionRate > 0.55 {
		t.Fatalf("collection rate %f", d.CollectionRate)
	}

	var tb tableOut
	if s := admin.do("GET", "/api/v1/reports/income?group=month", nil, &tb); s != 200 || len(tb.Rows) != 1 || tb.Rows[0][0] != "Março 2027" {
		t.Fatalf("income: %d %+v", s, tb)
	}
	if s := admin.do("GET", "/api/v1/reports/arrears", nil, &tb); s != 200 || len(tb.Rows) != 1 || tb.Rows[0][5].(float64) != 513000 {
		t.Fatalf("arrears: %d %+v", s, tb)
	}
	if s := admin.do("GET", "/api/v1/reports/pupils", nil, &tb); s != 200 || len(tb.Rows) != 6 || tb.Totals[3].(float64) != 1 {
		t.Fatalf("pupils: %d %+v", s, tb)
	}
	if s := admin.do("GET", "/api/v1/reports/penalties", nil, &tb); s != 200 || len(tb.Rows) != 2 || tb.Totals[2].(float64) != 53000 {
		t.Fatalf("penalties: %d %+v", s, tb)
	}
	if s := admin.do("GET", "/api/v1/reports/vat", nil, &tb); s != 200 || tb.Totals[2].(float64) != 265000 {
		t.Fatalf("vat: %d %+v", s, tb) // 10 months × 265 MT (penalties carry no VAT)
	}
	if s := admin.do("GET", "/api/v1/reports/discounts", nil, &tb); s != 200 || len(tb.Rows) != 0 {
		t.Fatalf("discounts: %d %+v", s, tb)
	}

	s, ct, body := admin.raw("GET", "/api/v1/reports/arrears?format=xlsx", nil)
	if s != 200 || ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || !bytes.HasPrefix(body, []byte("PK")) {
		t.Fatalf("xlsx: %d %s", s, ct)
	}
	saveDoc(t, "arrears.xlsx", body)

	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	if s := rec.do("GET", "/api/v1/reports/dashboard", nil, nil); s != 403 {
		t.Fatalf("reception dashboard: %d", s)
	}
}
