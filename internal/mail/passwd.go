package mail

import (
	"fmt"
	"os/exec"
	"strings"
)

// HashPassword: Dovecot'un SHA512-CRYPT şemasıyla uyumlu $6$... hash üretir. openssl
// zaten bu panelin sabit bağımlılığı (sertifikalar için) — ayrı bir crypt kütüphanesi
// eklemek yerine mevcut aracı, parola argv'ye hiç değmeden stdin üzerinden kullanır
// (rootParolaDogrula / internal/redis REDISCLI_AUTH ile aynı "sır asla argv'de değil" ilkesi).
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", fmt.Errorf("boş parola")
	}
	cmd := exec.Command("openssl", "passwd", "-6", "-stdin")
	cmd.Stdin = strings.NewReader(plain)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("openssl passwd: %w", err)
	}
	hash := strings.TrimSpace(string(out))
	if !strings.HasPrefix(hash, "$6$") {
		return "", fmt.Errorf("beklenmeyen hash biçimi")
	}
	return hash, nil
}
