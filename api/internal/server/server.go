// Package server builds the HTTP router.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/modules/authapi"
	"github.com/putodoho/nova-vida-sms/api/internal/modules/entities"
)

type Deps struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
	Log  *slog.Logger
}

// New returns the API handler mounted at /api.
func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(audit.Middleware)

	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.Pool.Ping(r.Context()); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	authH := &authapi.Handler{Pool: d.Pool, Auth: d.Auth}
	entitiesH := &entities.Handler{Pool: d.Pool}

	r.Route("/api/v1", func(r chi.Router) {
		authH.PublicRoutes(r)
		r.Group(func(r chi.Router) {
			r.Use(d.Auth.Authenticate)
			authH.Routes(r)
			r.Group(func(r chi.Router) {
				r.Use(auth.ScopeEntity)
				entitiesH.Routes(r)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.Fail(w, r, httpx.NotFound()) })
	return r
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if r.URL.Path == "/api/healthz" {
				return
			}
			log.Info("http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"ms", time.Since(start).Milliseconds(), "req_id", middleware.GetReqID(r.Context()))
		})
	}
}
