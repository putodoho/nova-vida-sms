package server_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

type paymentOut struct {
	ID               string `json:"id"`
	ReceiptNo        string `json:"receipt_no"`
	Allocated        int64  `json:"allocated"`
	Credit           int64  `json:"credit"`
	PupilOverdue     *int64 `json:"pupil_overdue"`
	PupilOutstanding *int64 `json:"pupil_outstanding"`
	CancelledAt      *string
	Allocations      []struct {
		Amount int64 `json:"amount"`
		Active bool  `json:"active"`
	} `json:"allocations"`
}

// enrolPrimary enrols a new pupil in 2.ª A with after-school and lunch (5.300 MT/month).
func enrolPrimary(t *testing.T, c *client, name string) string {
	t.Helper()
	pupil := newPupil(t, c, name)
	var cls list[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}]
	c.do("GET", "/api/v1/classes", nil, &cls)
	var fees list[feeOut]
	c.do("GET", "/api/v1/fee-items", nil, &fees)
	var class string
	for _, x := range cls.Items {
		if x.Name == "2.ª A" {
			class = x.ID
		}
	}
	var optional []string
	for _, f := range fees.Items {
		if f.Code == "after_school" || f.Code == "lunch" {
			optional = append(optional, f.ID)
		}
	}
	var years list[struct {
		ID   string `json:"id"`
		Year int    `json:"year"`
	}]
	c.do("GET", "/api/v1/school-years", nil, &years)
	var e enrolmentOut
	if s := c.do("POST", "/api/v1/enrolments", map[string]any{"pupil_id": pupil, "school_year_id": years.Items[0].ID, "class_id": class,
		"type": "new", "monthly_fee_ids": optional, "consents": map[string]bool{"fees_agreement": true}}, &e); s != 201 {
		t.Fatalf("draft: %d", s)
	}
	if s := c.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, nil); s != 200 {
		t.Fatalf("confirm: %d", s)
	}
	return pupil
}

func accountID(t *testing.T, c *client, entity, kind string) string {
	t.Helper()
	var accs list[struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}]
	c.do("GET", "/api/v1/entities/"+entity+"/accounts", nil, &accs)
	for _, a := range accs.Items {
		if a.Kind == kind {
			return a.ID
		}
	}
	t.Fatalf("no %s account", kind)
	return ""
}

func TestPayments(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	admin.do("POST", "/api/v1/entities/"+primary+"/accounts", map[string]string{"kind": "mpesa", "label": "M-Pesa", "number": "845892706"}, nil)
	admin.do("POST", "/api/v1/entities/"+primary+"/accounts", map[string]string{"kind": "bank", "label": "BIM", "nib": "0001"}, nil)
	cash, mpesa, bank := accountID(t, admin, primary, "cash"), accountID(t, admin, primary, "mpesa"), accountID(t, admin, primary, "bank")

	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	rec.entity = primary
	pupil := enrolPrimary(t, rec, "Lúcia Mabunda")

	// Preview: February (5.300) fully, then 700 of March.
	var pv struct {
		Allocations []struct {
			Amount int64 `json:"amount"`
		} `json:"allocations"`
		Credit  int64 `json:"credit"`
		Overdue int64 `json:"overdue"`
		Open    []any `json:"open"`
	}
	if s := rec.do("POST", "/api/v1/payments/preview", map[string]any{"pupil_id": pupil, "amount": 600000}, &pv); s != 200 ||
		len(pv.Allocations) != 4 || pv.Credit != 0 || pv.Overdue != 1060000 || len(pv.Open) != 30 {
		t.Fatalf("preview: %d %+v", s, pv)
	}

	var p1 paymentOut
	if s := rec.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 600000, "method": "cash", "receiving_account_id": cash}, &p1); s != 201 ||
		p1.ReceiptNo != "REC-2027-00001" || p1.Allocated != 600000 || *p1.PupilOverdue != 460000 {
		t.Fatalf("cash payment: %d %+v", s, p1)
	}

	bad := []map[string]any{
		{"pupil_id": pupil, "amount": 100000, "method": "mpesa", "receiving_account_id": mpesa},                             // no reference
		{"pupil_id": pupil, "amount": 100000, "method": "mpesa", "receiving_account_id": bank, "reference": "X1"},           // account/method mismatch
		{"pupil_id": pupil, "amount": 100000, "method": "cash", "receiving_account_id": cash, "paid_on": "2027-03-11"},      // future
		{"pupil_id": pupil, "amount": 0, "method": "cash", "receiving_account_id": cash},                                    // zero
		{"pupil_id": "00000000-0000-0000-0000-000000000000", "amount": 100, "method": "cash", "receiving_account_id": cash}, // not enrolled
	}
	for i, b := range bad {
		if s := rec.do("POST", "/api/v1/payments", b, nil); s != 422 {
			t.Errorf("bad payment %d: %d", i, s)
		}
	}
	var p2 paymentOut
	mp := map[string]any{"pupil_id": pupil, "amount": 100000, "method": "mpesa", "receiving_account_id": mpesa, "reference": "QK7H2X9", "paid_on": "2027-03-09"}
	if s := rec.do("POST", "/api/v1/payments", mp, &p2); s != 201 || p2.ReceiptNo != "REC-2027-00002" {
		t.Fatalf("mpesa: %d %+v", s, p2)
	}
	mp["reference"] = "qk7h2x9"
	if s := rec.do("POST", "/api/v1/payments", mp, nil); s != 409 {
		t.Fatalf("duplicate reference: %d", s)
	}

	// Advance payment for the rest of the year leaves 4.000 MT credit.
	var p3 paymentOut
	if s := rec.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 5000000, "method": "cash", "receiving_account_id": cash}, &p3); s != 201 ||
		p3.Credit != 400000 || *p3.PupilOutstanding != 0 {
		t.Fatalf("advance: %d %+v", s, p3)
	}
	var acc accountOut
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 0 || acc.Summary.Credit != 400000 || acc.Summary.Paid != 5300000 {
		t.Fatalf("account after advance: %+v", acc.Summary)
	}

	// Cancelling the first payment frees 6.000 MT; the 4.000 MT credit fills part of it.
	if s := rec.do("POST", "/api/v1/payments/"+p1.ID+"/cancel", map[string]string{"reason": "Valor errado"}, nil); s != 403 {
		t.Fatalf("reception cancels: %d", s)
	}
	if s := admin.do("POST", "/api/v1/payments/"+p1.ID+"/cancel", map[string]string{"reason": "Valor errado"}, &p1); s != 200 || p1.Allocated != 0 {
		t.Fatalf("cancel: %d %+v", s, p1)
	}
	if s := admin.do("POST", "/api/v1/payments/"+p1.ID+"/cancel", map[string]string{"reason": "Valor errado"}, nil); s != 409 {
		t.Fatalf("cancel twice: %d", s)
	}
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 200000 || acc.Summary.Credit != 0 {
		t.Fatalf("account after cancel: %+v", acc.Summary)
	}

	var pl struct {
		Total    int              `json:"total"`
		Sum      int64            `json:"sum"`
		ByMethod map[string]int64 `json:"by_method"`
	}
	if s := rec.do("GET", "/api/v1/payments?from=2027-03-01&to=2027-03-31", nil, &pl); s != 200 || pl.Total != 3 || pl.Sum != 5100000 ||
		pl.ByMethod["cash"] != 5000000 || pl.ByMethod["mpesa"] != 100000 {
		t.Fatalf("list: %d %+v", s, pl)
	}
	rec.do("GET", "/api/v1/payments?status=cancelled", nil, &pl)
	if pl.Total != 1 {
		t.Fatalf("cancelled filter: %+v", pl)
	}
}

