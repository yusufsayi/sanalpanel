package cliapi

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"sanalpanel/internal/hesaplar"
	"sanalpanel/internal/httpx"
)

type Handlers struct{ DB *sql.DB }

// GET /db/export?databaseName=...&file=...
// "file" sadece uzantıya bakılıp gzip'lenip lenmeyeceğine karar vermek için kullanılır,
// bir dosya yolu olarak SUNUCU TARAFINDA hiç kullanılmaz — path traversal yüzeyi yok.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	domainID, _, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	dbName := r.URL.Query().Get("databaseName")
	if !hesaplar.GecerliDBKimlik(dbName) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}
	var exists int
	if err := h.DB.QueryRow(`SELECT 1 FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbName).Scan(&exists); err != nil {
		httpx.WriteError(w, http.StatusForbidden, "bu veritabanı size ait değil")
		return
	}

	gz := strings.HasSuffix(r.URL.Query().Get("file"), ".gz")
	var buf, stderr bytes.Buffer
	cmd := exec.Command("mysqldump", "--single-transaction", dbName)
	cmd.Stderr = &stderr

	if gz {
		gzw := gzip.NewWriter(&buf)
		cmd.Stdout = gzw
		runErr := cmd.Run()
		_ = gzw.Close()
		if runErr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
	} else {
		cmd.Stdout = &buf
		if err := cmd.Run(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
			return
		}
		w.Header().Set("Content-Type", "application/sql")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// POST /db/import?databaseName=... — govde ham SQL veya gzip'li SQL baytlari
// (ilk 2 byte 0x1f 0x8b ise otomatik gzip olarak algilanir, dosya uzantisina bakilmaz).
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	domainID, _, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	dbName := r.URL.Query().Get("databaseName")
	if !hesaplar.GecerliDBKimlik(dbName) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı adı")
		return
	}
	var exists int
	if err := h.DB.QueryRow(`SELECT 1 FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbName).Scan(&exists); err != nil {
		httpx.WriteError(w, http.StatusForbidden, "bu veritabanı size ait değil")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<30)) // 2GiB ust sinir
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gövde okunamadı: "+err.Error())
		return
	}

	var sqlReader io.Reader = bytes.NewReader(body)
	if isGzip(body) {
		gzr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gzip okunamadı: "+err.Error())
			return
		}
		defer gzr.Close()
		sqlReader = gzr
	}

	cmd := exec.Command("mysql", dbName)
	cmd.Stdin = sqlReader
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mysql: "+strings.TrimSpace(stderr.String()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func isGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}
