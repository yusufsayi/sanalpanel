# GirginOSPanel

Boş bir **AlmaLinux 10** sunucuyu tek komutla komple bir hosting kontrol paneline çevirir — nginx + MariaDB + çok sürümlü PHP + Valkey (Redis) + phpMyAdmin + güvenlik duvarı, hepsi otomatik kurulur ve ayarlanır.

## Tek satır kurulum

Temiz bir AlmaLinux 10 (min. 2 GB RAM) sunucuda **root** olarak:

```bash
curl -fsSL https://raw.githubusercontent.com/girginos/gpanel/main/install.sh | bash
```

Kurulum ~5-10 dakika sürer (paket indirmeleri). Bittiğinde panel adresi + giriş bilgileri ekrana yazılır.

## Kurulum sonrası

- **Panel:** `https://SUNUCU_IP:8443` (self-signed sertifika — tarayıcı uyarısını geçin)
- **Giriş:** kullanıcı **`root`** · parola = **sunucunun root parolası**
  (panel yöneticisini işletim sistemi root'u üzerinden PAM ile doğrular; ayrı bir panel parolası yoktur)

## Ne kurar?

| Bileşen | Detay |
|---|---|
| **Web** | nginx (panel :8443 + müşteri siteleri :80/:443) |
| **PHP** | 7.4 / 8.2 / 8.3 / 8.4 / 8.5 (remi) — her domain bağımsız sürüm seçer, per-domain FPM havuzu |
| **Veritabanı** | MariaDB 10.11 (`panel` DB) + phpMyAdmin (`/pma/`) |
| **Cache** | Valkey (Redis) — per-tenant izole object cache (WordPress'e otomatik bağlanır) |
| **Güvenlik** | nftables güvenlik duvarı, SELinux uyumlu, ClamAV |
| **Performans** | MariaDB + nginx + OPcache otomatik tuning (`girginospanel-optimize`) |

## Panel özellikleri

- Domain / subdomain yönetimi, DNS düzenleme, toplu işlemler
- Tek-tık **WordPress** kurulumu + WP-CLI
- Per-tenant **Redis object cache** (tek tıkla aç/kapa, WP'ye otomatik bağlama)
- **Güvenlik duvarı** arayüzü (IP ban / whitelist / port kapatma + hazır şablonlar)
- Backup yöneticisi, izleme/loglar, istatistikler
- Müşteri / bayi yönetimi, hizmet planları

## Sistem gereksinimleri

- **AlmaLinux 10** (RHEL 10 / Rocky 10 de çalışır)
- En az **2 GB RAM**, 2 vCPU (5 PHP sürümü + MariaDB + Valkey için)
- Root erişimi + internet bağlantısı

## Kurulum sonrası yardımcı araçlar

Kurulumla birlikte `/usr/local/bin`'e şu araçlar gelir:

```bash
girginospanel-optimize      # MariaDB/nginx/PHP'yi sunucu kaynaklarına göre yeniden ayarla
girginospanel-redis-setup   # Valkey (Redis) altyapısını kur/onar
girginospanel-wp-redis <sk> # bir domainin WordPress'ine Redis cache bağla/çöz
girginospanel-repair        # izin / SELinux / sahiplik onarımı (idempotent)
```

## Notlar

- Kurulum **idempotent** değildir; her çalıştırma yeni secret (JWT/DB parola) üretir. Yeniden çalıştırma yerine `girginospanel-repair` / `girginospanel-optimize` kullanın.
- Panel HTTP/2 + self-signed SSL ile :8443'te yayınlanır; gerçek alan adı için Let's Encrypt panel üzerinden eklenebilir.
