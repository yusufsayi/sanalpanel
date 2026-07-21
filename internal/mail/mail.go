// Package mail: Postfix/Dovecot sanal posta kutusu yönetimi (domain başına e-posta hesabı).
// Postfix/Dovecot bu paketin oluşturduğu tabloları CANLI MySQL sorgusuyla okur — statik
// config yeniden üretimi yok, bu yüzden create/suspend/delete servis restart'sız anında etkilidir.
package mail

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"girginospanel/internal/auth"
	"girginospanel/internal/hesaplar"
	"girginospanel/internal/httpx"
	"girginospanel/internal/kota"
	"girginospanel/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB *sql.DB
}

type Mailbox struct {
	ID        int64  `json:"id"`
	LocalPart string `json:"local_part"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Durum struct {
	Etkin        bool   `json:"etkin"`
	DKIMSelector string `json:"dkim_selector,omitempty"`
}

var reLocalPart = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$`)

func (h *Handlers) domain(r *http.Request) (id int64, sk string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo); err != nil {
		return id, "", false, false
	}
	return id, sk, isDemo == 1, true
}

func (h *Handlers) audit(r *http.Request, action, target string, ok bool) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), action, target, ok)
}

// GET /domains/{id}/mail/durum — bu domain için mail etkin mi?
func (h *Handlers) MailDurum(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	var durum, selector string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT durum, dkim_selector FROM mail_domains WHERE domain_id=?`, id).Scan(&durum, &selector)
	httpx.WriteJSON(w, http.StatusOK, Durum{Etkin: err == nil && durum == "active", DKIMSelector: selector})
}

// POST /domains/{id}/mail/etkinlestir — domain için maili açar (MailUygula: OS + DNS/DKIM tohumlama).
func (h *Handlers) Etkinlestir(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if err := MailUygula(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mail etkinleştirilemedi: "+err.Error())
		return
	}
	h.audit(r, "mail.etkinlestir", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /domains/{id}/mail/etkinlestir — domain için maili devre dışı bırakır (kutular silinmez).
func (h *Handlers) Devredisi(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if err := MailKaldir(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mail devre dışı bırakılamadı: "+err.Error())
		return
	}
	h.audit(r, "mail.devredisi", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /domains/{id}/mail
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, local_part, email, status, created_at FROM mailboxes WHERE domain_id=? ORDER BY local_part`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []Mailbox{}
	for rows.Next() {
		var m Mailbox
		if err := rows.Scan(&m.ID, &m.LocalPart, &m.Email, &m.Status, &m.CreatedAt); err == nil {
			out = append(out, m)
		}
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listeleme hatası")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/mail  {local_part, parola?}
func (h *Handlers) Ekle(w http.ResponseWriter, r *http.Request) {
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
		LocalPart string `json:"local_part"`
		Parola    string `json:"parola"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	lp := strings.ToLower(strings.TrimSpace(req.LocalPart))
	if !reLocalPart.MatchString(lp) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kutu adı (örn: bilgi, destek-ekibi)")
		return
	}
	if req.Parola == "" {
		req.Parola = hesaplar.RandomParola(20)
	}
	if !hesaplar.ParolaGecerli(req.Parola) {
		httpx.WriteError(w, http.StatusBadRequest, "parola geçersiz karakter (satır sonu) içeriyor")
		return
	}

	var mdID int64
	var alanAdi, maildirRoot string
	var uidN, gidN int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, alan_adi, maildir_root, uid_n, gid_n FROM mail_domains WHERE domain_id=? AND durum='active'`, id).
		Scan(&mdID, &alanAdi, &maildirRoot, &uidN, &gidN)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, "önce bu domain için e-postayı etkinleştirin")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mail_domains okunamadı")
		return
	}

	if err := kota.CheckMailboxEklenebilir(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	email := lp + "@" + alanAdi
	hash, err := HashPassword(req.Parola)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "parola hazırlanamadı: "+err.Error())
		return
	}

	maildir := filepath.Join(maildirRoot, lp) + "/"
	if err := os.MkdirAll(maildir, 0o700); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "maildir oluşturulamadı: "+err.Error())
		return
	}
	_ = os.Chown(maildir, uidN, gidN)
	_ = exec.Command("restorecon", "-R", maildir).Run() // SELinux: user_home_t, best-effort

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO mailboxes(domain_id, mail_domain_id, local_part, email, password_hash, maildir)
		 VALUES(?,?,?,?,?,?)`,
		id, mdID, lp, email, hash, maildir)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "kutu zaten var veya eklenemedi")
		return
	}
	mid, _ := res.LastInsertId()
	h.audit(r, "mail.create", email, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id": mid, "email": email, "parola": req.Parola,
	})
}

// DELETE /domains/{id}/mail/{mid} — DB satırı silinir, Maildir diskte KALIR (yanlışlıkla
// silinen postanın kurtarılabilmesi için — backups.Create zaten tenant home'u tar'lıyor).
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var email string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT email FROM mailboxes WHERE id=? AND domain_id=?`, mid, id).Scan(&email); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "kutu bulunamadı")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM mailboxes WHERE id=? AND domain_id=?`, mid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	h.audit(r, "mail.delete", email, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// PUT /domains/{id}/mail/{mid}/parola  {parola?}
func (h *Handlers) ParolaSifirla(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req struct {
		Parola string `json:"parola"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if req.Parola == "" {
		req.Parola = hesaplar.RandomParola(20)
	}
	if !hesaplar.ParolaGecerli(req.Parola) {
		httpx.WriteError(w, http.StatusBadRequest, "parola geçersiz karakter (satır sonu) içeriyor")
		return
	}
	hash, err := HashPassword(req.Parola)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "parola hazırlanamadı: "+err.Error())
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE mailboxes SET password_hash=? WHERE id=? AND domain_id=?`, hash, mid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "kutu bulunamadı")
		return
	}
	h.audit(r, "mail.parola", strconv.FormatInt(mid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "parola": req.Parola})
}

// POST /domains/{id}/mail/{mid}/durum  {status: "active"|"suspended"}
func (h *Handlers) DurumDegistir(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Status != "active" && req.Status != "suspended") {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz durum")
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE mailboxes SET status=? WHERE id=? AND domain_id=?`, req.Status, mid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "kutu bulunamadı")
		return
	}
	h.audit(r, "mail.durum", strconv.FormatInt(mid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
