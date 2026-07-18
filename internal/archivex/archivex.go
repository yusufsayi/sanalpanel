// Package archivex: jail'li + tenant-user + symlink-korumalı ortak arşiv çıkarma.
//
// Güvenlik modeli (çift savunma / defense-in-depth):
//
//	Katman 1 (DAC): çıkarma işlemi ROOT değil, tenant kullanıcısı (c_<sk>) olarak
//	  `runuser -u <sk>` ile çalışır. Bir symlink/hardlink üyesi jail'i aşsa bile,
//	  yetkisiz kullanıcı başka tenant'ın home'una veya /root'a YAZAMAZ.
//	Katman 2 (üye doğrulama): çıkarmadan ÖNCE arşiv Go stdlib (archive/zip,
//	  archive/tar) ile taranır; mutlak yollu, ".." bileşenli, jail dışına çıkan veya
//	  symlink/hardlink/aygıt üyesi tespit edilirse çıkarma tamamen REDDEDİLİR.
//
// Bu iki katman birbirinden bağımsızdır: biri baypas edilse bile diğeri korur.
// Bu paket, hem dosya yöneticisi Extract hem de yedek Restore tarafından ORTAK
// kullanılır (tek güvenli-extract yolu).
package archivex

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Güvenlik hataları.
var (
	// ErrDesteklenmeyen: bu ortak helper üye-tabanlı arşivleri (zip/tar ailesi) çıkarır;
	// tek dosyalık .gz çağıran tarafından ayrı ele alınır.
	ErrDesteklenmeyen = errors.New("desteklenmeyen arşiv formatı (zip, tar, tar.gz/tgz, tar.bz2, tar.xz)")
	// ErrUyeJailDisi: arşiv üyesi mutlak yol / ".." ile jail dışına çıkmaya çalışıyor.
	ErrUyeJailDisi = errors.New("güvenlik: arşiv üyesi ev dizini (jail) dışına çıkıyor — reddedildi")
	// ErrUyeSymlink: arşivde symlink/hardlink/aygıt üyesi var (jail-escape vektörü) — reddedildi.
	ErrUyeSymlink = errors.New("güvenlik: arşiv içinde symlink/hardlink/aygıt üyesi reddedildi")
)

// Tur: desteklenen arşiv türleri.
type Tur int

const (
	TurBilinmeyen Tur = iota
	TurZip
	TurTar
	TurTarGz
	TurTarBz2
	TurTarXz
)

// TuruBelirle: dosya adının uzantısından arşiv türünü döndürür (küçük harfe duyarsız).
func TuruBelirle(ad string) Tur {
	low := strings.ToLower(ad)
	switch {
	case strings.HasSuffix(low, ".zip"):
		return TurZip
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		return TurTarGz
	case strings.HasSuffix(low, ".tar.bz2"), strings.HasSuffix(low, ".tbz2"):
		return TurTarBz2
	case strings.HasSuffix(low, ".tar.xz"), strings.HasSuffix(low, ".txz"):
		return TurTarXz
	case strings.HasSuffix(low, ".tar"):
		return TurTar
	}
	return TurBilinmeyen
}

// uyeAdiTehlikeli: bir arşiv üye adı, çıkarma aracının (tar/unzip) HEDEF dizini aşmasına
// yol açar mı? Aracın ham adı nasıl yorumladığını modeller: mutlak yol veya ".." bileşeni
// içeriyorsa tehlikelidir. (Ham adı sanitize etmeyiz — tespit edip reddederiz.)
func uyeAdiTehlikeli(ad string) bool {
	// zip içinde Windows tarzı ters-eğik-çizgi ayraç gelebilir; onu da böl.
	ad = strings.ReplaceAll(ad, "\\", "/")
	if ad == "" {
		return false // boş ad zararsız; araç zaten atlar
	}
	if strings.HasPrefix(ad, "/") {
		return true // mutlak yol
	}
	for _, part := range strings.Split(ad, "/") {
		if part == ".." {
			return true // yol yukarı-çıkış bileşeni
		}
	}
	return false
}

// Tara: arşivin TÜM üyelerini Go stdlib ile önceden tarar; tehlikeli bir üye
// (jail-dışı ad, symlink, hardlink, aygıt) bulursa hata döner. Hiçbir şey yazmaz.
func Tara(archivePath string, tur Tur) error {
	switch tur {
	case TurZip:
		return zipTara(archivePath)
	case TurTar, TurTarGz, TurTarBz2, TurTarXz:
		return tarTara(archivePath, tur)
	default:
		return ErrDesteklenmeyen
	}
}

