package phpsurum

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestPaketMevcutCacheOnly: istek path'inin (paketMevcut / TumSurumler) ASLA dnf çağırmadığını,
// yalnızca arka-plan sweep'in doldurduğu cache'i okuduğunu ve eşzamanlı erişimin race içermediğini
// doğrular. `go test -race` altında çalıştırılmalı.
func TestPaketMevcutCacheOnly(t *testing.T) {
	// Gerçek arka-plan goroutine'ini başlatMA: sweeperOnce'ı boş fonk. ile tüket → deterministik.
	sweeperOnce.Do(func() {})

	var probeCalls int64
	old := dnfProbe
	dnfProbe = func(pkg string) bool { // dnf'e ASLA gitmeyen sahte prob
		atomic.AddInt64(&probeCalls, 1)
		return pkg == "php82-php-fpm" // sadece php82 kurulabilir
	}
	defer func() { dnfProbe = old }()

	// Cache'i elle (senkron) doldur — normalde bunu arka-plan sweeper yapar.
	sweepOnce()

	// Doğruluk: değerler cache'ten okunur.
	if !paketMevcut(SurumMeta{Surum: "8.2", Kod: "82", Kaynak: "remi"}) {
		t.Fatal("php82 kurulabilir bekleniyordu")
	}
	if paketMevcut(SurumMeta{Surum: "8.1", Kod: "81", Kaynak: "remi"}) {
		t.Fatal("php81 kurulamaz bekleniyordu (cache=false)")
	}
	if !paketMevcut(SurumMeta{Surum: "8.3", Kod: "", Kaynak: "appstream"}) {
		t.Fatal("appstream daima mevcut olmalı")
	}

	base := atomic.LoadInt64(&probeCalls)

	// Eşzamanlı istek yükü: cache-only okuma, dnf çağrısı OLMAMALI + race OLMAMALI.
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = paketMevcut(SurumMeta{Surum: "8.2", Kod: "82", Kaynak: "remi"})
			_ = paketMevcut(SurumMeta{Surum: "8.4", Kod: "84", Kaynak: "remi"})
			_ = TumSurumler()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&probeCalls); got != base {
		t.Fatalf("istek path'i dnf çağırdı: base=%d got=%d (cache-only olmalı)", base, got)
	}
}

// TestBosCacheBloklamaz: cache boşken (ilk boot) istek path'i dnf'e gitmeden hemen makul
// varsayılan (false) döner — asılmaz.
func TestBosCacheBloklamaz(t *testing.T) {
	sweeperOnce.Do(func() {})

	old := dnfProbe
	dnfProbe = func(pkg string) bool { t.Fatalf("istek path'inde dnf çağrıldı: %s", pkg); return false }
	defer func() { dnfProbe = old }()

	// Cache'i boşalt (ilk-boot simülasyonu).
	availMu.Lock()
	availCache = map[string]bool{}
	availMu.Unlock()

	// Boş cache → false, dnf çağrısı YOK (dnfProbe çağrılırsa test patlar).
	if paketMevcut(SurumMeta{Surum: "8.5", Kod: "85", Kaynak: "remi"}) {
		t.Fatal("boş cache'te varsayılan false bekleniyordu")
	}
}
