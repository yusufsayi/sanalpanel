package auth

import (
	"context"
	"database/sql"
)

// GÜVENLİK: login uç noktaları (admin + müşteri) hiçbir deneme sınırı olmadan
// hedef alınabiliyordu — admin girişi doğrudan sunucunun ROOT parolasına karşı
// çalıştığı için bu, sınırsız brute-force imkanı demekti. audit_log zaten her
// login denemesini (başarılı/başarısız) IP ile kaydediyor; ayrı bir sayaç/tablo
// eklemeden aynı tabloyu pencereli bir kilit için kullanıyoruz.
const (
	LockoutMaxAttempts = 5
	LockoutWindowMin   = 15
)

// TooManyFailedAttempts: verilen action için son LockoutWindowMin dakikada bu
// IP'den LockoutMaxAttempts veya daha fazla başarısız (ok=0) deneme var mı?
// DB hatasında fail-open döner (locked=false) — bu kontrol asıl kimlik
// doğrulamanın YERİNE değil, ÖNÜNE eklenen ek bir savunma katmanıdır.
func TooManyFailedAttempts(ctx context.Context, db *sql.DB, action, ip string) bool {
	if ip == "" || db == nil {
		return false
	}
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log
		 WHERE action=? AND ip=? AND ok=0 AND ts > (NOW() - INTERVAL ? MINUTE)`,
		action, ip, LockoutWindowMin).Scan(&n)
	if err != nil {
		return false
	}
	return n >= LockoutMaxAttempts
}
