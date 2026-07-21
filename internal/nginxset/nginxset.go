// Package nginxset: per-domain nginx ayarlari (security header toggle + cache + ek direktifler)
package nginxset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type Settings struct {
	HdrXContentType bool `json:"hdr_x_content_type"`
	HdrXXSS         bool `json:"hdr_x_xss"`
	HdrReferrer     bool `json:"hdr_referrer"`
	HdrPermissions  bool `json:"hdr_permissions"`
	HdrCSPUpgrade   bool `json:"hdr_csp_upgrade"`
	HdrHSTS         bool `json:"hdr_hsts"`
	HSTSMaxAge      int  `json:"hsts_max_age"`
	HSTSSubdomains  bool `json:"hsts_subdomains"`
	HSTSPreload     bool `json:"hsts_preload"`

	// Performans onbellegi
	FastCgiCache       bool `json:"fastcgi_cache"`
	FastCgiCacheDakika int  `json:"fastcgi_cache_dakika"`
	BrowserCache       bool `json:"browser_cache"`
	BrowserCacheGun    int  `json:"browser_cache_gun"`

	EkDirektifler string `json:"ek_direktifler"`
}

func Defaults() Settings {
	return Settings{
		HdrXContentType: true, HdrXXSS: true, HdrReferrer: true,
		HdrPermissions: true, HdrCSPUpgrade: true, HdrHSTS: true,
		HSTSMaxAge: 31536000, HSTSSubdomains: true, HSTSPreload: false,
		FastCgiCache: false, FastCgiCacheDakika: 60,
		BrowserCache: true, BrowserCacheGun: 30,
		EkDirektifler: "",
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func Get(ctx context.Context, db *sql.DB, domainID int64) (Settings, error) {
	s := Defaults()
	var b1, b2, b3, b4, b5, b6, b7, b8, bFC, bBC int
	err := db.QueryRowContext(ctx,
		`SELECT hdr_x_content_type, hdr_x_xss, hdr_referrer, hdr_permissions,
		        hdr_csp_upgrade, hdr_hsts, hsts_max_age, hsts_subdomains, hsts_preload,
		        ek_direktifler, fastcgi_cache, fastcgi_cache_dakika,
		        browser_cache, browser_cache_gun
		 FROM nginx_settings WHERE domain_id=?`, domainID).
		Scan(&b1, &b2, &b3, &b4, &b5, &b6, &s.HSTSMaxAge, &b7, &b8,
			&s.EkDirektifler, &bFC, &s.FastCgiCacheDakika, &bBC, &s.BrowserCacheGun)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.HdrXContentType = b1 == 1
	s.HdrXXSS = b2 == 1
	s.HdrReferrer = b3 == 1
	s.HdrPermissions = b4 == 1
	s.HdrCSPUpgrade = b5 == 1
	s.HdrHSTS = b6 == 1
	s.HSTSSubdomains = b7 == 1
	s.HSTSPreload = b8 == 1
	s.FastCgiCache = bFC == 1
	s.BrowserCache = bBC == 1
	return s, nil
}

func Save(ctx context.Context, db *sql.DB, domainID int64, s Settings) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO nginx_settings(domain_id, hdr_x_content_type, hdr_x_xss, hdr_referrer,
		    hdr_permissions, hdr_csp_upgrade, hdr_hsts, hsts_max_age, hsts_subdomains, hsts_preload,
		    ek_direktifler, fastcgi_cache, fastcgi_cache_dakika, browser_cache, browser_cache_gun)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		    hdr_x_content_type=VALUES(hdr_x_content_type),
		    hdr_x_xss=VALUES(hdr_x_xss),
		    hdr_referrer=VALUES(hdr_referrer),
		    hdr_permissions=VALUES(hdr_permissions),
		    hdr_csp_upgrade=VALUES(hdr_csp_upgrade),
		    hdr_hsts=VALUES(hdr_hsts),
		    hsts_max_age=VALUES(hsts_max_age),
		    hsts_subdomains=VALUES(hsts_subdomains),
		    hsts_preload=VALUES(hsts_preload),
		    ek_direktifler=VALUES(ek_direktifler),
		    fastcgi_cache=VALUES(fastcgi_cache),
		    fastcgi_cache_dakika=VALUES(fastcgi_cache_dakika),
		    browser_cache=VALUES(browser_cache),
		    browser_cache_gun=VALUES(browser_cache_gun)`,
		domainID, b2i(s.HdrXContentType), b2i(s.HdrXXSS), b2i(s.HdrReferrer),
		b2i(s.HdrPermissions), b2i(s.HdrCSPUpgrade), b2i(s.HdrHSTS),
		s.HSTSMaxAge, b2i(s.HSTSSubdomains), b2i(s.HSTSPreload),
		s.EkDirektifler, b2i(s.FastCgiCache), s.FastCgiCacheDakika,
		b2i(s.BrowserCache), s.BrowserCacheGun)
	return err
}

type Handlers struct {
	DB *sql.DB
}

func (h *Handlers) Goster(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi FROM domains WHERE id=?`, id).Scan(&alanAdi); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	s, err := Get(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"alan_adi": alanAdi,
		"ayarlar":  s,
	})
}

