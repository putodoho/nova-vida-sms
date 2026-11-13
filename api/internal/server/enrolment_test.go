package server_test

import (
	"context"
	"testing"
)

type enrolmentOut struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Services []struct {
		Code string `json:"code"`
	} `json:"services"`
	Totals struct {
		MonthlyGross int64 `json:"monthly_gross"`
		AnnualGross  int64 `json:"annual_gross"`
		OneOffGross  int64 `json:"one_off_gross"`
		Months       int   `json:"months"`
	} `json:"totals"`
	Preview []struct {
		Description string `json:"description"`
	} `json:"preview"`
}

type accountOut struct {
	Summary struct {
		Overdue     int64 `json:"overdue"`
		Outstanding int64 `json:"outstanding"`
		Paid        int64 `json:"paid"`
		Credit      int64 `json:"credit"`
	} `json:"summary"`
	Charges []struct {
		Status string `json:"status"`
		Kind   string `json:"kind"`
	} `json:"charges"`
}

// newPupil creates a pupil with a paying guardian and returns its id.
func newPupil(t *testing.T, c *client, name string) string {
	t.Helper()
	var p struct {
		ID     string `json:"id"`
		Number string `json:"number"`
	}
	if s := c.do("POST", "/api/v1/pupils", map[string]any{"full_name": name, "sex": "F", "birth_date": "2019-06-14", "neighbourhood": "Chinunguine"}, &p); s != 201 {
		t.Fatalf("create pupil: %d", s)
	}
	links := []map[string]any{{
		"guardian":     map[string]any{"full_name": "Sónia " + name, "phone1": "84 000 0001"},
		"relationship": "mother", "is_payer": true, "receives_notices": true, "may_pick_up": true, "lives_with": true,
	}}
	if s := c.do("PUT", "/api/v1/pupils/"+p.ID+"/guardians", links, nil); s != 200 {
		t.Fatalf("guardians: %d", s)
	}
	return p.ID
}

func TestPupilProfile(t *testing.T) {
	srv, _ := setup(t)
	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)

	var p struct {
		ID     string `json:"id"`
		Number string `json:"number"`
	}
	if s := rec.do("POST", "/api/v1/pupils", map[string]any{"full_name": "  Lúcia   Mabunda ", "sex": "F", "birth_date": "2019-06-14"}, &p); s != 201 || p.Number != "NV-2027-0001" {
		t.Fatalf("create: %d %+v", s, p)
	}
	if s := rec.do("POST", "/api/v1/pupils", map[string]any{"full_name": "X", "sex": "Z", "birth_date": "2040-01-01"}, nil); s != 422 {
		t.Fatalf("invalid pupil: %d", s)
	}
	// Two payers is invalid.
	two := []map[string]any{
		{"guardian": map[string]any{"full_name": "Mãe Teste", "phone1": "840000001"}, "relationship": "mother", "is_payer": true},
		{"guardian": map[string]any{"full_name": "Pai Teste", "phone1": "820000002"}, "relationship": "father", "is_payer": true},
	}
	if s := rec.do("PUT", "/api/v1/pupils/"+p.ID+"/guardians", two, nil); s != 422 {
		t.Fatalf("two payers: %d", s)
	}
	two[1]["is_payer"] = false
	if s := rec.do("PUT", "/api/v1/pupils/"+p.ID+"/guardians", two, nil); s != 200 {
		t.Fatalf("guardians: %d", s)
	}
	if s := rec.do("PUT", "/api/v1/pupils/"+p.ID+"/health", map[string]any{"allergies": "Amendoim", "emergency_care_consent": true}, nil); s != 200 {
		t.Fatalf("health: %d", s)
	}
	rec.do("PUT", "/api/v1/pupils/"+p.ID+"/emergency-contacts", []map[string]any{{"full_name": "Avó", "relationship": "avó", "phone": "860000000"}}, nil)

	var prof struct {
		FullName    string `json:"full_name"`
		HealthAlert bool   `json:"health_alert"`
		Health      *struct {
			Allergies string `json:"allergies"`
		} `json:"health"`
		Guardians []struct {
			GuardianID string `json:"guardian_id"`
			IsPayer    bool   `json:"is_payer"`
		} `json:"guardians"`
		EmergencyContacts []any `json:"emergency_contacts"`
	}
	if s := rec.do("GET", "/api/v1/pupils/"+p.ID, nil, &prof); s != 200 || prof.FullName != "Lúcia Mabunda" || !prof.HealthAlert ||
		prof.Health == nil || prof.Health.Allergies != "Amendoim" || len(prof.Guardians) != 2 || !prof.Guardians[0].IsPayer || len(prof.EmergencyContacts) != 1 {
		t.Fatalf("profile: %d %+v", s, prof)
	}

	// Sibling: link the existing mother found by phone search.
	var gs list[struct {
		ID string `json:"id"`
	}]
	if s := rec.do("GET", "/api/v1/guardians?q=840000001", nil, &gs); s != 200 || gs.Total != 1 {
		t.Fatalf("guardian search: %d %+v", s, gs)
	}
	var sib struct {
		ID string `json:"id"`
	}
	rec.do("POST", "/api/v1/pupils", map[string]any{"full_name": "Tiago Mabunda", "sex": "M", "birth_date": "2022-02-01"}, &sib)
	if s := rec.do("PUT", "/api/v1/pupils/"+sib.ID+"/guardians", []map[string]any{{"guardian_id": gs.Items[0].ID, "relationship": "mother", "is_payer": true}}, nil); s != 200 {
		t.Fatalf("link sibling: %d", s)
	}
	var gd struct {
		Children []any `json:"children"`
	}
	if s := rec.do("GET", "/api/v1/guardians/"+gs.Items[0].ID, nil, &gd); s != 200 || len(gd.Children) != 2 {
		t.Fatalf("guardian children: %d %+v", s, gd)
	}
	var hits list[struct {
		Number string `json:"number"`
	}]
	if s := rec.do("GET", "/api/v1/pupils/search?q=mabunda", nil, &hits); s != 200 || hits.Total != 2 {
		t.Fatalf("search: %d %+v", s, hits)
	}
}

