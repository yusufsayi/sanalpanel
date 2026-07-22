package auth

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	yescrypt "github.com/openwall/yescrypt-go"

	"sanalpanel/internal/httpx"
)

type Handlers struct {
	DB          *sql.DB
	Secret      []byte
	LifetimeSec int
}

type loginReq struct {
	Kullanici string `json:"kullanici"`
	Parola    string `json:"parola"`
	Kod       string `json:"kod"`
}

type loginResp struct {
	Token     string `json:"token"`
	Bitis     int64  `json:"bitis"`
	Kullanici struct {
		ID      int64  `json:"id"`
		Adi     string `json:"adi"`
		Rol     string `json:"rol"`
		AdSoyad string `json:"ad_soyad"`
	} `json:"kullanici"`
}

// rootShadowHash — /etc/shadow'dan root parola hash'ini okur ("" = bulunamadı).
func rootShadowHash() string {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "root:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return parts[1]
			}
			return ""
		}
	}
	return ""
}

// rootParolaDogrula — /etc/shadow'daki root hash'iyle parolayı doğrular.
//
// yescrypt ($y$) — AlmaLinux 10 varsayılanı — NATİF Go ile hesaplanır
// (github.com/openwall/yescrypt-go: yescrypt yazarlarının kendi uygulaması).
// Python 3.13'te `crypt` modülü kaldırıldığı için ana yol artık ona bağımlı
// değil; sunucu Python'u güncellediğinde panel girişi kırılmaz.
//
// Eski formatlar ($6$/$5$/$1$) için python3 yedeği korunur. Karşılaştırma
// subtle.ConstantTimeCompare ile sabit zamanlıdır (timing side-channel'a karşı).
func rootParolaDogrula(parola string) bool {
	hash := rootShadowHash()
	// Kilitli ("!", "!!", "*") veya parolasız hesap → asla kabul etme.
	if len(hash) < 3 || !strings.HasPrefix(hash, "$") {
		return false
	}
	if strings.HasPrefix(hash, "$y$") { // yescrypt → natif Go
		hesap, err := yescrypt.Hash([]byte(parola), []byte(hash))
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare(hesap, []byte(hash)) == 1
	}
	return pythonCryptDogrula(parola, hash)
}

// pythonCryptDogrula — ESKİ YOL: yalnız yescrypt-dışı formatlar (sha512/sha256/
// md5crypt) için yedek. UYARI: python3 `crypt` modülü Python 3.13'te kaldırıldı;
// bu yol o sürümde çalışmaz (yescrypt zaten ana yolda natif olduğu için etkilenmez).
func pythonCryptDogrula(parola, hash string) bool {
	cmd := exec.Command("python3", "-c",
		"import sys, crypt; p = sys.stdin.read(); sys.stdout.write(crypt.crypt(p, sys.argv[1]))",
		hash)
	cmd.Stdin = strings.NewReader(parola)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(out))), []byte(hash)) == 1
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 64KB üstü login gövdesi = kötüye kullanım (DoS)
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	req.Kullanici = strings.TrimSpace(req.Kullanici)
	if req.Kullanici == "" || req.Parola == "" {
		httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı ve parola zorunlu")
		return
	}

	// GÜVENLİK: kaba-kuvvet koruması artık router seviyesinde middleware.GirisLimiti
	// ile yapılıyor (bkz. cmd/server/main.go) — IP başına kayan pencere + kademeli
	// gecikme + kilit, bu handler her 401 dönüşünde otomatik sayılır. Eskiden burada
	// ayrı ayrı audit_log sorgulayan iki kontrol (auth.login/auth.2fa) vardı; tek
	// katmanda birleştirildi (daha güçlü: progresif gecikme + panel restart'tan
	// bağımsız DB yükü yok).
	ip := httpx.ClientIP(r)

	if req.Kullanici != "root" {
		WriteAudit(h.DB, 0, req.Kullanici, ip, "auth.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "yalnızca sunucu root kullanıcısı admin paneline giriş yapabilir")
		return
	}
	if !rootParolaDogrula(req.Parola) {
		WriteAudit(h.DB, 0, req.Kullanici, ip, "auth.login", req.Kullanici, false)
		httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı adı veya parola hatalı")
		return
	}

	// 2FA — parola doğru; 2FA açıksa TOTP kodu da gerekir.
	// FAIL-CLOSED: 2FA durumu okunamıyorsa (DB hatası) giriş REDDEDİLİR (eskiden
	// hata yutulup 2FA sessizce atlanıyordu = fail-open).
	{
		var en int
		var sec string
		var sonAdim int64
		if err := h.DB.QueryRow(`SELECT totp_enabled, totp_secret, totp_last_step FROM users WHERE id=1`).Scan(&en, &sec, &sonAdim); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "2FA durumu doğrulanamadı")
			return
		}
		if en == 1 {
			if strings.TrimSpace(sec) == "" {
				httpx.WriteError(w, http.StatusInternalServerError, "2FA yapılandırması hatalı")
				return
			}
			if strings.TrimSpace(req.Kod) == "" {
				httpx.WriteJSON(w, http.StatusOK, map[string]any{"iki_fa_gerekli": true})
				return
			}
			adim, ok := TOTPVerifyAdim(sec, req.Kod, sonAdim)
			if !ok {
				WriteAudit(h.DB, 1, "root", ip, "auth.2fa", "root", false)
				httpx.WriteError(w, http.StatusUnauthorized, "2FA kodu hatalı veya tekrar kullanıldı")
				return
			}
			_, _ = h.DB.Exec(`UPDATE users SET totp_last_step=? WHERE id=1`, adim) // replay koruması
		}
	}

	const adminUID = int64(1)
	tok, err := Issue(h.Secret, h.LifetimeSec, adminUID, "root", "admin")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token üretilemedi")
		return
	}
	WriteAudit(h.DB, adminUID, "root", ip, "auth.login", "root", true)

	resp := loginResp{Token: tok, Bitis: time.Now().Add(time.Duration(h.LifetimeSec) * time.Second).Unix()}
	resp.Kullanici.ID = adminUID
	resp.Kullanici.Adi = "root"
	resp.Kullanici.Rol = "admin"
	var adSoyad string
	_ = h.DB.QueryRow(`SELECT full_name FROM users WHERE id=1`).Scan(&adSoyad)
	resp.Kullanici.AdSoyad = adSoyad
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// WriteAudit: audit_log'a bir girişim kaydeder (login/2FA/parola vb.). Diğer
// paketler (ör. musteri) de kendi login denemelerini burada loglar — kalıcı
// izleme/analiz için tek ortak tablo.
func WriteAudit(db *sql.DB, uid int64, username, ip, action, target string, ok bool) {
	var uidVal any
	if uid > 0 {
		uidVal = uid
	}
	okv := 0
	if ok {
		okv = 1
	}
	_, _ = db.Exec(
		`INSERT INTO audit_log(actor_user_id, actor_username, ip, action, target, ok)
		 VALUES(?,?,?,?,?,?)`,
		uidVal, username, ip, action, target, okv)
}
