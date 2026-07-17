// trafik.go — per-domain AYLIK trafik sayacı.
// nginx access.log'undan ($body_bytes_sent) artımlı okuma yapar (ofset+boyut imleci ile
// log-rotasyonuna dayanıklı), her satırın tarihine göre aylık kovaya toplar ve
// içinde bulunulan ayın toplamını domains.trafik_kb alanına yazar (dashboard buradan okur).
// Kök neden (Bug 2): trafik_kb HİÇBİR YERDE yazılmıyordu → hep 0 görünüyordu.
package istatistik

import (
	"bufio"
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// StartTrafikAggregator: panel başlangıcında çağrılır, kendi goroutine'inde periyodik toplar.
func StartTrafikAggregator(db *sql.DB, every time.Duration) {
	go func() {
		// açılıştan 30 sn sonra ilk toplama (migration + servisler otursun)
		time.Sleep(30 * time.Second)
		AggregateAll(db)
		t := time.NewTicker(every)
		defer t.Stop()
		for range t.C {
			AggregateAll(db)
		}
	}()
}

// AggregateAll: tüm domainler için trafiği toplar, işlenen domain sayısını döner.
func AggregateAll(db *sql.DB) int {
	rows, err := db.Query(`SELECT id, alan_adi FROM domains`)
	if err != nil {
		log.Printf("trafik: domain listesi: %v", err)
		return 0
	}
	type dom struct {
		id      int64
		alanAdi string
	}
	var liste []dom
	for rows.Next() {
		var d dom
		if err := rows.Scan(&d.id, &d.alanAdi); err == nil {
			liste = append(liste, d)
		}
	}
	rows.Close()

	n := 0
	for _, d := range liste {
		if aggregateDomain(db, d.id, d.alanAdi) {
			n++
		}
	}
	return n
}

// aggregateDomain: tek domain'in access.log'unu artımlı okuyup aylık trafiği günceller.
func aggregateDomain(db *sql.DB, domainID int64, alanAdi string) bool {
	logPath := "/var/log/nginx/" + alanAdi + ".access.log"
	fi, err := os.Stat(logPath)
	if err != nil {
		// log yok → yine de içinde bulunulan ay kovasından trafik_kb tazele
		refreshTrafikKB(db, domainID)
		return false
	}
	size := fi.Size()

	var ofset, boyut int64
	_ = db.QueryRow(`SELECT ofset, boyut FROM domain_trafik_imlec WHERE domain_id=?`, domainID).Scan(&ofset, &boyut)

	start := ofset
	// rotasyon/truncate tespiti: dosya küçüldüyse baştan oku
	if size < ofset {
		start = 0
	}
	if start == size {
		refreshTrafikKB(db, domainID)
		return true
	}

	f, err := os.Open(logPath)
	if err != nil {
		return false
	}
	defer f.Close()
	if start > 0 {
		if _, err := f.Seek(start, 0); err != nil {
			start = 0
			_, _ = f.Seek(0, 0)
		}
	}

	reader := bufio.NewReaderSize(f, 256*1024)
	aylik := map[string]int64{}
	consumed := start
	for {
		line, rerr := reader.ReadString('\n')
		if len(line) > 0 && strings.HasSuffix(line, "\n") {
			consumed += int64(len(line))
			if ay, byt, ok := parseTrafikSatir(line); ok {
				aylik[ay] += byt
			}
		}
		if rerr != nil {
			break // EOF veya kısmi son satır → ofset ilerletme
		}
	}

	// aylık kovaları upsert et
	for ay, byt := range aylik {
		if byt <= 0 {
			continue
		}
		if _, err := db.Exec(
			`INSERT INTO domain_trafik(domain_id, yil_ay, bytes) VALUES(?,?,?)
			 ON DUPLICATE KEY UPDATE bytes=bytes+VALUES(bytes)`,
			domainID, ay, byt); err != nil {
			log.Printf("trafik upsert (domain=%d ay=%s): %v", domainID, ay, err)
		}
	}

	// imleci güncelle
	_, _ = db.Exec(
		`INSERT INTO domain_trafik_imlec(domain_id, ofset, boyut) VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE ofset=VALUES(ofset), boyut=VALUES(boyut)`,
		domainID, consumed, size)

	refreshTrafikKB(db, domainID)
	return true
}

// refreshTrafikKB: içinde bulunulan ayın toplamını domains.trafik_kb'ye yazar (KB).
func refreshTrafikKB(db *sql.DB, domainID int64) {
	ay := time.Now().UTC().Format("2006-01")
	var byt int64
	_ = db.QueryRow(`SELECT bytes FROM domain_trafik WHERE domain_id=? AND yil_ay=?`, domainID, ay).Scan(&byt)
	_, _ = db.Exec(`UPDATE domains SET trafik_kb=? WHERE id=?`, byt/1024, domainID)
}

// parseTrafikSatir: combined log satırından (ay 'YYYY-MM', bytes) çıkarır.
// Format: IP - - [17/Jul/2026:12:00:00 +0000] "GET / HTTP/1.1" 200 1234 "..." "..."
func parseTrafikSatir(line string) (string, int64, bool) {
	m := reLog.FindStringSubmatch(line)
	if m == nil {
		return "", 0, false
	}
	// m[2] = tarih kısmı (ör. 17/Jul/2026), m[6] = bytes (veya "-")
	t, err := time.Parse("02/Jan/2006", strings.TrimSpace(m[2]))
	if err != nil {
		return "", 0, false
	}
	byt := int64(0)
	if m[6] != "-" {
		byt, _ = strconv.ParseInt(m[6], 10, 64)
	}
	return t.Format("2006-01"), byt, true
}