func TestEnrolmentLifecycle(t *testing.T) {
	srv, pool := setup(t)
	primary, preschool := entityID(t, pool, "primary"), entityID(t, pool, "preschool")
	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	rec.entity = primary
	pupil := newPupil(t, rec, "Lúcia Mabunda")

	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	class := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = '2.ª A'`, primary)
	afterSchool := idOf(t, pool, `SELECT id FROM fee_item WHERE entity_id = $1 AND code = 'after_school'`, primary)
	lunch := idOf(t, pool, `SELECT id FROM fee_item WHERE entity_id = $1 AND code = 'lunch'`, primary)
	tuition4 := idOf(t, pool, `SELECT id FROM fee_item WHERE entity_id = $1 AND code = 'tuition_4'`, primary)

	body := map[string]any{"pupil_id": pupil, "school_year_id": year, "class_id": class, "type": "new", "start_month": 2,
		"monthly_fee_ids": []string{afterSchool, lunch, tuition4}, "consents": map[string]bool{"fees_agreement": true}}
	if s := rec.do("POST", "/api/v1/enrolments", body, nil); s != 422 {
		t.Fatalf("4.ª tuition in a 2.ª class: %d", s)
	}
	body["monthly_fee_ids"] = []string{afterSchool, lunch}
	var e enrolmentOut
	if s := rec.do("POST", "/api/v1/enrolments", body, &e); s != 201 {
		t.Fatalf("create draft: %d", s)
	}
	// The client's statement: 5.300 MT/month, 53.000 MT/year, 30 charges.
	if e.Status != "draft" || len(e.Services) != 3 || e.Totals.MonthlyGross != 530000 || e.Totals.AnnualGross != 5300000 ||
		e.Totals.Months != 10 || len(e.Preview) != 30 {
		t.Fatalf("draft: %+v", e)
	}
	if s := rec.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, &e); s != 200 || e.Status != "confirmed" || len(e.Preview) != 0 {
		t.Fatalf("confirm: %d %+v", s, e)
	}
	if s := rec.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, nil); s != 409 {
		t.Fatalf("confirm twice: %d", s)
	}
	if s := rec.do("PUT", "/api/v1/enrolments/"+e.ID, body, nil); s != 409 {
		t.Fatalf("edit confirmed: %d", s)
	}

	var acc accountOut
	if s := rec.do("GET", "/api/v1/pupils/"+pupil+"/account?year=2027", nil, &acc); s != 200 || len(acc.Charges) != 30 {
		t.Fatalf("account: %d %d", s, len(acc.Charges))
	}
	// Today is 10 March: February and March are overdue.
	if acc.Summary.Overdue != 1060000 || acc.Summary.Outstanding != 5300000 || acc.Summary.Credit != 0 {
		t.Fatalf("summary: %+v", acc.Summary)
	}

	var lst list[struct {
		FullName string `json:"full_name"`
		Overdue  *int64 `json:"overdue"`
	}]
	if s := rec.do("GET", "/api/v1/enrolments?debt=overdue", nil, &lst); s != 200 || lst.Total != 1 || lst.Items[0].Overdue == nil || *lst.Items[0].Overdue != 1060000 {
		t.Fatalf("list: %d %+v", s, lst)
	}

	// Drop lunch from June, then take it again from August.
	if s := rec.do("PUT", "/api/v1/enrolments/"+e.ID+"/services", map[string]any{"monthly_fee_ids": []string{afterSchool}, "from_month": 6}, &e); s != 200 {
		t.Fatalf("remove lunch: %d", s)
	}
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 5300000-6*150000 {
		t.Fatalf("after removing lunch: %+v", acc.Summary)
	}
	if s := rec.do("PUT", "/api/v1/enrolments/"+e.ID+"/services", map[string]any{"monthly_fee_ids": []string{afterSchool, lunch}, "from_month": 8}, nil); s != 200 {
		t.Fatalf("add lunch: %d", s)
	}
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 5300000-2*150000 {
		t.Fatalf("after adding lunch back: %+v", acc.Summary)
	}

	// A second enrolment of the same pupil in the same year cannot be confirmed.
	var e2 enrolmentOut
	rec.do("POST", "/api/v1/enrolments", body, &e2)
	if s := rec.do("POST", "/api/v1/enrolments/"+e2.ID+"/confirm", nil, nil); s != 409 {
		t.Fatalf("duplicate confirm: %d", s)
	}
	if s := rec.do("DELETE", "/api/v1/enrolments/"+e2.ID, nil, nil); s != 204 {
		t.Fatalf("delete draft: %d", s)
	}

	// Withdrawal on 20 April cancels everything due later.
	if s := rec.do("POST", "/api/v1/enrolments/"+e.ID+"/withdraw", map[string]any{"date": "2027-04-20", "reason": "Mudança de cidade"}, &e); s != 200 || e.Status != "withdrawn" {
		t.Fatalf("withdraw: %d %+v", s, e)
	}
	rec.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc)
	if acc.Summary.Outstanding != 3*530000 {
		t.Fatalf("after withdrawal: %+v", acc.Summary)
	}

	// The preschool scope sees none of this money.
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = preschool
	if s := admin.do("GET", "/api/v1/pupils/"+pupil+"/account", nil, &acc); s != 200 || len(acc.Charges) != 0 {
		t.Fatalf("preschool scope: %d %d", s, len(acc.Charges))
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE table_name = 'enrolment'`).Scan(&n); err != nil || n < 6 {
		t.Fatalf("audit: %d %v", n, err)
	}
}