func zipTara(archivePath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip okuma: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		// Symlink üyesi (zip'te mod bitlerinden anlaşılır) → reddet.
		if f.Mode()&os.ModeSymlink != 0 {
			return ErrUyeSymlink
		}
		if uyeAdiTehlikeli(f.Name) {
			return ErrUyeJailDisi
		}
	}
	return nil
}

func tarTara(archivePath string, tur Tur) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("arşiv okuma: %w", err)
	}
	defer f.Close()

	var r io.Reader = f
	switch tur {
	case TurTarGz:
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return fmt.Errorf("gzip: %w", gerr)
		}
		defer gz.Close()
		r = gz
	case TurTarBz2:
		r = bzip2.NewReader(f)
	case TurTarXz:
		// Go stdlib xz çözmez → sadece TARAMA için `xz -dc` ile aç (root okur).
		xzc := exec.Command("xz", "-dc")
		xzc.Stdin = f
		pipe, perr := xzc.StdoutPipe()
		if perr != nil {
			return fmt.Errorf("xz pipe: %w", perr)
		}
		if serr := xzc.Start(); serr != nil {
			return fmt.Errorf("xz başlat: %w", serr)
		}
		defer func() { _ = xzc.Wait() }()
		defer pipe.Close()
		r = pipe
	}

	tr := tar.NewReader(r)
	for {
		hdr, nerr := tr.Next()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			return fmt.Errorf("tar okuma: %w", nerr)
		}
		// Tehlikeli üye tipleri: symlink, hardlink, char/block aygıt, fifo → reddet.
		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return ErrUyeSymlink
		}
		if uyeAdiTehlikeli(hdr.Name) {
			return ErrUyeJailDisi
		}
	}
	return nil
}

// runuserKomut: argv'yi tenant kullanıcısı (sk) olarak, panel sırları OLMADAN,
// temiz env ile çalıştıracak komutu hazırlar (panelin composer/git/redis deseni).
func runuserKomut(sk string, argv ...string) *exec.Cmd {
	full := append([]string{"-u", sk, "--"}, argv...)
	cmd := exec.Command("runuser", full...)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/home/" + sk,
	}
	return cmd
}

// GuvenliCikar: arşivi destDir içine, tenant kullanıcısı sk olarak, üye-yollarını
// doğrulayarak güvenli biçimde çıkarır (çift savunma).
//
// Önkoşul: destDir sk tarafından yazılabilir olmalı (çağıran chown etmelidir).
// Dönüş: aracın birleşik çıktısı (hata mesajı için) ve hata.
//
// tar ailesi için arşiv baytları stdin üzerinden akıtılır; böylece root-sahipli
// arşivler (örn. yedek deposu) bile tenant kullanıcısına okutulmadan çıkarılabilir.
func GuvenliCikar(archivePath, destDir, sk string) (string, error) {
	tur := TuruBelirle(archivePath)
	if tur == TurBilinmeyen {
		return "", ErrDesteklenmeyen
	}
	if !strings.HasPrefix(sk, "c_") {
		return "", errors.New("güvenlik: geçersiz tenant kullanıcısı")
	}

	// Katman 2: üye ön-taraması (jail-dışı / symlink / hardlink reddi).
	if err := Tara(archivePath, tur); err != nil {
		return "", err
	}

	// Katman 1: tenant-user (DAC) altında çıkar.
	var cmd *exec.Cmd
	switch tur {
	case TurZip:
		// unzip stdin okuyamaz; arşiv sk-okunur olmalı (tenant home'undaki dosya).
		cmd = runuserKomut(sk, "unzip", "-o", "-q", archivePath, "-d", destDir)
	default:
		// tar ailesi: root arşivi açar, baytlar tenant tar'a stdin'den akar.
		f, err := os.Open(archivePath)
		if err != nil {
			return "", fmt.Errorf("arşiv aç: %w", err)
		}
		defer f.Close()
		flag := "-x"
		switch tur {
		case TurTarGz:
			flag = "-xz"
		case TurTarBz2:
			flag = "-xj"
		case TurTarXz:
			flag = "-xJ"
		}
		cmd = runuserKomut(sk, "tar", flag, "-f", "-", "-C", destDir)
		cmd.Stdin = f
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("çıkarma (tenant=%s): %w", sk, err)
	}
	return string(out), nil
}
