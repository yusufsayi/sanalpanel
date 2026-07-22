// Package panelayarlari: panelin KENDİ erişim adresi (özel domain + otomatik Let's
// Encrypt). Tenant domainlerden farklı — vhost render ETMEZ, sabit _panel.conf'un
// (assets/nginx/_panel.conf, port 8443, server_name _;) işaret ettiği tek sertifika
// dosya çiftinin İÇERİĞİNİ değiştirir. Panel erişimi ASLA bozulmaz: her adımda
// başarısızlık self-signed'a güvenle geri döner (tenant SSL akışındaki sslFailSafe
// felsefesiyle aynı — bkz. internal/provisioner/ssl_heal.go).
package panelayarlari

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"
)

const (
	certYol      = "/etc/ssl/sanalpanel/panel.crt"
	keyYol       = "/etc/ssl/sanalpanel/panel.key"
	certYedekYol = "/etc/ssl/sanalpanel/panel.crt.selfsigned-bak"
	keyYedekYol  = "/etc/ssl/sanalpanel/panel.key.selfsigned-bak"
	acmeWebroot  = "/var/www/_acme"
	acmeBinYolu  = "/root/.acme.sh/acme.sh"
)

type Handlers struct{ DB *sql.DB }

type durumResp struct {
	OzelDomain string `json:"ozel_domain"`
	SSLDurum   string `json:"ssl_durum"`
	SSLHata    string `json:"ssl_hata,omitempty"`
	SSLBitis   string `json:"ssl_bitis,omitempty"`
	SunucuIP   string `json:"sunucu_ip"`
}

// serverIPv4: cmd/server/main.go'daki detectIPv4() ile aynı, küçük ve tek kullanım
// yerli — paylaşılan pakete çıkarmaya değmez.
func serverIPv4() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_PUBLIC_IPV4")); v != "" {
		return v
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}