func TestCapacityAndRenewal(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	admin.entity = primary
	ctx := context.Background()

	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	class := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = '4.ª'`, primary)
	lunch := idOf(t, pool, `SELECT id FROM fee_item WHERE entity_id = $1 AND code = 'lunch'`, primary)
	if _, err := pool.Exec(ctx, `UPDATE class SET capacity = 1 WHERE id = $1`, class); err != nil {
		t.Fatal(err)
	}
	enrol := func(pupil string) (string, int) {
		var e enrolmentOut
		admin.do("POST", "/api/v1/enrolments", map[string]any{"pupil_id": pupil, "school_year_id": year, "class_id": class,
			"type": "new", "monthly_fee_ids": []string{lunch}, "consents": map[string]bool{"fees_agreement": true}}, &e)
		return e.ID, admin.do("POST", "/api/v1/enrolments/"+e.ID+"/confirm", nil, nil)
	}
	p1, p2 := newPupil(t, admin, "Ana Cossa"), newPupil(t, admin, "Rui Cossa")
	if _, s := enrol(p1); s != 200 {
		t.Fatalf("first: %d", s)
	}
	if _, s := enrol(p2); s != 409 {
		t.Fatalf("class full: %d", s)
	}

	// Renew p1 into 2028 (catalogue copied, classes cloned by name).
	admin.do("POST", "/api/v1/school-years", map[string]any{"year": 2028, "starts_on": "2028-02-01", "ends_on": "2028-11-30",
		"billing_months": []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, "due_day": 5, "status": "planning"}, nil)
	if _, err := pool.Exec(ctx, `
		INSERT INTO class (entity_id, level_id, school_year_id, name, grade, capacity)
		SELECT entity_id, level_id, (SELECT id FROM school_year WHERE year = 2028), name, grade, 20 FROM class WHERE entity_id = $1`, primary); err != nil {
		t.Fatal(err)
	}
	admin.do("POST", "/api/v1/fee-items/copy-from-year", map[string]int{"from_year": 2027, "to_year": 2028}, nil)
	year28 := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2028`)
	class28 := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 AND name = '4.ª' AND school_year_id = $2`, primary, year28)

	var res struct {
		Results []struct {
			Status      string  `json:"status"`
			EnrolmentID *string `json:"enrolment_id"`
			Error       string  `json:"error"`
		} `json:"results"`
	}
	if s := admin.do("POST", "/api/v1/enrolments/renew", map[string]any{"school_year_id": year28, "confirm": true,
		"items": []map[string]string{{"pupil_id": p1, "class_id": class28}, {"pupil_id": p1, "class_id": class28}}}, &res); s != 200 {
		t.Fatalf("renew: %d", s)
	}
	if res.Results[0].Status != "confirmed" || res.Results[1].Status != "error" {
		t.Fatalf("renew results: %+v", res)
	}
	var e enrolmentOut
	admin.do("GET", "/api/v1/enrolments/"+*res.Results[0].EnrolmentID, nil, &e)
	codes := map[string]bool{}
	for _, s := range e.Services {
		codes[s.Code] = true
	}
	if !codes["lunch"] || !codes["tuition_4"] || codes["after_school"] {
		t.Fatalf("renewed services: %+v", e.Services)
	}
}
