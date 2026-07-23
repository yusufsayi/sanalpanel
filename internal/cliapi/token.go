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
