package server_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func loginAdmin(t *testing.T, c *client) {
	t.Helper()
	if s := c.do("POST", "/api/v1/auth/login", map[string]string{"login": "admin@test", "password": "password-1"}, nil); s != 200 {
		t.Fatalf("admin login: %d", s)
	}
}

func idOf(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

type list[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type apiErr struct {
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func TestSchoolYears(t *testing.T) {
	srv, _ := setup(t)
	c := newClient(t, srv)
	loginAdmin(t, c)

	var years list[map[string]any]
	if s := c.do("GET", "/api/v1/school-years", nil, &years); s != 200 || years.Total != 1 {
		t.Fatalf("list: %d %+v", s, years)
	}
	y := map[string]any{"year": 2028, "starts_on": "2028-02-01", "ends_on": "2028-11-30",
		"billing_months": []int{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "due_day": 5, "status": "planning"}
	var created struct {
		ID            string `json:"id"`
		BillingMonths []int  `json:"billing_months"`
	}
	if s := c.do("POST", "/api/v1/school-years", y, &created); s != 201 || created.BillingMonths[0] != 2 {
		t.Fatalf("create: %d %+v", s, created)
	}
	var e apiErr
	if s := c.do("POST", "/api/v1/school-years", y, &e); s != 422 || e.Error.Fields["year"] == "" {
		t.Fatalf("duplicate: %d %+v", s, e)
	}
	y["billing_months"] = []int{2, 2, 13}
	y["due_day"] = 31
	e = apiErr{}
	if s := c.do("PUT", "/api/v1/school-years/"+created.ID, y, &e); s != 422 || len(e.Error.Fields) != 2 {
		t.Fatalf("invalid update: %d %+v", s, e)
	}
}

func TestLevelsAndClasses(t *testing.T) {
	srv, pool := setup(t)
	primary, preschool := entityID(t, pool, "primary"), entityID(t, pool, "preschool")
	c := newClient(t, srv)
	loginAdmin(t, c)
	c.entity = primary

	var levels list[struct {
		ID          string         `json:"id"`
		PenaltyRule map[string]any `json:"penalty_rule"`
	}]
	if s := c.do("GET", "/api/v1/levels", nil, &levels); s != 200 || levels.Total != 1 || levels.Items[0].PenaltyRule["type"] != "percent" {
		t.Fatalf("levels: %d %+v", s, levels)
	}
	lv := levels.Items[0].ID
	if s := c.do("PUT", "/api/v1/levels/"+lv, map[string]any{"name": "Primário", "penalty_rule": map[string]any{"type": "percent", "rate_bp": 0}}, nil); s != 422 {
		t.Fatalf("invalid rule: %d", s)
	}
	rule := map[string]any{"type": "fixed_tiers", "mode": "cumulative", "grace_days": 0,
		"tiers": []map[string]any{{"after_day": 10, "amount": 20000}, {"after_day": 5, "amount": 10000}}}
	if s := c.do("PUT", "/api/v1/levels/"+lv, map[string]any{"name": "Primário", "penalty_rule": rule}, nil); s != 200 {
		t.Fatalf("update rule: %d", s)
	}

	var classes list[struct {
		Name string `json:"name"`
	}]
	if s := c.do("GET", "/api/v1/classes?year=2027", nil, &classes); s != 200 || classes.Total != 6 || classes.Items[0].Name != "1.ª A" {
		t.Fatalf("classes: %d %+v", s, classes)
	}
	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	nc := map[string]any{"level_id": lv, "school_year_id": year, "name": "5.ª", "grade": 5, "capacity": 20}
	var cls struct {
		ID string `json:"id"`
	}
	if s := c.do("POST", "/api/v1/classes", nc, &cls); s != 201 {
		t.Fatalf("create class: %d", s)
	}
	if s := c.do("POST", "/api/v1/classes", nc, nil); s != 422 {
		t.Fatalf("duplicate class: %d", s)
	}
	// A preschool level cannot be used in the primary school.
	preLevel := idOf(t, pool, `SELECT id FROM level WHERE entity_id = $1`, preschool)
	nc["level_id"], nc["name"] = preLevel, "6.ª"
	if s := c.do("POST", "/api/v1/classes", nc, nil); s != 422 {
		t.Fatalf("foreign level: %d", s)
	}
	nc["level_id"], nc["name"], nc["capacity"] = lv, "5.ª", 25
	if s := c.do("PUT", "/api/v1/classes/"+cls.ID, nc, nil); s != 200 {
		t.Fatalf("update class: %d", s)
	}
	// Same id through the preschool scope is invisible.
	c.entity = preschool
	if s := c.do("DELETE", "/api/v1/classes/"+cls.ID, nil, nil); s != 404 {
		t.Fatalf("delete outside scope: %d", s)
	}
	c.entity = primary
	if s := c.do("DELETE", "/api/v1/classes/"+cls.ID, nil, nil); s != 204 {
		t.Fatalf("delete: %d", s)
	}

	rec := newClient(t, srv)
	rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil)
	rec.entity = primary
	if s := rec.do("POST", "/api/v1/classes", nc, nil); s != 403 {
		t.Fatalf("reception create class: %d", s)
	}
}

type feeOut struct {
	ID       string   `json:"id"`
	Code     string   `json:"code"`
	ClassIDs []string `json:"class_ids"`
	Lines    struct {
		Net   int64 `json:"net"`
		VAT   int64 `json:"vat"`
		Gross int64 `json:"gross"`
	} `json:"lines"`
}

func TestFeeCatalogue(t *testing.T) {
	srv, pool := setup(t)
	primary, preschool := entityID(t, pool, "primary"), entityID(t, pool, "preschool")
	c := newClient(t, srv)
	loginAdmin(t, c)
	c.entity = primary

	var fees list[feeOut]
	if s := c.do("GET", "/api/v1/fee-items?year=2027", nil, &fees); s != 200 || fees.Total != 4 {
		t.Fatalf("list: %d %+v", s, fees)
	}
	for _, f := range fees.Items {
		if f.Code == "tuition_1_3" && (f.Lines.Net != 266000 || f.Lines.VAT != 14000 || len(f.ClassIDs) != 5) {
			t.Fatalf("tuition_1_3: %+v", f)
		}
	}

	year := idOf(t, pool, `SELECT id FROM school_year WHERE year = 2027`)
	preClass := idOf(t, pool, `SELECT id FROM class WHERE entity_id = $1 LIMIT 1`, preschool)
	fee := map[string]any{"school_year_id": year, "code": "enrolment", "name": "Matrícula", "kind": "one_off",
		"amount": 250000, "vat_mode": "none", "due_rule": map[string]any{"on_enrolment": true}}
	var created feeOut
	if s := c.do("POST", "/api/v1/fee-items", fee, &created); s != 201 {
		t.Fatalf("create: %d", s)
	}
	fee["code"], fee["due_rule"] = "uniform", map[string]any{}
	if s := c.do("POST", "/api/v1/fee-items", fee, nil); s != 422 {
		t.Fatalf("one-off without due rule: %d", s)
	}
	fee["due_rule"], fee["class_ids"] = map[string]any{"date": "2027-01-30"}, []string{preClass}
	if s := c.do("POST", "/api/v1/fee-items", fee, nil); s != 422 {
		t.Fatalf("class of another entity: %d", s)
	}
	upd := map[string]any{"name": "Matrícula", "kind": "one_off", "amount": 300000, "vat_mode": "added", "vat_rate_bp": 500,
		"due_rule": map[string]any{"on_enrolment": true}}
	var updated feeOut
	if s := c.do("PUT", "/api/v1/fee-items/"+created.ID, upd, &updated); s != 200 || updated.Lines.Gross != 315000 {
		t.Fatalf("update: %d %+v", s, updated)
	}

	// Copy to a new year: class scopes follow class names.
	c.do("POST", "/api/v1/school-years", map[string]any{"year": 2028, "starts_on": "2028-02-01", "ends_on": "2028-11-30",
		"billing_months": []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, "due_day": 5, "status": "planning"}, nil)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO class (entity_id, level_id, school_year_id, name, grade, capacity)
		SELECT entity_id, level_id, (SELECT id FROM school_year WHERE year = 2028), name, grade, capacity
		FROM class WHERE entity_id = $1`, primary); err != nil {
		t.Fatal(err)
	}
	var res struct {
		Copied  int      `json:"copied"`
		Skipped []string `json:"skipped"`
	}
	if s := c.do("POST", "/api/v1/fee-items/copy-from-year", map[string]int{"from_year": 2027, "to_year": 2028}, &res); s != 200 || res.Copied != 5 {
		t.Fatalf("copy: %d %+v", s, res)
	}
	if s := c.do("POST", "/api/v1/fee-items/copy-from-year", map[string]int{"from_year": 2027, "to_year": 2028}, &res); s != 200 || res.Copied != 0 || len(res.Skipped) != 5 {
		t.Fatalf("copy again: %d %+v", s, res)
	}
	var fees28 list[feeOut]
	c.do("GET", "/api/v1/fee-items?year=2028", nil, &fees28)
	for _, f := range fees28.Items {
		if f.Code == "tuition_4" && len(f.ClassIDs) != 1 {
			t.Fatalf("2028 tuition_4 class scope: %+v", f)
		}
	}
}

func TestDiscountTypes(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	c := newClient(t, srv)
	loginAdmin(t, c)
	c.entity = primary

	var types list[struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}]
	if s := c.do("GET", "/api/v1/discount-types", nil, &types); s != 200 || types.Total != 3 {
		t.Fatalf("list: %d %+v", s, types)
	}
	body := map[string]any{"name": "Irmãos", "mode": "percent", "value": 1000, "fee_item_codes": []string{"tuition_1_3", "tuition_4"}, "requires_approval": true}
	if s := c.do("PUT", "/api/v1/discount-types/"+types.Items[0].ID, body, nil); s != 200 {
		t.Fatalf("update: %d", s)
	}
	body["value"] = 20000
	if s := c.do("PUT", "/api/v1/discount-types/"+types.Items[0].ID, body, nil); s != 422 {
		t.Fatalf("over 100%%: %d", s)
	}
}

func TestUsersAndRoles(t *testing.T) {
	srv, pool := setup(t)
	preschool := entityID(t, pool, "preschool")
	admin := newClient(t, srv)
	loginAdmin(t, admin)
	adminID := idOf(t, pool, `SELECT id FROM app_user WHERE login = 'admin@test'`)
	adminRole := idOf(t, pool, `SELECT id FROM role WHERE code = 'admin'`)
	receptionRole := idOf(t, pool, `SELECT id FROM role WHERE code = 'reception'`)

	var staff struct {
		ID string `json:"id"`
	}
	if s := admin.do("POST", "/api/v1/staff", map[string]string{"full_name": "Benigna", "function": "Secretaria", "phone": "840000000"}, &staff); s != 201 {
		t.Fatalf("staff: %d", s)
	}
	u := map[string]any{"login": "Benigna@Test", "display_name": "Benigna", "staff_id": staff.ID, "password": "secret-123",
		"role_ids": []string{receptionRole}, "entity_ids": []string{preschool}}
	var user struct {
		ID    string `json:"id"`
		Login string `json:"login"`
	}
	if s := admin.do("POST", "/api/v1/users", u, &user); s != 201 || user.Login != "benigna@test" {
		t.Fatalf("create user: %d %+v", s, user)
	}
	if s := admin.do("POST", "/api/v1/users", u, nil); s != 422 {
		t.Fatalf("duplicate login: %d", s)
	}

	b := newClient(t, srv)
	var me meResp
	if s := b.do("POST", "/api/v1/auth/login", map[string]string{"login": "benigna@test", "password": "secret-123"}, &me); s != 200 || len(me.Entities) != 1 {
		t.Fatalf("new user login: %d %+v", s, me)
	}
	if s := b.do("GET", "/api/v1/users", nil, nil); s != 403 {
		t.Fatalf("reception lists users: %d", s)
	}

	// Lock-out guards.
	self := map[string]any{"display_name": "Admin", "active": false, "role_ids": []string{adminRole}, "entity_ids": me.User.EntityIDs}
	if s := admin.do("PUT", "/api/v1/users/"+adminID, self, nil); s != 409 {
		t.Fatalf("self deactivate: %d", s)
	}
	self["active"], self["role_ids"] = true, []string{receptionRole}
	if s := admin.do("PUT", "/api/v1/users/"+adminID, self, nil); s != 409 {
		t.Fatalf("remove last admin: %d", s)
	}

	// Deactivation ends the user's sessions.
	u["active"] = false
	if s := admin.do("PUT", "/api/v1/users/"+user.ID, u, nil); s != 200 {
		t.Fatalf("deactivate: %d", s)
	}
	if s := b.do("GET", "/api/v1/auth/me", nil, nil); s != 401 {
		t.Fatalf("deactivated session still valid: %d", s)
	}

	// Roles.
	var role struct {
		ID string `json:"id"`
	}
	if s := admin.do("POST", "/api/v1/roles", map[string]any{"code": "tesouraria", "name": "Tesouraria", "permissions": []string{"payments.create", "charges.read"}}, &role); s != 201 {
		t.Fatalf("create role: %d", s)
	}
	if s := admin.do("POST", "/api/v1/roles", map[string]any{"code": "x1", "name": "X", "permissions": []string{"nope"}}, nil); s != 422 {
		t.Fatalf("unknown permission: %d", s)
	}
	if s := admin.do("PUT", "/api/v1/roles/"+adminRole, map[string]any{"name": "Administrador", "permissions": []string{"pupils.read"}}, nil); s != 409 {
		t.Fatalf("shrink admin role: %d", s)
	}
	if s := admin.do("DELETE", "/api/v1/roles/"+receptionRole, nil, nil); s != 409 {
		t.Fatalf("delete system role: %d", s)
	}
	if s := admin.do("DELETE", "/api/v1/roles/"+role.ID, nil, nil); s != 204 {
		t.Fatalf("delete custom role: %d", s)
	}

	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE table_name IN ('app_user','role','staff')`).Scan(&n); err != nil || n < 5 {
		t.Fatalf("audit rows: %d %v", n, err)
	}
}