func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	var resp durumResp
	var ozelDomain, sslHata, sslBitis sql.NullString
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT ozel_domain, ssl_durum, ssl_hata, COALESCE(DATE_FORMAT(ssl_bitis,'%Y-%m-%d'),'') FROM panel_ayarlari WHERE id=1`).
		Scan(&ozelDomain, &resp.SSLDurum, &sslHata, &sslBitis)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}
	resp.OzelDomain = ozelDomain.String
	resp.SSLHata = sslHata.String
	resp.SSLBitis = sslBitis.String
	resp.SunucuIP = serverIPv4()
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type kaydetReq struct {
	Domain string `json:"domain"`
}

func (h *Handlers) Kaydet(w http.ResponseWriter, r *http.Request) {
	var req kaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if err := provisioner.ValidateDomain(domain); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	sunucuIP := serverIPv4()
	if sunucuIP == "" {
		httpx.WriteError(w, http.StatusInternalServerError, "sunucu IP adresi tespit edilemedi")
		return
	}
	ips, _ := net.LookupHost(domain)
	if !contains(ips, sunucuIP) {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"\""+domain+"\" için A kaydı şu an bu sunucuyu ("+sunucuIP+") göstermiyor. "+
				"DNS kaydını ekleyip yayılmasını bekledikten sonra tekrar deneyin.")
		return
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET ozel_domain=?, ssl_durum='yok', ssl_hata=NULL WHERE id=1`, domain); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}

	sslDurum, sslHata, sslBitis := sslKur(domain)
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET ssl_durum=?, ssl_hata=?, ssl_bitis=? WHERE id=1`,
		sslDurum, nullIfEmpty(sslHata), nullIfEmpty(sslBitis)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}

	resp := map[string]any{"ok": true, "domain": domain, "ssl_durum": sslDurum}
	if sslDurum != "aktif" {
		port443VhostSil() // gerçek LE yoksa port'suz erişim asla açılmaz (bkz. vhost443.go)
		resp["uyari"] = "Domain kaydedildi ama Let's Encrypt sertifikası alınamadı: " + sslHata +
			" — panel şu an yine sunucu IP'si ve mevcut sertifikayla erişilebilir durumda."
	} else if err := port443VhostYaz(domain); err != nil {
		// SSL kuruldu, sadece port'suz-erişim adımı basarisiz oldu — istek yine de
		// basarili sayilir, :8443 zaten calisiyor.
		resp["uyari"] = "SSL kuruldu ama port'suz erişim (https://" + domain + ") ayarlanamadı: " +
			err.Error() + " — https://" + domain + ":8443 üzerinden erişebilirsiniz."
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handlers) Kaldir(w http.ResponseWriter, r *http.Request) {
	selfSignedaDon()
	port443VhostSil()
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET ozel_domain=NULL, ssl_durum='yok', ssl_hata=NULL, ssl_bitis=NULL WHERE id=1`); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sslKur: domain için gerçek bir Let's Encrypt sertifikası almayı dener ve BAŞARILIYSA
// panel.crt/.key'i yerinde günceller + nginx -t + reload eder. HERHANGİ bir adım
// başarısız olursa panel.crt/.key'e ASLA dokunulmadan (veya anında geri alınarak) döner
// — panel erişimi hiçbir koşulda kesilmez.
func sslKur(domain string) (durum, hata, bitis string) {
	_ = os.MkdirAll(acmeWebroot, 0755)
	_, _ = exec.Command("restorecon", "-R", acmeWebroot).CombinedOutput()

	issueArgs := []string{"--issue", "--webroot", acmeWebroot, "-d", domain, "--keylength", "2048"}
	issueCmd := exec.Command(acmeBinYolu, issueArgs...)
	out, err := issueCmd.CombinedOutput()
	if err != nil {
		// acme.sh exit code 2 = RENEW_SKIP: store'da zaten gecerli (yenileme penceresine
		// girmemis) bir sertifika var, YENIDEN CEKMEDI — bu GERCEK bir hata DEGIL. Bu
		// durumu hata sayip vazgecmek, biraz once tenant SSL akisinda duzeltilen "sessizce
		// self-signed'a dusme" hatasinin ayni sinifi: mevcut gecerli sertifikayla devam
		// edip install-cert adimina gecilmeli.
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 2 {
			return "basarisiz", strings.TrimSpace(string(out)), ""
		}
	}

	// Orijinal self-signed'ı SADECE ilk seferde yedekle — sonraki domain değişikliklerinde
	// üzerine yazılmaz, kalıcı "fabrika ayarı" kaçış kapısı olarak kalır.
	if _, err := os.Stat(certYedekYol); os.IsNotExist(err) {
		_ = copyFile(certYol, certYedekYol)
		_ = copyFile(keyYol, keyYedekYol)
	}

	insArgs := []string{
		"--install-cert", "-d", domain,
		"--cert-file", certYol,
		"--key-file", keyYol,
		"--fullchain-file", certYol,
	}
	if out, err := exec.Command(acmeBinYolu, insArgs...).CombinedOutput(); err != nil {
		selfSignedaDon()
		return "basarisiz", "install-cert: " + strings.TrimSpace(string(out)), ""
	}
	_ = os.Chmod(certYol, 0644)
	_ = os.Chmod(keyYol, 0600)

	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		selfSignedaDon()
		return "basarisiz", "nginx -t: " + strings.TrimSpace(string(out)), ""
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		selfSignedaDon()
		return "basarisiz", "nginx reload: " + strings.TrimSpace(string(out)), ""
	}

	bitisT, _ := certBitisOku(certYol)
	return "aktif", "", bitisT
}

// selfSignedaDon: panel.crt/.key'i yedekten geri yükler (yedek yoksa dokunmaz — henüz
// hiç LE denemesi olmamış demektir, dosyalar zaten self-signed).
func selfSignedaDon() {
	if _, err := os.Stat(certYedekYol); err != nil {
		return
	}
	_ = copyFile(certYedekYol, certYol)
	_ = copyFile(keyYedekYol, keyYol)
	_ = os.Chmod(certYol, 0644)
	_ = os.Chmod(keyYol, 0600)
	_, _ = exec.Command("nginx", "-t").CombinedOutput()
	_, _ = exec.Command("systemctl", "reload", "nginx").CombinedOutput()
}

func certBitisOku(path string) (string, error) {
	out, err := exec.Command("openssl", "x509", "-in", path, "-noout", "-enddate").CombinedOutput()
	if err != nil {
		return "", err
	}
	// "notAfter=Oct 20 10:29:33 2026 GMT" → YYYY-MM-DD'ye çevirmek için openssl'e bırak.
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "notAfter="))
	dateOut, err := exec.Command("date", "-d", s, "+%Y-%m-%d").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(dateOut)), nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0600)
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
