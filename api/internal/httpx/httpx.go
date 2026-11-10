// Package httpx contains JSON request/response helpers and the API error
// format: {"error": {"code": "...", "message": "...", "fields": {...}}}.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
)

// Error is an API error with an HTTP status and a stable machine code.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func BadRequest(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Dados inválidos.", Fields: fields}
}
func Unauthorized() *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Sessão inválida ou expirada."}
}
func Forbidden() *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "Sem permissão para esta acção."}
}
func NotFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: "Registo não encontrado."}
}
func Conflict(code, msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: msg}
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Fail writes err as an API error. Unknown errors become a 500 and are logged.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		slog.ErrorContext(r.Context(), "internal error", "err", err, "path", r.URL.Path)
		apiErr = &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "Erro interno."}
	}
	JSON(w, apiErr.Status, map[string]*Error{"error": apiErr})
}

// Decode reads a JSON body into dst, rejecting unknown fields and bodies over 1 MB.
func Decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return BadRequest("Corpo do pedido inválido: " + err.Error())
	}
	return nil
}

// Handler adapts a handler that returns an error.
type Handler func(w http.ResponseWriter, r *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		Fail(w, r, err)
	}
}

// Page holds list pagination parameters.
type Page struct {
	Page     int
	PageSize int
}

func (p Page) Offset() int { return (p.Page - 1) * p.PageSize }

// ParsePage reads ?page= and ?page_size= (default 1 and 25, max 200).
func ParsePage(r *http.Request) Page {
	p := Page{Page: 1, PageSize: 25}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		p.Page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 {
		p.PageSize = min(v, 200)
	}
	return p
}

// List is the standard list response.
type List[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
