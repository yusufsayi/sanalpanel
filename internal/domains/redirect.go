// redirect.go — domain başına tüm-domain URL yönlendirme (redirect) editörü.
// Sadece whole-domain redirect (v1 kapsamı path bazlı değil). Uygulama, DB satırını
// yazıp provisioner.ApplyVhostForDomain'i tetiklemekten ibaret — asıl render mantığı
// (redirectVhostTmpl, öncelik: Askida > vhost_ozel > Redirect > normal) provisioner
// paketinde, her render'da domain_redirects'i okuyarak kendini korur.
package domains

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// reHedefURL: nginx "return <kod> <url>$request_uri;" içine gömülecek hedef.
// Boşluk/tırnak/noktalı virgül/parantez YASAK — vhost config injection'ı engeller
// (bkz. custom-vhost'taki admin-only ayrıcalıkla aynı risk, ama burası müşteri girdisi).
var reHedefURL = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~!$&'()*+,;=:@%/\-]*)?$`)

type Redirect struct {
	HedefURL string `json:"hedef_url"`
	Kod      int    `json:"kod"`
}

func (h *Handlers) domainInfo(r *http.Request) (id int64, sk, phpSurum string, isDemo bool, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var demo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, php_surum, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&sk, &phpSurum, &demo)
	if err != nil {
		return id, "", "", false, false
	}
	return id, sk, phpSurum, demo == 1, true
}

func (h *Handlers) applyVhost(r *http.Request, id int64, sk, phpSurum string) error {
	socket, err := provisioner.PHPSocketFor(sk, phpSurum)
	if err != nil {
		return err
	}
	return provisioner.ApplyVhostForDomain(h.DB, id, socket, phpSurum)
}

// GET /domains/{id}/yonlendirme
func (h *Handlers) YonlendirmeDurum(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	var re Redirect
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT hedef_url, kod FROM domain_redirects WHERE domain_id=?`, id).Scan(&re.HedefURL, &re.Kod)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"aktif": false})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"aktif": true, "hedef_url": re.HedefURL, "kod": re.Kod})
}

// PUT /domains/{id}/yonlendirme  {hedef_url, kod: 301|302}
func (h *Handlers) YonlendirmeAyarla(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req Redirect
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if !reHedefURL.MatchString(req.HedefURL) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz hedef URL (örn: https://ornek.com)")
		return
	}
	if req.Kod != 301 && req.Kod != 302 {
		httpx.WriteError(w, http.StatusBadRequest, "kod yalnızca 301 veya 302 olabilir")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domain_redirects(domain_id, hedef_url, kod) VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE hedef_url=VALUES(hedef_url), kod=VALUES(kod)`,
		id, req.HedefURL, req.Kod); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi: "+err.Error())
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /domains/{id}/yonlendirme
func (h *Handlers) YonlendirmeKaldir(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domain_redirects WHERE domain_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaldırılamadı")
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
