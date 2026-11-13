// Package pdf turns HTML documents into PDF through Gotenberg (headless
// Chromium). Page size comes from the document's CSS @page rule.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Renderer converts a complete HTML document to PDF.
type Renderer interface {
	PDF(ctx context.Context, html []byte) ([]byte, error)
}

// Gotenberg calls a Gotenberg 8 service.
type Gotenberg struct {
	URL    string // e.g. http://gotenberg:3000
	Client *http.Client
}

func (g *Gotenberg) PDF(ctx context.Context, html []byte) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(html); err != nil {
		return nil, err
	}
	for k, v := range map[string]string{"preferCssPageSize": "true", "printBackground": "true"} {
		if err := mw.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.URL+"/forms/chromium/convert/html", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gotenberg: status %d: %s", resp.StatusCode, bytes.TrimSpace(out[:min(len(out), 300)]))
	}
	return out, nil
}