func TestReceiptNumbering(t *testing.T) {
	srv, pool := setup(t)
	primary, preschool := entityID(t, pool, "primary"), entityID(t, pool, "preschool")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	cash := accountID(t, admin, primary, "cash")
	a, b := enrolPrimary(t, admin, "Ana Cossa"), enrolPrimary(t, admin, "Rui Cossa")

	// Ten concurrent payments: distinct, gap-free numbers.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var receipts []string
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pupil := a
			if i%2 == 1 {
				pupil = b
			}
			var p paymentOut
			if s := admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 10000, "method": "cash", "receiving_account_id": cash}, &p); s != 201 {
				t.Errorf("payment %d: %d", i, s)
				return
			}
			mu.Lock()
			receipts = append(receipts, p.ReceiptNo)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	sort.Strings(receipts)
	for i, r := range receipts {
		if want := fmt.Sprintf("REC-2027-%05d", i+1); r != want {
			t.Fatalf("receipts %v", receipts)
		}
	}

	// Family payment: one receipt per child, same group.
	var grp list[struct {
		ReceiptNo      string `json:"receipt_no"`
		PaymentGroupID string `json:"payment_group_id"`
	}]
	if s := admin.do("POST", "/api/v1/payments/group", map[string]any{"method": "cash", "receiving_account_id": cash,
		"splits": []map[string]any{{"pupil_id": a, "amount": 100000}, {"pupil_id": b, "amount": 50000}}}, &grp); s != 201 ||
		grp.Total != 2 || grp.Items[0].PaymentGroupID != grp.Items[1].PaymentGroupID || grp.Items[1].ReceiptNo != "REC-2027-00012" {
		t.Fatalf("group: %d %+v", s, grp)
	}

	// The preschool has its own series.
	admin.entity = preschool
	pupil := newPupil(t, admin, "Tiago Cossa")
	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	class := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = 'Sala 2'`, preschool)
	var e enrolmentOut
	admin.do("POST", "/api/v1/enrolments", map[string]any{"pupil_id": pupil, "school_year_id": year, "class_id": class, "type": "new",
		"consents": map[string]bool{"fees_agreement": true}}, &e)
	if s := admin.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, nil); s != 200 {
		t.Fatalf("preschool confirm: %d", s)
	}
	var p paymentOut
	if s := admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 300000, "method": "cash",
		"receiving_account_id": accountID(t, admin, preschool, "cash")}, &p); s != 201 || p.ReceiptNo != "REC-JI-2027-00001" {
		t.Fatalf("preschool receipt: %d %+v", s, p)
	}
	// A primary-school pupil cannot be paid through the preschool.
	if s := admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": a, "amount": 1000, "method": "cash",
		"receiving_account_id": accountID(t, admin, preschool, "cash")}, nil); s != 422 {
		t.Fatalf("cross-entity payment: %d", s)
	}
}
