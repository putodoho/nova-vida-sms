// Package audit records who changed what. Writes go through the caller's
// transaction so the audit row commits or rolls back with the change.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
)

// Entry is one audit record.
type Entry struct {
	EntityID *uuid.UUID // nil for global records (pupils, users, ...)
	Action   string     // create, update, cancel, approve, login, ...
	Table    string
	RecordID string
	Before   any // marshalled to JSON; nil allowed
	After    any
}

type ipKey struct{}

// WithIP stores the client IP for audit rows (set by HTTP middleware).
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipKey{}, ip)
}

// Middleware copies the client IP into the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithIP(r.Context(), auth.ClientIP(r))))
	})
}

// Write inserts e using q (normally the transaction doing the change). The
// acting user comes from the context.
func Write(ctx context.Context, q db.Querier, e Entry) error {
	var userID *uuid.UUID
	if u := auth.CurrentUser(ctx); u != nil {
		id := u.ID
		userID = &id
	}
	before, err := toJSON(e.Before)
	if err != nil {
		return err
	}
	after, err := toJSON(e.After)
	if err != nil {
		return err
	}
	ip, _ := ctx.Value(ipKey{}).(string)
	_, err = q.Exec(ctx, `
		INSERT INTO audit_log (user_id, entity_id, action, table_name, record_id, before, after, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID, e.EntityID, e.Action, e.Table, e.RecordID, before, after, ip)
	if err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

func toJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
