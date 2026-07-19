package files

// safeio_test.go — TOCTOU symlink-güvenli mutasyonların testi.
// Kanıt: ara-dizin (veya leaf) symlink ile jail-DIŞINA point ederken chmod/write/delete
// jail-DIŞINA ÇIKMIYOR (hata dönüyor / dış dosyaya dokunmuyor); meşru işlemler ÇALIŞIYOR.

import (
	"os"
	"path/filepath"
	"testing"
)

// setupJail: home (jail) + outside (jail-dışı /etc benzeri) + içlerinde dosyalar kurar.
func setupJail(t *testing.T) (home, outside string) {
	t.Helper()
	home = t.TempDir()
	outside = t.TempDir()
	// jail içi meşru içerik
	if err := os.MkdirAll(filepath.Join(home, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "real", "f"), []byte("legit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// jail dışı hassas hedef
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	// ara-dizin symlink saldırısı: home/link -> outside
	if err := os.Symlink(outside, filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}
	// leaf symlink saldırısı: home/lnk -> outside/secret
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(home, "lnk")); err != nil {
		t.Fatal(err)
	}
	return home, outside
}

func TestChmodBeneath_AraDizinSymlinkReddedilir(t *testing.T) {
	home, outside := setupJail(t)
	secret := filepath.Join(outside, "secret")
	before, _ := os.Stat(secret)

	// home/link -> outside; "link/secret" ara-bileşen symlink ile jail-dışı hedefe iner.
	if err := chmodBeneath(home, "link/secret", 0o777); err == nil {
		t.Fatal("BEKLENEN HATA: ara-dizin symlink üzerinden chmod jail-dışına çıktı")
	}
	after, _ := os.Stat(secret)
	if before.Mode() != after.Mode() {
		t.Fatalf("jail-dışı dosya modu DEĞİŞTİ: %v -> %v", before.Mode(), after.Mode())
	}
}

func TestChmodBeneath_LeafSymlinkReddedilir(t *testing.T) {
	home, outside := setupJail(t)
	secret := filepath.Join(outside, "secret")
	before, _ := os.Stat(secret)
	if err := chmodBeneath(home, "lnk", 0o777); err == nil {
		t.Fatal("BEKLENEN HATA: leaf symlink üzerinden chmod jail-dışına çıktı")
	}
	after, _ := os.Stat(secret)
	if before.Mode() != after.Mode() {
		t.Fatalf("jail-dışı dosya modu DEĞİŞTİ: %v -> %v", before.Mode(), after.Mode())
	}
}

func TestWriteBeneath_AraDizinSymlinkReddedilir(t *testing.T) {
	home, outside := setupJail(t)
	secret := filepath.Join(outside, "secret")
	if err := writeBeneath(home, "link/secret", []byte("PWNED"), 0o644, ""); err == nil {
		t.Fatal("BEKLENEN HATA: symlink üzerinden write jail-dışına çıktı")
	}
	b, _ := os.ReadFile(secret)
	if string(b) != "SECRET" {
		t.Fatalf("jail-dışı dosya içeriği DEĞİŞTİ: %q", string(b))
	}
}

func TestRemoveAllBeneath_SymlinkTakipEtmez(t *testing.T) {
	home, outside := setupJail(t)
	// outside'da korunması gereken bir dosya daha
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("KEEP"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "link/keep": ara-dizin symlink → parent açılışı reddedilmeli (hata) veya dokunmamalı
	if err := removeAllBeneath(home, "link/keep"); err == nil {
		t.Fatal("BEKLENEN HATA: symlink üzerinden delete jail-dışına indi")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatalf("jail-dışı dosya SİLİNDİ: %v", err)
	}
	// "link" (symlink'in kendisi) silinince SADECE link kalkmalı, hedef içerik durmalı.
	if err := removeAllBeneath(home, "link"); err != nil {
		t.Fatalf("symlink'in kendisini silmek başarısız: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "link")); !os.IsNotExist(err) {
		t.Fatal("home/link symlink'i silinmedi")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Fatalf("symlink hedefindeki jail-dışı içerik SİLİNDİ: %v", err)
	}
}

func TestMesruIslemlerCalisir(t *testing.T) {
	home, _ := setupJail(t)

	// chmod meşru dosya
	if err := chmodBeneath(home, "real/f", 0o600); err != nil {
		t.Fatalf("meşru chmod başarısız: %v", err)
	}
	if fi, _ := os.Stat(filepath.Join(home, "real", "f")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("meşru chmod uygulanmadı: %v", fi.Mode().Perm())
	}

	// write yeni dosya (ara dizinler mkdir-p ile)
	if err := mkdirAllBeneath(home, "a/b", ""); err != nil {
		t.Fatalf("mkdirAll başarısız: %v", err)
	}
	if err := writeBeneath(home, "a/b/yeni.txt", []byte("merhaba"), 0o644, ""); err != nil {
		t.Fatalf("meşru write başarısız: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "a", "b", "yeni.txt")); string(b) != "merhaba" {
		t.Fatalf("meşru write içeriği yanlış: %q", string(b))
	}

	// rename meşru
	if err := renameBeneath(home, "real/f", "real/f2", ""); err != nil {
		t.Fatalf("meşru rename başarısız: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "real", "f2")); err != nil {
		t.Fatalf("rename hedefi yok: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "real", "f")); !os.IsNotExist(err) {
		t.Fatal("rename kaynağı hâlâ var")
	}

	// yeni boş dosya (O_EXCL)
	if err := createExclBeneath(home, "a/b/bos.txt", ""); err != nil {
		t.Fatalf("createExcl başarısız: %v", err)
	}
	if err := createExclBeneath(home, "a/b/bos.txt", ""); err == nil {
		t.Fatal("createExcl var olan dosyada hata vermeliydi (EEXIST)")
	}

	// recursive copy meşru
	if err := os.MkdirAll(filepath.Join(home, "src", "in"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "src", "in", "x"), []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeBeneath(home, "src", "dst", ""); err != nil {
		t.Fatalf("meşru copy başarısız: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "dst", "in", "x")); string(b) != "X" {
		t.Fatalf("copy içeriği yanlış: %q", string(b))
	}

	// recursive delete meşru
	if err := removeAllBeneath(home, "src"); err != nil {
		t.Fatalf("meşru delete başarısız: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "src")); !os.IsNotExist(err) {
		t.Fatal("delete edilen dizin hâlâ var")
	}
}

func TestCopyTreeBeneath_JailDisiSymlinkIceriginiSizdirmaz(t *testing.T) {
	home, outside := setupJail(t)
	// src içinde jail-dışına point eden bir symlink olsun
	if err := os.MkdirAll(filepath.Join(home, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(home, "src", "leak")); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeBeneath(home, "src", "dst", ""); err != nil {
		t.Fatalf("copy başarısız: %v", err)
	}
	// Kopya, symlink'i OLDUĞU gibi yeniden kurmalı (içeriği düz dosya olarak SIZDIRMAMALI).
	fi, err := os.Lstat(filepath.Join(home, "dst", "leak"))
	if err != nil {
		t.Fatalf("kopyalanan symlink yok: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("jail-dışı symlink düz dosyaya çözülmüş (içerik sızması riski)")
	}
}
