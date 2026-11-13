package server_test

import (
	"testing"
)

type runOut struct {
	Pupils    int   `json:"pupils"`
	Penalties int   `json:"penalties"`
	Amount    int64 `json:"amount"`
}

type chargesOut struct {
	Items []struct {
		ID          string `json:"id"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
		Status      string `json:"status"`
		DaysOverdue int    `json:"days_overdue"`
		Remaining   int64  `json:"remaining"`
	} `json:"items"`
	Total int `json:"total"`
	Aging struct {
		D1to30  int64 `json:"d1_30"`
		D31to60 int64 `json:"d31_60"`
		Total   int64 `json:"total"`
		Pupils  int   `json:"pupils"`
	} `json:"aging"`
}

func TestPenaltiesPrimary(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	pupil := enrolPrimary(t, admin, "Lúcia Mabunda")

	// 10 March: February and March are late; 5% of 5.300 MT each → 2 × 265 MT.
	var run runOut
	if s := admin.do("POST", "/api/v1/penalties/run", nil, &run); s != 200 || run.Penalties != 6 || run.Amount != 53000 || run.Pupils != 1 {
		t.Fatalf("run: %d %+v", s, run)
	}
	if admin.do("POST", "/api/v1/penalties/run", nil, &run); run.Penalties != 0 {
		t.Fatalf("second run should add nothing: %+v", run)
	}
	var acc accountOut
	admin.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 5300000+53000 || acc.Summary.Overdue != 1060000+53000 {
		t.Fatalf("account: %+v", acc.Summary)
	}

	// Arrears list and aging: February is 33 days late, March 5.
	var ch chargesOut
	if s := admin.do("GET", "/api/v1/charges?status=overdue", nil, &ch); s != 200 || ch.Total != 12 ||
		ch.Aging.D31to60 != 530000+26500 || ch.Aging.D1to30 != 530000+26500 || ch.Aging.Pupils != 1 {
		t.Fatalf("arrears: %d total=%d aging=%+v", s, ch.Total, ch.Aging)
	}
	var penalty, marchLunch string
	for _, c := range ch.Items {
		if c.Kind == "penalty" && c.Description == "Multa – Mensalidade 1.ª–3.ª classe – Março 2027" {
			penalty = c.ID
		}
		if c.Kind == "monthly" && c.Description == "Almoço – Março 2027" {
			marchLunch = c.ID
		}
	}
	if penalty == "" || marchLunch == "" {
		t.Fatalf("charges not found: %+v", ch.Items)
	}

	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	rec.entity = primary
	if s := rec.do("POST", "/api/v1/penalties/"+penalty+"/waive", map[string]string{"reason": "Primeiro atraso"}, nil); s != 403 {
		t.Fatalf("reception waives: %d", s)
	}
	if s := admin.do("POST", "/api/v1/penalties/"+marchLunch+"/waive", map[string]string{"reason": "Primeiro atraso"}, nil); s != 409 {
		t.Fatalf("waive a fee: %d", s)
	}
	if s := admin.do("POST", "/api/v1/penalties/"+penalty+"/waive", map[string]string{"reason": "Primeiro atraso"}, nil); s != 204 {
		t.Fatalf("waive: %d", s)
	}
	// A waived penalty is never charged again.
	if admin.do("POST", "/api/v1/penalties/run", nil, &run); run.Penalties != 0 {
		t.Fatalf("waived penalty recreated: %+v", run)
	}
	// Cancelling a fee cancels its penalty too.
	if s := admin.do("POST", "/api/v1/charges/"+marchLunch+"/cancel", map[string]string{"reason": "Não almoçou em Março"}, nil); s != 204 {
		t.Fatalf("cancel charge: %d", s)
	}
	admin.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if want := int64(5300000 + 53000 - 14000 - 150000 - 7500); acc.Summary.Outstanding != want {
		t.Fatalf("after waiver and cancel: %+v want %d", acc.Summary, want)
	}

	// Payment settles February's penalties before February's fees.
	var p paymentOut
	admin.do("POST", "/api/v1/payments", map[string]any{"pupil_id": pupil, "amount": 26500, "method": "cash",
		"receiving_account_id": accountID(t, admin, primary, "cash")}, &p)
	admin.do("GET", "/api/v1/charges?kind=penalty&status=paid", nil, &ch)
	if ch.Total != 3 {
		t.Fatalf("february penalties should be paid first: %+v", ch)
	}
}

func TestPenaltiesPreschoolTiers(t *testing.T) {
	srv, pool := setup(t)
	preschool := entityID(t, pool, "preschool")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = preschool
	pupil := newPupil(t, admin, "Tiago Cossa")
	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	class := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = 'Sala 2'`, preschool)
	var e enrolmentOut
	admin.do("POST", "/api/v1/enrolments", map[string]any{"pupil_id": pupil, "school_year_id": year, "class_id": class, "type": "new",
		"consents": map[string]bool{"fees_agreement": true}}, &e)
	admin.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, nil)

	// February: past the 5th and the 10th → 100 + 100 (replace mode, total 200).
	// March: past the 5th only (today is the 10th) → 100.
	var run runOut
	if s := admin.do("POST", "/api/v1/penalties/run", nil, &run); s != 200 || run.Penalties != 3 || run.Amount != 30000 {
		t.Fatalf("run: %d %+v", s, run)
	}
}