func (h *Handlers) Kaydet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Ayarlar Settings `json:"ayarlar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	var php, sk string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT php_surum, sistem_kullanici FROM domains WHERE id=?`, id).
		Scan(&php, &sk); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	// Guvenlik: tenant ek_direktifler LFD/SSRF/RCE denylist + nginx -t sozdizim dogrulama
	if bad := provisioner.TehlikeliNginxDirektifi(req.Ayarlar.EkDirektifler); bad != "" {
		httpx.WriteError(w, http.StatusBadRequest, "güvenlik: nginx '"+bad+"' direktifine izin verilmiyor")
		return
	}
	if err := provisioner.ValidateNginxDirectives(req.Ayarlar.EkDirektifler); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz nginx direktifi: "+err.Error())
		return
	}
	if err := Save(r.Context(), h.DB, id, req.Ayarlar); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydet: "+err.Error())
		return
	}
	socket, err := provisioner.PHPSocketFor(sk, php)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "socket: "+err.Error())
		return
	}
	if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, php); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- Özel (ham) vhost modu — yalnızca admin. Bkz. internal/provisioner/provisioner.go
// renderAndReload: vhost_ozel=1 iken şablon hiç çalışmaz, aşağıdaki icerik birebir yazılır.

type vhostOzelResp struct {
	Ozel    bool   `json:"ozel"`
	Icerik  string `json:"icerik"`
	AlanAdi string `json:"alan_adi"`
}

// GET /domains/{id}/vhost-ozel — ozel=false ise icerik, panelin O AN gerçekten sunduğu
// dosyanın (disk üzerindeki) içeriğidir — admin çalışan bir kopyadan başlar (ACME
// doğrulama bloğu, redirect vb. zaten hazır), boş bir kutudan değil.
func (h *Handlers) GetVhostOzel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi, sk string
	var ozel int
	var icerikDB sql.NullString
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, COALESCE(vhost_ozel,0), vhost_ozel_icerik FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &ozel, &icerikDB); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	resp := vhostOzelResp{Ozel: ozel == 1, AlanAdi: alanAdi}
	if resp.Ozel && icerikDB.Valid {
		resp.Icerik = icerikDB.String
	} else {
		body, err := os.ReadFile("/etc/nginx/conf.d/dom_" + sk + ".conf")
		if err == nil {
			resp.Icerik = string(body)
		} else if icerikDB.Valid {
			resp.Icerik = icerikDB.String // daha önce kaydedilmiş ama şu an kapalı — onu göster
		}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type setVhostOzelReq struct {
	Ozel   bool   `json:"ozel"`
	Icerik string `json:"icerik"`
}

// PUT /domains/{id}/vhost-ozel — açarken nginx -t doğrulanır; render/nginx -t başarısız
// olursa DB eski haline geri alınır (sifrekoruma.Ekle ile aynı ekle-render-başarısız-geri-al
// deseni) — canlı dosyaya da hiç dokunulmamış olur (renderAndReload kendi backup/rollback'ini
// zaten yapıyor). Kapatırken icerik SİLİNMEZ — tekrar açılırsa kaldığı yerden devam edilir.
func (h *Handlers) SetVhostOzel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setVhostOzelReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	var php, sk string
	var oldOzel int
	var oldIcerik sql.NullString
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT php_surum, sistem_kullanici, COALESCE(vhost_ozel,0), vhost_ozel_icerik FROM domains WHERE id=?`, id).
		Scan(&php, &sk, &oldOzel, &oldIcerik); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}

	if req.Ozel {
		if err := provisioner.ValidateRawVhost(req.Icerik); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz nginx yapılandırması: "+err.Error())
			return
		}
	}

	newOzel := 0
	if req.Ozel {
		newOzel = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET vhost_ozel=?, vhost_ozel_icerik=? WHERE id=?`, newOzel, req.Icerik, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydet: "+err.Error())
		return
	}

	var applyErr error
	if socket, err := provisioner.PHPSocketFor(sk, php); err != nil {
		applyErr = err
	} else {
		applyErr = provisioner.ApplyVhostForDomain(h.DB, id, socket, php)
	}
	if applyErr != nil {
		// geri al — canlı dosya renderAndReload'un kendi backup/rollback'i sayesinde
		// zaten bozulmadı, ama DB'yi de tutarsız bırakmayalım.
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE domains SET vhost_ozel=?, vhost_ozel_icerik=? WHERE id=?`, oldOzel, oldIcerik, id)
		httpx.WriteError(w, http.StatusInternalServerError, "vhost uygulanamadı: "+applyErr.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
