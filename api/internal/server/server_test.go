package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/seed"
	"github.com/putodoho/nova-vida-sms/api/internal/server"
)

// Integration tests need TEST_DATABASE_URL pointing to a disposable database;
// the public schema is dropped and recreated.
func setup(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateAdmin(ctx, pool, "admin@test", "Admin", "password-1"); err != nil {
		t.Fatal(err)
	}
	// Receptionist with access to the primary school only.
	hash, _ := auth.HashPassword("password-2")
	if _, err := pool.Exec(ctx, `
		WITH u AS (INSERT INTO app_user (login, display_name, password_hash) VALUES ('rec@test', 'Recepção', $1) RETURNING id)
		, r AS (INSERT INTO user_role (user_id, role_id) SELECT u.id, role.id FROM u, role WHERE role.code = 'reception')
		INSERT INTO user_entity (user_id, entity_id) SELECT u.id, entity.id FROM u, entity WHERE entity.code = 'primary'`, hash); err != nil {
		t.Fatal(err)
	}
	h := server.New(server.Deps{Pool: pool, Log: log, Auth: &auth.Service{Pool: pool, TTL: time.Hour}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, pool
}

type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	entity string
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.entity != "" {
		req.Header.Set(auth.EntityHeader, c.entity)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

type meResp struct {
	User struct {
		Permissions []string    `json:"permissions"`
		EntityIDs   []uuid.UUID `json:"entity_ids"`
	} `json:"user"`
	Entities []struct {
		ID   uuid.UUID `json:"id"`
		Code string    `json:"code"`
	} `json:"entities"`
}

func entityID(t *testing.T, pool *pgxpool.Pool, code string) string {
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM entity WHERE code=$1`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestAuthFlow(t *testing.T) {
	srv, _ := setup(t)
	c := newClient(t, srv)

	if s := c.do("GET", "/api/v1/auth/me", nil, nil); s != 401 {
		t.Fatalf("me without session: %d", s)
	}
	if s := c.do("POST", "/api/v1/auth/login", map[string]string{"login": "admin@test", "password": "wrong-pass"}, nil); s != 401 {
		t.Fatalf("bad password: %d", s)
	}
	if s := c.do("POST", "/api/v1/auth/login", map[string]string{"login": "nobody@test", "password": "whatever1"}, nil); s != 401 {
		t.Fatalf("unknown user: %d", s)
	}
	var me meResp
	if s := c.do("POST", "/api/v1/auth/login", map[string]string{"login": "ADMIN@test", "password": "password-1"}, &me); s != 200 {
		t.Fatalf("login: %d", s)
	}
	if len(me.Entities) != 2 || len(me.User.Permissions) != len(auth.All) {
		t.Fatalf("admin me: %+v", me)
	}
	if me.Entities[0].Code != "primary" {
		t.Errorf("primary should come first, got %s", me.Entities[0].Code)
	}
	if s := c.do("GET", "/api/v1/auth/me", nil, &me); s != 200 {
		t.Fatalf("me: %d", s)
	}
	if s := c.do("POST", "/api/v1/auth/logout", nil, nil); s != 204 {
		t.Fatalf("logout: %d", s)
	}
	if s := c.do("GET", "/api/v1/auth/me", nil, nil); s != 401 {
		t.Fatalf("me after logout: %d", s)
	}
}

func TestEntityScopeAndPermissions(t *testing.T) {
	srv, pool := setup(t)
	primary, preschool := entityID(t, pool, "primary"), entityID(t, pool, "preschool")

	rec := newClient(t, srv)
	if s := rec.do("POST", "/api/v1/auth/login", map[string]string{"login": "rec@test", "password": "password-2"}, nil); s != 200 {
		t.Fatalf("login: %d", s)
	}

	var list struct {
		Items []struct{ Code string } `json:"items"`
	}
	// No header on GET = all accessible entities: only the primary for this user.
	if s := rec.do("GET", "/api/v1/entities", nil, &list); s != 200 || len(list.Items) != 1 || list.Items[0].Code != "primary" {
		t.Fatalf("list: %d %+v", s, list)
	}
	rec.entity = preschool
	if s := rec.do("GET", "/api/v1/entities", nil, nil); s != 403 {
		t.Fatalf("foreign entity scope: %d", s)
	}
	rec.entity = primary
	if s := rec.do("GET", "/api/v1/entities/"+preschool, nil, nil); s != 404 {
		t.Fatalf("get entity outside scope: %d", s)
	}
	// Reception lacks settings.manage.
	if s := rec.do("PUT", "/api/v1/entities/"+primary, map[string]string{}, nil); s != 403 {
		t.Fatalf("update without permission: %d", s)
	}

	admin := newClient(t, srv)
	admin.do("POST", "/api/v1/auth/login", map[string]string{"login": "admin@test", "password": "password-1"}, nil)
	body := map[string]string{
		"legal_name": "Escola Primária Nova Vida, SU, Lda.", "short_name": "Primária", "nuit": "401785507",
		"address": "Xai-Xai", "motto": "Educando para a Eternidade", "vat_method": "included_flat",
	}
	// Writes need a single entity.
	if s := admin.do("PUT", "/api/v1/entities/"+primary, body, nil); s != 400 {
		t.Fatalf("write without entity header: %d", s)
	}
	admin.entity = preschool
	// The path entity is outside the selected scope: not visible.
	if s := admin.do("PUT", "/api/v1/entities/"+primary, body, nil); s != 404 {
		t.Fatalf("write to entity other than header: %d", s)
	}
	admin.entity = primary
	bad := map[string]string{"legal_name": "", "short_name": "x", "nuit": "12", "vat_method": "nope"}
	var errResp struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if s := admin.do("PUT", "/api/v1/entities/"+primary, bad, &errResp); s != 422 || len(errResp.Error.Fields) != 3 {
		t.Fatalf("validation: %d %+v", s, errResp)
	}
	body["address"] = "Paragem Portão – Chinunguine B, Xai-Xai"
	if s := admin.do("PUT", "/api/v1/entities/"+primary, body, nil); s != 200 {
		t.Fatalf("update: %d", s)
	}

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE table_name='entity' AND action='update' AND before IS NOT NULL AND after->>'address' LIKE 'Paragem%'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rows: %d %v", n, err)
	}
}

func TestReceivingAccounts(t *testing.T) {
	srv, pool := setup(t)
	primary := entityID(t, pool, "primary")
	admin := newClient(t, srv)
	admin.do("POST", "/api/v1/auth/login", map[string]string{"login": "admin@test", "password": "password-1"}, nil)
	admin.entity = primary

	if s := admin.do("POST", "/api/v1/entities/"+primary+"/accounts", map[string]string{"kind": "mpesa", "label": "M-Pesa"}, nil); s != 422 {
		t.Fatalf("mpesa without number: %d", s)
	}
	var acc struct {
		ID string `json:"id"`
	}
	if s := admin.do("POST", "/api/v1/entities/"+primary+"/accounts",
		map[string]string{"kind": "bank", "label": "Millennium BIM", "holder": "Escola Primária Nova Vida, SU, Lda.", "number": "000", "nib": "0001"}, &acc); s != 201 {
		t.Fatalf("create: %d", s)
	}
	off := false
	if s := admin.do("PUT", "/api/v1/entities/"+primary+"/accounts/"+acc.ID,
		map[string]any{"kind": "bank", "label": "BIM", "nib": "0001", "active": off}, nil); s != 200 {
		t.Fatalf("update: %d", s)
	}
	var list struct {
		Total int `json:"total"`
	}
	admin.do("GET", "/api/v1/entities/"+primary+"/accounts", nil, &list)
	if list.Total != 2 { // seeded cash + bank
		t.Fatalf("accounts: %d", list.Total)
	}
}
