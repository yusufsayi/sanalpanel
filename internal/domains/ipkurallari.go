// ipkurallari.go — domain bazlı IP izin/engel listesi. Liste sadece IP/CIDR tutar;
// "izin mi engel mi" anlamı domains.ip_erisim_modu kolonundan gelir (bkz.
// provisioner.buildIPRules — her render'da bu ikisini birlikte okuyup vhost'a
// server-context allow/deny bloğu olarak gömer).
package domains

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

var gecerliIPModlar = map[string]bool{"kapali": true, "engelle": true, "izin_ver": true}

type IPKural struct {
	ID        int64  `json:"id"`
	IPCidr    string `json:"ip_cidr"`
	CreatedAt string `json:"created_at"`
}

// GET /domains/{id}/ip-kurallari
func (h *Handlers) IPKurallariListe(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	var mod string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT ip_erisim_modu FROM domains WHERE id=?`, id).Scan(&mod)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, ip_cidr, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM domain_ip_kurallari WHERE domain_id=? ORDER BY id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []IPKural{}
	for rows.Next() {
		var k IPKural
		if rows.Scan(&k.ID, &k.IPCidr, &k.CreatedAt) == nil {
			out = append(out, k)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"mod": mod, "kurallar": out})
}

// PUT /domains/{id}/ip-kurallari/mod  {mod: "kapali"|"engelle"|"izin_ver"}
func (h *Handlers) IPKurallariModAyarla(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		Mod string `json:"mod"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !gecerliIPModlar[req.Mod] {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz mod (kapali|engelle|izin_ver)")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE domains SET ip_erisim_modu=? WHERE id=?`, req.Mod, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi")
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /domains/{id}/ip-kurallari  {ip_cidr}
func (h *Handlers) IPKuralEkle(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		IPCidr string `json:"ip_cidr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	ip := strings.TrimSpace(req.IPCidr)
	if net.ParseIP(ip) == nil {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz IP/CIDR (örn: 1.2.3.4 veya 1.2.3.0/24)")
			return
		}
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domain_ip_kurallari(domain_id, ip_cidr) VALUES(?,?)`, id, ip); err != nil {
		httpx.WriteError(w, http.StatusConflict, "bu IP zaten listede veya eklenemedi")
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

// DELETE /domains/{id}/ip-kurallari/{kid}
func (h *Handlers) IPKuralSil(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	kid, _ := strconv.ParseInt(chi.URLParam(r, "kid"), 10, 64)
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domain_ip_kurallari WHERE id=? AND domain_id=?`, kid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
