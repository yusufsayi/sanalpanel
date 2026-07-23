package cliapi

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Routes: site kullanıcısı CLI'ının /api/cli/* uç noktaları. Sadece loopback-only
// listener'a mount edilmeli (bkz. cmd/server/main.go) — dışarıya asla açılmamalı.
func Routes(db *sql.DB) http.Handler {
	h := &Handlers{DB: db}
	r := chi.NewRouter()
	r.Use(RequireToken(db))
	r.Get("/db/export", h.Export)
	r.Post("/db/import", h.Import)
	r.Post("/cache/purge", h.Purge)
	return r
}
