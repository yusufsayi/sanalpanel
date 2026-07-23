# Site Kullanıcısı CLI Komutları — Uygulama Planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Jail'li site kullanıcılarının CloudPanel'in `clpctl` site-user komutlarına eşdeğer 4 kısa komutu (`db:export`, `db:import`, `permissions:reset`, `cache:purge`) kendi SSH oturumlarından çalıştırabilmesi.

**Architecture:** `permissions:reset` jail içinde doğrudan `chmod`/`find` ile çalışır (root gerekmez). Diğer 3 komut, jail'e eklenecek bir bash dispatcher script (`sanalpanel`) üzerinden, panel sunucusunun **sadece 127.0.0.1'de dinleyen ayrı bir HTTP listener'ına** (`/api/cli/*`), domain başına üretilen bir bearer token ile istek atar. Sunucu tarafı: `mysqldump`/`mysql` (db:export/import, mevcut `db_accounts` ile sahiplik doğrulaması), Redis ACL-scoped key silme + nginx fastcgi cache-version sayacını artırıp vhost'u yeniden render/reload etme (cache:purge).

**Tech Stack:** Go (chi router, `database/sql`, `os/exec`), bash (jail dispatcher script), MariaDB migration, mevcut `internal/provisioner` nginx vhost render altyapısı.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-23-site-user-cli-design.md` — bu planın tüm kararları o spec'e dayanır.
- `/api/cli/*` **sadece 127.0.0.1'de** dinlemeli — ayrı bir `http.Server`, mevcut `:8080` genel dinleyiciden bağımsız.
- Ham CLI token hiçbir zaman DB'ye yazılmaz, sadece SHA-256 hash'i saklanır.
- `databaseName` girdisi her zaman mevcut `hesaplar.GecerliDBKimlik` ile doğrulanır ve `db_accounts` tablosundan domain sahipliği kontrol edilir.
- Migration dosyaları `IF NOT EXISTS` / idempotent olmalı (mevcut `migrations/` deseni).
- Jail template'e yapılan değişiklikler **hem** `scripts/sanalpanel-jail` **hem** `assets/ops/sanalpanel-jail` dosyasına aynı şekilde uygulanmalı (ikisi de aynı içerikte tutulan, elle senkronize edilen kopyalar — bkz. Task 10).

---

### Task 1: Migration — `cli_tokens` tablosu + `domains.cache_version` kolonu

**Files:**
- Create: `migrations/0045_site_cli.sql`

**Interfaces:**
- Produces: `cli_tokens(domain_id BIGINT PK, token_hash CHAR(64), created_at TIMESTAMP)` tablosu; `domains.cache_version INT NOT NULL DEFAULT 0` kolonu. Task 2 (`cliapi.GenerateToken`/`Lookup`) ve Task 5 (`ApplyVhostForDomain`) bunları okur/yazar.

- [ ] **Step 1: Migration dosyasını yaz**

`migrations/0045_site_cli.sql`:
```sql
-- 0045 - Site kullanicisi CLI komutlari: token tablosu + fastcgi cache-version sayaci
--
-- cli_tokens: her domain icin tek bir CLI bearer token'inin SHA-256 hash'ini tutar.
-- Ham token asla DB'ye yazilmaz — sadece /home/<sk>/.sanalpanel/token dosyasinda bulunur
-- (bkz. internal/cliapi paketi). domain_id PRIMARY KEY: domain basina tek token,
-- ON DUPLICATE KEY UPDATE ile rotasyon (cp_domain_redis ile ayni desen).
CREATE TABLE IF NOT EXISTS cli_tokens (
  domain_id  BIGINT NOT NULL PRIMARY KEY,
  token_hash CHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- cache_version: domain basina fastcgi cache-anahtarina gomulen sayac. "cache:purge"
-- CLI komutu bu sayaci artirip vhost'u yeniden render eder — diskte dosya SILMEZ,
-- eski cache girdileri sadece anahtar degistigi icin bir daha eslesmez (inactive=60m
-- ile kendiliginden temizlenir). Bkz. internal/provisioner/provisioner.go VhostOpts.CacheVersion.
ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS cache_version INT NOT NULL DEFAULT 0;
```

- [ ] **Step 2: Dosyanın mevcut migration deseniyle tutarlı olduğunu gözden geçir**

Run: `diff <(head -5 migrations/0038_waf.sql) <(head -5 migrations/0045_site_cli.sql)`
Expected: İki dosya da `-- NNNN - açıklama` yorum satırıyla başlıyor, `IF NOT EXISTS`/`ADD COLUMN IF NOT EXISTS` kullanıyor (fark komutu sadece görsel karşılaştırma için, gerçek bir pass/fail beklentisi yok — dosyanın var olduğunu ve boş olmadığını doğrula: `wc -l migrations/0045_site_cli.sql` en az 10 satır dönmeli).

- [ ] **Step 3: Commit**

```bash
git add migrations/0045_site_cli.sql
git commit -m "feat(cli): cli_tokens tablosu + domains.cache_version kolonu"
```

---

### Task 2: `internal/cliapi` paketi — token üretimi, hash, lookup, token dosyası yazımı

**Files:**
- Create: `internal/cliapi/token.go`
- Create: `internal/cliapi/token_test.go`

**Interfaces:**
- Consumes: `database/sql.DB` (mevcut panel DB handle, tüm diğer `internal/*` paketleriyle aynı şekilde).
- Produces: `GenerateToken(db *sql.DB, domainID int64) (string, error)`, `Lookup(db *sql.DB, raw string) (domainID int64, sk string, ok bool)`, `WriteTokenFile(sk, raw string, uid, gid int) error` — Task 3 (provisioning) ve Task 6 (auth middleware) bunları kullanır.

- [ ] **Step 1: Başarısız testi yaz (hashToken saflık testi)**

`internal/cliapi/token_test.go`:
```go
package cliapi

import "testing"

func TestHashTokenDeterministic(t *testing.T) {
	a := hashToken("abc123")
	b := hashToken("abc123")
	if a != b {
		t.Fatalf("hashToken aynı girdi için farklı çıktı üretti: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("hashToken 64 hex karakter (sha256) döndürmeli, uzunluk=%d", len(a))
	}
	if hashToken("abc123") == hashToken("xyz789") {
		t.Fatalf("farklı girdiler aynı hash üretti")
	}
}
```

- [ ] **Step 2: Testin derlenmediğini/başarısız olduğunu doğrula**

Run: `go test ./internal/cliapi/... -run TestHashTokenDeterministic -v`
Expected: FAIL — `undefined: hashToken` (paket henüz yok)

- [ ] **Step 3: `internal/cliapi/token.go` dosyasını yaz**

```go
// Package cliapi: site kullanıcılarının jail'den çalıştırdığı kısa CLI komutları
// (db:export, db:import, cache:purge) için sadece 127.0.0.1'de dinleyen dahili API.
package cliapi

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateToken: domain icin yeni bir CLI token uretir, hash'ini cli_tokens'a yazar,
// ham token'i dondurur (cagiran WriteTokenFile ile diske yazmali — burada saklanmaz).
func GenerateToken(db *sql.DB, domainID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(b)
	if _, err := db.Exec(
		`INSERT INTO cli_tokens (domain_id, token_hash) VALUES (?,?)
		 ON DUPLICATE KEY UPDATE token_hash=VALUES(token_hash)`,
		domainID, hashToken(raw)); err != nil {
		return "", err
	}
	return raw, nil
}

// Lookup: ham bearer token'dan domain_id + sistem_kullanici doner. Bulunamazsa ok=false.
func Lookup(db *sql.DB, raw string) (domainID int64, sk string, ok bool) {
	err := db.QueryRow(
		`SELECT ct.domain_id, d.sistem_kullanici FROM cli_tokens ct
		 JOIN domains d ON d.id = ct.domain_id
		 WHERE ct.token_hash=?`, hashToken(raw)).Scan(&domainID, &sk)
	return domainID, sk, err == nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// WriteTokenFile: ham token'i site kullanicisinin home dizinine yazar
// (/home/<sk>/.sanalpanel/token, chmod 600, sahibi sk:sk) — jail bu home'u
// bind-mount ettigi icin kullanici jail icinden okuyabilir.
func WriteTokenFile(sk, raw string, uid, gid int) error {
	dir := "/home/" + sk + "/.sanalpanel"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("dizin oluşturulamadı: %w", err)
	}
	_ = os.Chown(dir, uid, gid)
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(raw+"\n"), 0600); err != nil {
		return fmt.Errorf("token dosyası yazılamadı: %w", err)
	}
	return os.Chown(path, uid, gid)
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

Run: `go test ./internal/cliapi/... -run TestHashTokenDeterministic -v`
Expected: PASS

- [ ] **Step 5: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 6: Commit**

```bash
git add internal/cliapi/token.go internal/cliapi/token_test.go
git commit -m "feat(cliapi): CLI token üretim/hash/lookup + home dizinine yazma"
```

---

### Task 3: Domain oluşturma akışına CLI token provisioning'i ekle

**Files:**
- Modify: `internal/domains/handlers.go:226-231` (mevcut MySQL create bloğunun hemen altı)

**Interfaces:**
- Consumes: `cliapi.GenerateToken`, `cliapi.WriteTokenFile` (Task 2), mevcut `uidGidOf(pr.SistemKullanici)` (aynı fonksiyonda FTP adımı için zaten hesaplanıyor).
- Produces: Her yeni domain oluşturulduğunda `/home/<sk>/.sanalpanel/token` dosyası + `cli_tokens` satırı.

- [ ] **Step 1: Import ekle**

`internal/domains/handlers.go` dosyasının import bloğuna ekle (mevcut `"sanalpanel/internal/hesaplar"` satırının yanına, alfabetik sırayla):
```go
	"sanalpanel/internal/cliapi"
```

- [ ] **Step 2: Provisioning adımını ekle**

Mevcut kodun (bkz. `internal/domains/handlers.go:226-231`):
```go
	// 4) Default MySQL veritabanı + kullanıcı
	dbPass := hesaplar.RandomParola(24)
	if err := hesaplar.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		log.Printf("MySQL create %q hata: %v", dbName, err)
	}

	// 5) DNS şablonu otomatik tohumla + BIND zone yaz + reload
```
hemen `// 5)` yorumundan ÖNCEYE şu bloğu ekle:
```go
	// 4) Default MySQL veritabanı + kullanıcı
	dbPass := hesaplar.RandomParola(24)
	if err := hesaplar.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		log.Printf("MySQL create %q hata: %v", dbName, err)
	}

	// 4b) Site kullanıcısı CLI token'ı (db:export/import, cache:purge komutları için)
	if cliToken, err := cliapi.GenerateToken(h.DB, id); err != nil {
		log.Printf("CLI token oluştur %q hata: %v", pr.SistemKullanici, err)
	} else if err := cliapi.WriteTokenFile(pr.SistemKullanici, cliToken, uidN, gidN); err != nil {
		log.Printf("CLI token dosyası yaz %q hata: %v", pr.SistemKullanici, err)
	}

	// 5) DNS şablonu otomatik tohumla + BIND zone yaz + reload
```

- [ ] **Step 3: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir (import döngüsü yok — `cliapi` paketi `domains` paketini import etmiyor)

- [ ] **Step 4: Commit**

```bash
git add internal/domains/handlers.go
git commit -m "feat(domains): domain oluşturulurken CLI token üret ve home dizinine yaz"
```

---

### Task 4: `internal/redis` — `PurgeSK` (domain'in redis key'lerini temizleme)

**Files:**
- Modify: `internal/redis/redis.go` (yeni fonksiyon, dosya sonuna eklenir)

**Interfaces:**
- Consumes: mevcut paket-içi `cli()` helper, `reSK` regex, `adminPass()`.
- Produces: `PurgeSK(sk string) (int, error)` — Task 8 (`cache:purge` handler) bunu çağırır.

- [ ] **Step 1: Fonksiyonu ekle**

`internal/redis/redis.go` dosyasının sonuna (mevcut `disableUser` fonksiyonundan sonra) ekle. Önce dosyanın import bloğuna `"fmt"` ekle (şu an yok):

```go
import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)
```

Sonra ekle:
```go
// PurgeSK: bu domain'e ait TUM ~sk:* redis key'lerini siler (cache:purge CLI komutu icin).
// Admin yetkisiyle calisir (tenant'in kendi ACL parolasina ihtiyac yok).
func PurgeSK(sk string) (int, error) {
	if !reSK.MatchString(sk) {
		return 0, fmt.Errorf("geçersiz sistem kullanıcısı: %s", sk)
	}
	out, err := cli("--scan", "--pattern", sk+":*")
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	keys := strings.Split(out, "\n")
	args := append([]string{"DEL"}, keys...)
	if _, err := cli(args...); err != nil {
		return 0, err
	}
	return len(keys), nil
}
```

- [ ] **Step 2: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 3: Commit**

```bash
git add internal/redis/redis.go
git commit -m "feat(redis): PurgeSK — domain başına redis key temizleme"
```

---

### Task 5: `internal/provisioner` — vhost'a `CacheVersion` (fastcgi cache-key sayacı) ekle

**Files:**
- Modify: `internal/provisioner/provisioner.go` (VhostOpts struct, template, `ApplyVhostForDomain`)

**Interfaces:**
- Consumes: `domains.cache_version` kolonu (Task 1).
- Produces: `VhostOpts.CacheVersion int` alanı; her `ApplyVhostForDomain` çağrısı artık domain'in güncel `cache_version` değerini vhost'a gömer. Task 8 (`cache:purge` handler) `cache_version`'ı artırıp `ApplyVhostForDomain`'i tekrar çağırarak yeni değeri devreye sokar.

- [ ] **Step 1: `VhostOpts` struct'ına alan ekle**

`internal/provisioner/provisioner.go` içinde mevcut:
```go
	// Performans onbellegi
	FastCgiCache       bool
	FastCgiCacheDakika int
	BrowserCache       bool
	BrowserCacheGun    int
```
şu şekilde değiştir:
```go
	// Performans onbellegi
	FastCgiCache       bool
	FastCgiCacheDakika int
	BrowserCache       bool
	BrowserCacheGun    int
	CacheVersion       int // cache:purge CLI komutuyla artan sayaç; fastcgi_cache_key'e gömülür
```

- [ ] **Step 2: Template'e `fastcgi_cache_key` direktifini ekle**

Dosyada (HTTP ve HTTPS vhost blokları için) **iki kez** aynen geçen şu satırları:
```
{{.SecHeaders}}{{if .FastCgiCache}}        fastcgi_cache sanalcache;
        fastcgi_cache_valid 200 301 302 {{.FastCgiCacheDakika}}m;
```
şuna değiştir (her iki geçiş için de — `replace_all` ile tek seferde):
```
{{.SecHeaders}}{{if .FastCgiCache}}        fastcgi_cache_key "$scheme$request_method$host$request_uri:{{.CacheVersion}}";
        fastcgi_cache sanalcache;
        fastcgi_cache_valid 200 301 302 {{.FastCgiCacheDakika}}m;
```

- [ ] **Step 3: `ApplyVhostForDomain` içinde `cache_version` oku**

Mevcut:
```go
	var alanAdi, sk, certPath, keyPath, sslKaynak, backend string
	var askida int
	if err := db.QueryRow(
		`SELECT alan_adi, sistem_kullanici, COALESCE(cert_path,''), COALESCE(key_path,''), COALESCE(ssl_kaynak,''), COALESCE(web_backend,'php-fpm'), COALESCE(askida,0)
		 FROM domains WHERE id=?`, domainID).
		Scan(&alanAdi, &sk, &certPath, &keyPath, &sslKaynak, &backend, &askida); err != nil {
		return fmt.Errorf("domain bilgi cek: %w", err)
	}
```
şu şekilde değiştir:
```go
	var alanAdi, sk, certPath, keyPath, sslKaynak, backend string
	var askida, cacheVersion int
	if err := db.QueryRow(
		`SELECT alan_adi, sistem_kullanici, COALESCE(cert_path,''), COALESCE(key_path,''), COALESCE(ssl_kaynak,''), COALESCE(web_backend,'php-fpm'), COALESCE(askida,0), COALESCE(cache_version,0)
		 FROM domains WHERE id=?`, domainID).
		Scan(&alanAdi, &sk, &certPath, &keyPath, &sslKaynak, &backend, &askida, &cacheVersion); err != nil {
		return fmt.Errorf("domain bilgi cek: %w", err)
	}
```

Ve hemen altındaki `opts := VhostOpts{...}` literal'inde mevcut:
```go
		Backend:         backend,
		Askida:          askida == 1, // askıdaysa her render'da 503 vhost'u tekrar uygulanır
```
şu şekilde değiştir:
```go
		Backend:         backend,
		Askida:          askida == 1, // askıdaysa her render'da 503 vhost'u tekrar uygulanır
		CacheVersion:    cacheVersion,
```

- [ ] **Step 4: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 5: nginx template sözdizimini elle gözden geçir**

Run: `grep -n "fastcgi_cache_key\|fastcgi_cache sanalcache" internal/provisioner/provisioner.go`
Expected: `fastcgi_cache_key` satırı, her `fastcgi_cache sanalcache;` satırından hemen ÖNCE, toplam 2'şer kez (2 fastcgi_cache_key + 2 fastcgi_cache sanalcache) görünür.

- [ ] **Step 6: Commit**

```bash
git add internal/provisioner/provisioner.go
git commit -m "feat(provisioner): fastcgi_cache_key'e domain başına cache-version sayacı göm"
```

---

### Task 6: `internal/cliapi` — bearer token auth middleware

**Files:**
- Create: `internal/cliapi/middleware.go`

**Interfaces:**
- Consumes: `Lookup` (Task 2).
- Produces: `RequireToken(db *sql.DB) func(http.Handler) http.Handler`, `DomainFrom(r *http.Request) (domainID int64, sk string, ok bool)` — Task 7/8 handler'ları ve Task 9 router bunları kullanır.

- [ ] **Step 1: Middleware dosyasını yaz**

`internal/cliapi/middleware.go`:
```go
package cliapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"sanalpanel/internal/httpx"
)

type ctxKey int

const (
	ctxDomainID ctxKey = iota
	ctxSK
)

// RequireToken: "Authorization: Bearer <token>" header'ını doğrular, geçerliyse
// domain_id + sistem_kullanici'yi request context'ine koyar.
func RequireToken(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, prefix) {
				httpx.WriteError(w, http.StatusUnauthorized, "Authorization header eksik")
				return
			}
			raw := strings.TrimPrefix(auth, prefix)
			domainID, sk, ok := Lookup(db, raw)
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
				return
			}
			ctx := context.WithValue(r.Context(), ctxDomainID, domainID)
			ctx = context.WithValue(ctx, ctxSK, sk)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DomainFrom: RequireToken middleware'inin context'e koyduğu domain bilgisini okur.
func DomainFrom(r *http.Request) (domainID int64, sk string, ok bool) {
	domainID, ok1 := r.Context().Value(ctxDomainID).(int64)
	sk, ok2 := r.Context().Value(ctxSK).(string)
	return domainID, sk, ok1 && ok2
}
```

- [ ] **Step 2: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 3: Commit**

```bash
git add internal/cliapi/middleware.go
git commit -m "feat(cliapi): bearer token auth middleware"
```

---

### Task 7: `internal/cliapi` — `db:export` + `db:import` handler'ları

**Files:**
- Create: `internal/cliapi/db_handlers.go`
- Create: `internal/cliapi/db_handlers_test.go`

**Interfaces:**
- Consumes: `DomainFrom` (Task 6), `hesaplar.GecerliDBKimlik`, `db_accounts` tablosu.
- Produces: `type Handlers struct{ DB *sql.DB }`, `(*Handlers).Export(w, r)`, `(*Handlers).Import(w, r)` — Task 9 router bunları mount eder. `isGzip([]byte) bool` (paket-içi).

- [ ] **Step 1: Başarısız testi yaz (isGzip)**

`internal/cliapi/db_handlers_test.go`:
```go
package cliapi

import "testing"

func TestIsGzip(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"gzip magic", []byte{0x1f, 0x8b, 0x08, 0x00}, true},
		{"plain sql", []byte("-- MySQL dump\n"), false},
		{"empty", []byte{}, false},
		{"tek byte", []byte{0x1f}, false},
	}
	for _, c := range cases {
		if got := isGzip(c.in); got != c.want {
			t.Errorf("%s: isGzip=%v, istenen=%v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Testin başarısız olduğunu doğrula**

Run: `go test ./internal/cliapi/... -run TestIsGzip -v`
Expected: FAIL — `undefined: isGzip`

- [ ] **Step 3: `internal/cliapi/db_handlers.go` dosyasını yaz**

```go
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
```

- [ ] **Step 4: Testin geçtiğini doğrula**

Run: `go test ./internal/cliapi/... -run TestIsGzip -v`
Expected: PASS

- [ ] **Step 5: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 6: Tüm cliapi testlerinin geçtiğini doğrula**

Run: `go test ./internal/cliapi/... -v`
Expected: `TestHashTokenDeterministic` ve `TestIsGzip` PASS

- [ ] **Step 7: Commit**

```bash
git add internal/cliapi/db_handlers.go internal/cliapi/db_handlers_test.go
git commit -m "feat(cliapi): db:export + db:import handler'ları (mysqldump/mysql + sahiplik doğrulama)"
```

---

### Task 8: `internal/cliapi` — `cache:purge` handler

**Files:**
- Create: `internal/cliapi/cache_handlers.go`

**Interfaces:**
- Consumes: `DomainFrom` (Task 6), `redis.PurgeSK` (Task 4), `provisioner.PHPSocketFor` + `provisioner.ApplyVhostForDomain` (Task 5, mevcut fonksiyon).
- Produces: `(*Handlers).Purge(w, r)` — Task 9 router bunu mount eder.

- [ ] **Step 1: `internal/cliapi/cache_handlers.go` dosyasını yaz**

```go
package cliapi

import (
	"fmt"
	"net/http"
	"strings"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"
	"sanalpanel/internal/redis"
)

// POST /cache/purge  (form body: purge=all|fastcgi|redis, varsayılan "all")
func (h *Handlers) Purge(w http.ResponseWriter, r *http.Request) {
	domainID, sk, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	_ = r.ParseForm()
	purge := r.FormValue("purge")
	if purge == "" {
		purge = "all"
	}
	if purge != "all" && purge != "fastcgi" && purge != "redis" {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz purge değeri (all|fastcgi|redis)")
		return
	}

	var mesajlar []string

	if purge == "redis" || purge == "all" {
		n, err := redis.PurgeSK(sk)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "redis purge: "+err.Error())
			return
		}
		mesajlar = append(mesajlar, fmt.Sprintf("redis: %d key silindi", n))
	}

	if purge == "fastcgi" || purge == "all" {
		if _, err := h.DB.Exec(`UPDATE domains SET cache_version = cache_version + 1 WHERE id=?`, domainID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "cache_version güncelle: "+err.Error())
			return
		}
		var php string
		if err := h.DB.QueryRow(`SELECT php_surum FROM domains WHERE id=?`, domainID).Scan(&php); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "domain oku: "+err.Error())
			return
		}
		socket, err := provisioner.PHPSocketFor(sk, php)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "socket: "+err.Error())
			return
		}
		if err := provisioner.ApplyVhostForDomain(h.DB, domainID, socket, php); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "vhost yeniden render: "+err.Error())
			return
		}
		mesajlar = append(mesajlar, "fastcgi: cache-version artırıldı, nginx reload edildi")
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "mesaj": strings.Join(mesajlar, "; ")})
}
```

**Not (bilinçli basitleştirme):** `cache_version` artışı ile vhost yeniden render'ı ayrı adımlar — eğer `ApplyVhostForDomain` içindeki `nginx -t` başarısız olursa (mevcut `renderAndReload` fail-safe'i eski `.conf` dosyasını geri yükler), DB'deki sayaç bir ileride kalır ama disk/nginx davranışı değişmez (zararsız sürüklenme — bir sonraki başarılı render zaten doğru değeri gömer). Ayrı bir rollback mekanizması EKLENMEDİ (gereksiz karmaşıklık).

- [ ] **Step 2: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 3: Commit**

```bash
git add internal/cliapi/cache_handlers.go
git commit -m "feat(cliapi): cache:purge handler (redis key silme + fastcgi cache-version)"
```

---

### Task 9: Router + config + ikinci (loopback-only) HTTP listener

**Files:**
- Create: `internal/cliapi/router.go`
- Modify: `internal/config/config.go` (dosya adı repoda farklıysa: `internal/config/` altındaki tek `.go` dosyası)
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `Handlers`, `RequireToken` (Task 6), `Export`/`Import` (Task 7), `Purge` (Task 8).
- Produces: `Routes(db *sql.DB) http.Handler` — `main.go` bunu yeni bir `http.Server`'a bağlar.

- [ ] **Step 1: `internal/cliapi/router.go` dosyasını yaz**

```go
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
```

- [ ] **Step 2: Config'e loopback-only adres ekle**

`internal/config/config.go` içinde mevcut:
```go
type Config struct {
	ListenAddr  string
	DBDsn       string
	JWTSecret   []byte
	JWTLifetime int // saniye
	Env         string
}
```
şu şekilde değiştir:
```go
type Config struct {
	ListenAddr    string
	CLIListenAddr string
	DBDsn         string
	JWTSecret     []byte
	JWTLifetime   int // saniye
	Env           string
}
```
ve mevcut:
```go
	c := &Config{
		ListenAddr:  envOr("PANEL_LISTEN", ":8080"),
		DBDsn:       envOr("PANEL_DB_DSN", "panel:panelpw@unix(/var/lib/mysql/mysql.sock)/panel?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"),
		Env:         envOr("PANEL_ENV", "production"),
		JWTLifetime: envInt("PANEL_JWT_LIFETIME_SEC", 8*3600),
	}
```
şu şekilde değiştir:
```go
	c := &Config{
		ListenAddr:    envOr("PANEL_LISTEN", ":8080"),
		CLIListenAddr: envOr("PANEL_CLI_LISTEN", "127.0.0.1:8090"),
		DBDsn:         envOr("PANEL_DB_DSN", "panel:panelpw@unix(/var/lib/mysql/mysql.sock)/panel?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"),
		Env:           envOr("PANEL_ENV", "production"),
		JWTLifetime:   envInt("PANEL_JWT_LIFETIME_SEC", 8*3600),
	}
```

- [ ] **Step 3: `cmd/server/main.go` — import ekle**

Mevcut import bloğuna (`"sanalpanel/internal/backups"` satırının yanına, alfabetik sırayla) ekle:
```go
	"sanalpanel/internal/cliapi"
```

- [ ] **Step 4: İkinci HTTP server'ı başlat**

Mevcut kod (`cmd/server/main.go`, ana `srv` tanımının hemen altı):
```go
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Minute,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
```
hemen altına (mevcut `monitor.StartYukSampler(...)` satırından ÖNCE) ekle:
```go
	cliSrv := &http.Server{
		Addr:              cfg.CLIListenAddr,
		Handler:           cliapi.Routes(d),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Minute, // buyuk db:import upload'lari icin genis ust sinir
		WriteTimeout:      10 * time.Minute, // buyuk db:export indirmeleri icin genis ust sinir
		IdleTimeout:       60 * time.Second,
	}
```

- [ ] **Step 5: goroutine'de dinlemeye başla**

Mevcut kod:
```go
	go func() {
		log.Printf("sanalpanel %s — %s üzerinde dinleniyor (env=%s)", version, cfg.ListenAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()
```
hemen altına ekle:
```go
	go func() {
		log.Printf("sanalpanel CLI API — %s üzerinde dinleniyor (loopback-only)", cfg.CLIListenAddr)
		if err := cliSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("cli listen: %v", err)
		}
	}()
```

- [ ] **Step 6: Shutdown'a ekle**

Mevcut kod:
```go
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("kapatılıyor...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
```
şu şekilde değiştir:
```go
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("kapatılıyor...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := cliSrv.Shutdown(ctx); err != nil {
		log.Printf("cli shutdown: %v", err)
	}
}
```

- [ ] **Step 7: Build kontrolü**

Run: `go build ./...`
Expected: hatasız derlenir

- [ ] **Step 8: Tüm testlerin geçtiğini doğrula**

Run: `go test ./...`
Expected: PASS (mevcut testler + `internal/cliapi` testleri)

- [ ] **Step 9: Commit**

```bash
git add internal/cliapi/router.go internal/config/*.go cmd/server/main.go
git commit -m "feat(cliapi): /api/cli/* için 127.0.0.1-only ikinci HTTP listener"
```

---

### Task 10: CLI dispatcher script + jail template'e ekleme

**Files:**
- Modify: `scripts/sanalpanel-jail`
- Modify: `assets/ops/sanalpanel-jail` (aynı değişiklik — bu ikisi elle senkronize edilen kopyalar)

**Interfaces:**
- Consumes: mevcut `build_template()` fonksiyonu, `curl` (zaten `BINS` listesinde).
- Produces: her jail'de `/usr/bin/sanalpanel` komutu.

- [ ] **Step 1: Dispatcher script içeriğini `build_template()` içine ekle**

Her iki dosyada da (`scripts/sanalpanel-jail` VE `assets/ops/sanalpanel-jail`), `build_template()` fonksiyonu içinde mevcut şu satırın:
```bash
  chown -R root:root "$TEMPLATE"
  echo "template OK: $TEMPLATE ($(du -sh "$TEMPLATE" 2>/dev/null | cut -f1))"
```
HEMEN ÜSTÜNE ekle:
```bash
  # sanalpanel CLI dispatcher (CloudPanel clpctl esdegeri site-kullanicisi komutlari)
  cat > "$TEMPLATE/usr/bin/sanalpanel" <<'CLIEOF'
#!/bin/bash
# sanalpanel: site kullanicisi icin kisa komutlar (CloudPanel clpctl esdegeri)
set -euo pipefail

CLI_API="http://127.0.0.1:8090"
TOKEN_FILE="$HOME/.sanalpanel/token"

usage() {
  echo "kullanım: sanalpanel <komut> [--flag=değer ...]"
  echo "komutlar: db:export db:import permissions:reset cache:purge"
  exit 1
}

token() {
  [ -r "$TOKEN_FILE" ] || { echo "HATA: token dosyası okunamadı: $TOKEN_FILE" >&2; exit 1; }
  cat "$TOKEN_FILE"
}

parse_flag() {
  local name="$1"; shift
  for arg in "$@"; do
    case "$arg" in
      "$name"=*) printf '%s' "${arg#"$name"=}"; return 0 ;;
    esac
  done
  return 1
}

cmd="${1:-}"; shift || true

case "$cmd" in
  permissions:reset)
    dirs="770"; files="660"; path="."
    d=$(parse_flag --directories "$@") && dirs="$d"
    f=$(parse_flag --files "$@") && files="$f"
    p=$(parse_flag --path "$@") && path="$p"
    find "$path" -type d -exec chmod "$dirs" {} +
    find "$path" -type f -exec chmod "$files" {} +
    echo "izinler sıfırlandı: dizinler=$dirs dosyalar=$files yol=$path"
    ;;
  db:export)
    dbName=$(parse_flag --databaseName "$@") || { echo "HATA: --databaseName gerekli" >&2; exit 1; }
    file=$(parse_flag --file "$@") || { echo "HATA: --file gerekli" >&2; exit 1; }
    http_code=$(curl -sS -o "$file" -w '%{http_code}' \
      -H "Authorization: Bearer $(token)" \
      -G --data-urlencode "databaseName=$dbName" --data-urlencode "file=$file" \
      "$CLI_API/db/export")
    if [ "$http_code" != "200" ]; then
      echo "HATA: export başarısız (HTTP $http_code)" >&2; cat "$file" >&2; rm -f "$file"; exit 1
    fi
    echo "dışa aktarıldı: $file"
    ;;
  db:import)
    dbName=$(parse_flag --databaseName "$@") || { echo "HATA: --databaseName gerekli" >&2; exit 1; }
    file=$(parse_flag --file "$@") || { echo "HATA: --file gerekli" >&2; exit 1; }
    [ -r "$file" ] || { echo "HATA: dosya okunamadı: $file" >&2; exit 1; }
    resp=$(curl -sS -w '\n%{http_code}' \
      -H "Authorization: Bearer $(token)" \
      --data-binary "@$file" \
      "$CLI_API/db/import?databaseName=$dbName")
    http_code=$(printf '%s' "$resp" | tail -n1)
    body=$(printf '%s' "$resp" | sed '$d')
    if [ "$http_code" != "200" ]; then
      echo "HATA: import başarısız (HTTP $http_code): $body" >&2; exit 1
    fi
    echo "içe aktarıldı: $file"
    ;;
  cache:purge)
    purge=$(parse_flag --purge "$@") || purge="all"
    resp=$(curl -sS -w '\n%{http_code}' \
      -H "Authorization: Bearer $(token)" \
      -X POST --data-urlencode "purge=$purge" \
      "$CLI_API/cache/purge")
    http_code=$(printf '%s' "$resp" | tail -n1)
    body=$(printf '%s' "$resp" | sed '$d')
    if [ "$http_code" != "200" ]; then
      echo "HATA: cache purge başarısız (HTTP $http_code): $body" >&2; exit 1
    fi
    echo "$body"
    ;;
  *) usage ;;
esac
CLIEOF
  chmod 755 "$TEMPLATE/usr/bin/sanalpanel"

  chown -R root:root "$TEMPLATE"
  echo "template OK: $TEMPLATE ($(du -sh "$TEMPLATE" 2>/dev/null | cut -f1))"
```

- [ ] **Step 2: İki dosyanın birebir aynı olduğunu doğrula**

Run: `diff scripts/sanalpanel-jail assets/ops/sanalpanel-jail`
Expected: (çıktı yok — fark yok)

- [ ] **Step 3: Bash sözdizimi kontrolü**

Run: `bash -n scripts/sanalpanel-jail && bash -n assets/ops/sanalpanel-jail`
Expected: hata yok (exit 0)

- [ ] **Step 4: Commit**

```bash
git add scripts/sanalpanel-jail assets/ops/sanalpanel-jail
git commit -m "feat(jail): sanalpanel CLI dispatcher script'i jail template'e ekle"
```

---

### Task 11: Canlı VPS'te uçtan uca doğrulama (manuel)

**Files:** yok (kod değişikliği yok — sadece doğrulama)

**Interfaces:** Task 1-10'un TAMAMININ canlıda birlikte çalıştığını doğrular.

- [ ] **Step 1: Repo'yu güncelle, derle, deploy et**

```bash
git push origin main   # yalnızca yusuf onaylarsa
ssh <vps> "sanalpanel-update"
```
Expected: `sanalpanel-update` migration'ı uygular (`0045_site_cli.sql`), yeni binary'yi devreye alır, health check geçer.

- [ ] **Step 2: Jail template'i yeniden kur + mevcut bir test kullanıcısı için jail'i tazele**

```bash
ssh <vps> "sanalpanel-jail template && sanalpanel-jail setup <test_sk>"
```
Expected: "template OK" ve "setup OK" çıktısı; `ssh <vps> "test -x /home/jails/<test_sk>/usr/bin/sanalpanel && echo VAR"` → `VAR`

- [ ] **Step 3: Test domain'i için token dosyasının oluştuğunu doğrula**

```bash
ssh <vps> "cat /home/<test_sk>/.sanalpanel/token | wc -c"
```
Expected: `65` (64 hex karakter + newline) — eğer domain bu migration'dan ÖNCE oluşturulduysa dosya yoktur; bu durumda test için yeni bir domain oluştur (panel UI'den) ve onunla devam et.

- [ ] **Step 4: `permissions:reset` komutunu test et**

```bash
ssh -o ProxyCommand="ssh <vps> -W %h:%p" <test_sk>@localhost \
  "cd public_html && touch deneme.txt && chmod 777 deneme.txt && sanalpanel permissions:reset --path=. && stat -c '%a' deneme.txt"
```
(Alternatif: doğrudan jail'e `ssh <test_sk>@<vps-ip>` ile bağlanıp elle çalıştır.)
Expected: `644` (dosya, `--files=660` varsayılanına göre... NOT: varsayılan `660` — çıktı `660` olmalı, `777` değil)

- [ ] **Step 5: `db:export` komutunu test et**

Jail içinden (test kullanıcısı olarak):
```bash
sanalpanel db:export --databaseName=<test_sk>_main --file=/home/<test_sk>/yedek.sql.gz
gunzip -t /home/<test_sk>/yedek.sql.gz && echo "gzip GEÇERLİ"
zcat /home/<test_sk>/yedek.sql.gz | head -5
```
Expected: "dışa aktarıldı: ..." mesajı, "gzip GEÇERLİ", dump içeriğinde `-- MySQL dump` başlığı görünür.

- [ ] **Step 6: Yabancı bir DB adıyla `db:export`'un reddedildiğini doğrula (güvenlik testi)**

```bash
sanalpanel db:export --databaseName=panel --file=/tmp/x.sql
```
Expected: `HATA: export başarısız (HTTP 403)` — panel'in kendi (başka domain'e ait) veritabanına erişim reddedilir.

- [ ] **Step 7: `db:import` komutunu test et**

```bash
sanalpanel db:import --databaseName=<test_sk>_main --file=/home/<test_sk>/yedek.sql.gz
```
Expected: "içe aktarıldı: ..." — hatasız tamamlanır (Step 5'te alınan dump'ı kendi DB'sine geri yüklüyor, no-op'a yakın ama komutun uçtan uca çalıştığını kanıtlar).

- [ ] **Step 8: `cache:purge` komutunu test et**

Önce panel UI'den test domain'inde FastCGI cache'i aç (nginx-settings), bir sayfayı iki kez ziyaret edip `X-Cache-Status: HIT` görülene kadar bekle, sonra:
```bash
sanalpanel cache:purge --purge=fastcgi
```
Expected: "fastcgi: cache-version artırıldı, nginx reload edildi"; ardından aynı sayfayı tekrar ziyaret et → `X-Cache-Status: MISS` (yeni cache-key nedeniyle eski girdi eşleşmiyor), sunucuda `nginx -t` hata vermedi (`ssh <vps> "journalctl -u nginx --since '5 min ago' | grep -i error"` boş dönmeli).

- [ ] **Step 9: Diğer bir domain'in cache'inin etkilenmediğini doğrula (izolasyon testi)**

Test'ten önce ikinci bir domain'in `X-Cache-Status` durumunu not al, Step 8'i uygula, ikinci domain'i tekrar kontrol et.
Expected: ikinci domain `HIT` kalmaya devam eder (kendi cache-version'ı değişmedi).

- [ ] **Step 10: Sonuçları özetle**

Tüm adımlar beklenen çıktıyı verdiyse, spec'teki "Test Planı" bölümü tamamlanmış sayılır — bunu yusuf'a raporla (hangi adımlar geçti, varsa hangi adımda beklenmeyen davranış oldu).
