// alias.go — mail forwarder (yönlendirme) ve catch-all yönetimi.
// Postfix mysql-virtual-aliases.cf zaten "SELECT destination FROM mail_aliases WHERE
// source='%s' AND status='active'" sorguluyor (bkz. assets/mail/postfix) — bu dosya
// yalnızca mail_aliases tablosuna CRUD ekler, OS tarafında ek kurulum gerekmez.
// Catch-all, source sütununa "@alanadi.com" (local part'sız) yazılarak temsil edilir —
// Postfix'in virtual_alias_maps araması tam adres eşleşmezse otomatik olarak "@domain"
// biçimine düşer (bkz. Postfix VIRTUAL_README, "catch-all" bölümü).
package mail

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Alias struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	CatchAll    bool   `json:"catch_all"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

var reDestEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// GET /domains/{id}/mail/aliases
func (h *Handlers) AliasListe(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, source, destination, status, created_at FROM mail_aliases WHERE domain_id=? ORDER BY source`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.Source, &a.Destination, &a.Status, &a.CreatedAt); err == nil {
			a.CatchAll = strings.HasPrefix(a.Source, "@")
			out = append(out, a)
		}
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listeleme hatası")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/mail/aliases  {local_part?, destination}
// local_part boşsa catch-all (@alanadi.com) oluşturulur.
func (h *Handlers) AliasEkle(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		LocalPart   string `json:"local_part"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}

	var alanAdi string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi FROM mail_domains WHERE domain_id=? AND durum='active'`, id).Scan(&alanAdi)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, "önce bu domain için e-postayı etkinleştirin")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mail_domains okunamadı")
		return
	}

	lp := strings.ToLower(strings.TrimSpace(req.LocalPart))
	var source string
	if lp == "" {
		source = "@" + alanAdi
	} else {
		if !reLocalPart.MatchString(lp) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz kutu adı (örn: bilgi, destek-ekibi)")
			return
		}
		source = lp + "@" + alanAdi
	}

	dest, err := normalizeDestination(req.Destination)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, d := range strings.Split(dest, ",") {
		if strings.EqualFold(d, source) {
			httpx.WriteError(w, http.StatusBadRequest, "hedef, kaynak adresle aynı olamaz (döngü)")
			return
		}
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO mail_aliases(domain_id, source, destination) VALUES(?,?,?)`, id, source, dest)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "bu kaynak için zaten bir yönlendirme var")
		return
	}
	aid, _ := res.LastInsertId()
	h.audit(r, "mail.alias.create", source+" -> "+dest, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id": aid, "source": source, "destination": dest, "catch_all": lp == "",
	})
}

// DELETE /domains/{id}/mail/aliases/{aid}
func (h *Handlers) AliasSil(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	aid, _ := strconv.ParseInt(chi.URLParam(r, "aid"), 10, 64)
	var source string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT source FROM mail_aliases WHERE id=? AND domain_id=?`, aid, id).Scan(&source); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "yönlendirme bulunamadı")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM mail_aliases WHERE id=? AND domain_id=?`, aid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	h.audit(r, "mail.alias.delete", source, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /domains/{id}/mail/aliases/{aid}/durum  {status: "active"|"suspended"}
func (h *Handlers) AliasDurumDegistir(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	aid, _ := strconv.ParseInt(chi.URLParam(r, "aid"), 10, 64)
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Status != "active" && req.Status != "suspended") {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz durum")
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE mail_aliases SET status=? WHERE id=? AND domain_id=?`, req.Status, aid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "yönlendirme bulunamadı")
		return
	}
	h.audit(r, "mail.alias.durum", strconv.FormatInt(aid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// normalizeDestination: virgülle ayrılmış hedef adresleri doğrular, boşlukları
// temizler ve kanonik ("a@b.com,c@d.com", boşluksuz) biçimde döner.
func normalizeDestination(raw string) (string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		e := strings.ToLower(strings.TrimSpace(p))
		if e == "" {
			continue
		}
		if !reDestEmail.MatchString(e) {
			return "", errors.New("geçersiz hedef e-posta adresi: " + p)
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return "", errors.New("en az bir hedef e-posta adresi girin")
	}
	return strings.Join(out, ","), nil
}
