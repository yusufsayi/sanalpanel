package kaynaklimit

import (
	"context"
	"os"
	"reflect"
	"testing"
)

// TestKotaLimitArgs: xfs_quota arg-slice'ının doğru kurulduğunu (soft=hard*0.95, 0=sınırsız,
// -u user quota, kök mount) DOĞRULAR. Shell yok — arg-slice bütünlüğü kritiktir.
func TestKotaLimitArgs(t *testing.T) {
	cases := []struct {
		sk           string
		diskMB       int
		inode        int
		wantLimitArg string
	}{
		// tam limit: soft = %95
		{"c_ornek", 5120, 500000, "limit -u bsoft=4864m bhard=5120m isoft=475000 ihard=500000 c_ornek"},
		// disk limit + inode sınırsız (0)
		{"c_foo", 1024, 0, "limit -u bsoft=972m bhard=1024m isoft=0 ihard=0 c_foo"},
		// her ikisi sınırsız (0 = limit yok)
		{"c_bar", 0, 0, "limit -u bsoft=0m bhard=0m isoft=0 ihard=0 c_bar"},
		// yalnız inode limiti
		{"c_baz", 0, 100000, "limit -u bsoft=0m bhard=0m isoft=95000 ihard=100000 c_baz"},
		// negatif → 0'a sıkıştır
		{"c_neg", -5, -9, "limit -u bsoft=0m bhard=0m isoft=0 ihard=0 c_neg"},
	}
	for _, c := range cases {
		got := kotaLimitArgs(c.sk, c.diskMB, c.inode)
		want := []string{"-x", "-c", c.wantLimitArg, "/"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("kotaLimitArgs(%q,%d,%d)\n  got  = %#v\n  want = %#v", c.sk, c.diskMB, c.inode, got, want)
		}
	}
}

// TestReKotaSK: sistem kullanıcı allowlist'i geçerli c_<slug> kabul eder, injection/geçersiz reddeder.
func TestReKotaSK(t *testing.T) {
	valid := []string{"c_foo", "c_reg_kalici_test_local", "c_a1b2c3", "c_x"}
	for _, v := range valid {
		if !reKotaSK.MatchString(v) {
			t.Errorf("reKotaSK geçerli sk'yı reddetti: %q", v)
		}
	}
	invalid := []string{
		"", "root", "foo", "c_", "c_Foo", "c_foo bar", "c_foo;rm -rf /",
		"c_foo`id`", "c_foo\nx", "../c_foo", "c_foo/../bar", "admin",
	}
	for _, v := range invalid {
		if reKotaSK.MatchString(v) {
			t.Errorf("reKotaSK geçersiz/tehlikeli sk'yı kabul etti: %q", v)
		}
	}
}

// TestEfektifKota: override > plan > varsayılan çözümleme mantığı.
func TestEfektifKota(t *testing.T) {
	cases := []struct {
		ad                  string
		dOver, iOver        int
		planVar             bool
		pDisk, pInode       int
		wantDisk, wantInode int
	}{
		{"plan yok → varsayılan", 0, 0, false, 0, 0, varsayilanDiskMB, varsayilanInode},
		{"plan var, açıkça sınırsız", 0, 0, true, 0, 0, 0, 0},
		{"plan değerleri", 0, 0, true, 1024, 50000, 1024, 50000},
		{"disk override plan'ı ezer", 2048, 0, true, 1024, 50000, 2048, 50000},
		{"inode override planless'ı ezer", 0, 99999, false, 0, 0, varsayilanDiskMB, 99999},
		{"her iki override", 3072, 123456, true, 1024, 50000, 3072, 123456},
	}
	for _, c := range cases {
		gd, gi := efektifKota(c.dOver, c.iOver, c.planVar, c.pDisk, c.pInode)
		if gd != c.wantDisk || gi != c.wantInode {
			t.Errorf("%s: efektifKota=(%d,%d) want=(%d,%d)", c.ad, gd, gi, c.wantDisk, c.wantInode)
		}
	}
}

// TestKotaUygulaNoquotaGracefulLive: 181 gibi noquota fs'te KotaUygula HATA DÖNMEMELİ
// (log + return nil = graceful skip). Yalnız KOTA_LIVE=1 ile çalışır (gerçek xfs_quota çağırır).
func TestKotaUygulaNoquotaGracefulLive(t *testing.T) {
	if os.Getenv("KOTA_LIVE") != "1" {
		t.Skip("KOTA_LIVE=1 ile gerçek (noquota) fs üstünde çalıştır")
	}
	acc, enf := mountKotaAktif()
	t.Logf("mountKotaAktif(): accounting=%v enforcement=%v", acc, enf)
	if err := KotaUygula(context.Background(), "c_kotatest_noquota", 1024, 50000); err != nil {
		t.Fatalf("noquota fs'te KotaUygula HATA döndürdü (graceful-skip beklendi): %v", err)
	}
	t.Log("KotaUygula noquota'da graceful skip: err=nil ✓")
}