type discountReqOut struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	EstimatedAmount int64  `json:"estimated_amount"`
}

func TestDiscountRequests(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	pupil := enrolPrimary(t, admin, "Lúcia Mabunda")
	enrolment := idOf(t, pool, `SELECT id FROM enrolment WHERE pupil_id = $1`, pupil)
	siblings := idOf(t, pool, `SELECT id FROM discount_type WHERE entity_id = $1 AND code = 'siblings'`, primary)
	staffChild := idOf(t, pool, `SELECT id FROM discount_type WHERE entity_id = $1 AND code = 'staff_child'`, primary)
	admin.do("PUT", "/api/v1/discount-types/"+siblings, map[string]any{"name": "Irmãos", "mode": "percent", "value": 1000,
		"fee_item_codes": []string{"tuition_1_3", "tuition_4"}, "requires_approval": true}, nil)
	admin.do("PUT", "/api/v1/discount-types/"+staffChild, map[string]any{"name": "Filho de colaborador", "mode": "percent", "value": 5000,
		"fee_item_codes": []string{"lunch"}, "requires_approval": false}, nil)

	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	rec.entity = primary

	// 10% of the 2.800 MT tuition from April: 8 months × 280 MT.
	var dr discountReqOut
	if s := rec.do("POST", "/api/v1/discount-requests", map[string]any{"enrolment_id": enrolment, "discount_type_id": siblings,
		"from_month": 4, "reason": "Irmão na Sala 2"}, &dr); s != 201 || dr.Status != "pending" || dr.EstimatedAmount != 224000 {
		t.Fatalf("request: %d %+v", s, dr)
	}
	if s := rec.do("POST", "/api/v1/discount-requests/"+dr.ID+"/approve", map[string]string{}, nil); s != 403 {
		t.Fatalf("reception approves: %d", s)
	}
	if s := admin.do("POST", "/api/v1/discount-requests/"+dr.ID+"/reject", map[string]string{}, nil); s != 422 {
		t.Fatalf("reject without note: %d", s)
	}
	if s := admin.do("POST", "/api/v1/discount-requests/"+dr.ID+"/approve", map[string]string{"note": "Ok"}, &dr); s != 200 ||
		dr.Status != "approved" || dr.EstimatedAmount != 224000 {
		t.Fatalf("approve: %d %+v", s, dr)
	}
	if s := admin.do("POST", "/api/v1/discount-requests/"+dr.ID+"/approve", map[string]string{"note": "Ok"}, nil); s != 409 {
		t.Fatalf("approve twice: %d", s)
	}
	var acc accountOut
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 5300000-224000 {
		t.Fatalf("account: %+v", acc.Summary)
	}

	// No approval needed: applied at once (50% of lunch, 10 months × 750 MT).
	if s := rec.do("POST", "/api/v1/discount-requests", map[string]any{"enrolment_id": enrolment, "discount_type_id": staffChild,
		"reason": "Mãe é professora"}, &dr); s != 201 || dr.Status != "approved" || dr.EstimatedAmount != 750000 {
		t.Fatalf("auto approval: %d %+v", s, dr)
	}
	var lst list[discountReqOut]
	if rec.do("GET", "/api/v1/discount-requests?status=approved", nil, &lst); lst.Total != 2 {
		t.Fatalf("list: %+v", lst)
	}
}
