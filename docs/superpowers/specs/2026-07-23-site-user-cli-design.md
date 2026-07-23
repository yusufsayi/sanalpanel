# Site Kullanıcısı CLI Komutları — Tasarım

**Tarih:** 2026-07-23
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

CloudPanel'in [site-user-commands](https://www.cloudpanel.io/docs/v2/cloudpanel-cli/site-user-commands/) sayfasındaki gibi, SanalPanel'de barındırılan sitelerin **site kullanıcılarının** (jail'li SSH hesapları) kendi başlarına çalıştırabileceği kısa komutlar eklemek. Şu an jail'de standart unix araçları var ama panel'e özel hiçbir CLI yok; DB export/import, izin sıfırlama, cache temizleme gibi işler için ya admin paneline girmeleri ya da bana yazmaları gerekiyor.

**Kapsam dışı (bilinçli):** CloudPanel'in [root-user-commands](https://www.cloudpanel.io/docs/v2/cloudpanel-cli/root-user-commands/) sayfasındaki admin komutları (`user:add`, `site:add:*`, `lets-encrypt:*`, `vhost-template:*` vb.) bu spec'in konusu değil — ayrı, sonraki bir iş.

## v1 Komut Listesi

CloudPanel site-user sayfasındaki 4 komutun SanalPanel'e uyarlanmış hali:

1. `sanalpanel db:export --databaseName=<ad> --file=<yol>`
2. `sanalpanel db:import --databaseName=<ad> --file=<yol>`
3. `sanalpanel permissions:reset --directories=770 --files=660 --path=.`
4. `sanalpanel cache:purge --purge=all|fastcgi|redis`

Başka komut yok (cron listeleme, php versiyonu vb. bilinçli olarak dışarıda bırakıldı — YAGNI).

## Mimari Genel Bakış

Komutlar iki kategoriye ayrılıyor:

- **Salt yerel** (`permissions:reset`): jail'de zaten mevcut `chmod`/`find` ile çalışır, sunucuya hiç gitmez. Kullanıcı zaten bind-mount'lanmış kendi home dizininin sahibi olduğu için root'a ihtiyaç yok.
- **Sunucu-aracılı** (`db:export`, `db:import`, `cache:purge`): jail'de `mysql`/`mysqldump` istemcisi yok, DB şifresi kullanıcıda değil; FastCGI cache tüm siteler arası **paylaşımlı** bir dizin (`/var/cache/nginx/sanalcache`, bkz. `internal/provisioner/provisioner.go`), kullanıcı doğrudan dokunamaz ve dokunmamalı (izolasyon). Bu üç komut, panel sunucusunun yeni bir `/api/cli/*` uç noktasına, **sadece loopback'te dinleyen ayrı bir listener** (mevcut `:8080` genel panel portundan bağımsız, dışarıya asla açılmayan) üzerinden, kullanıcıya özel bir bearer token ile HTTP isteği atarak çalışır.

```
[jail: site kullanıcısı]
   └─ sanalpanel <komut>   (bash dispatcher, /usr/local/bin/sanalpanel)
        ├─ permissions:reset → doğrudan chmod/find (yerel, API'ye gitmez)
        └─ db:export / db:import / cache:purge
             └─ curl -H "Authorization: Bearer <~/.sanalpanel/token>" 127.0.0.1:<cli-port>/api/cli/...
                  └─ [panel sunucusu, root]
                       ├─ token → sk (sistem_kullanici) eşlemesi doğrulanır
                       ├─ db:export/import → mysqldump/mysql (mevcut db_accounts doğrulamasıyla)
                       └─ cache:purge → redis ACL DEL (~sk:*) ve/veya nginx cache-version bump + reload
```

## Bileşenler

### 1. CLI dispatcher script (`/usr/local/bin/sanalpanel`)

Bash script, `scripts/sanalpanel-jail` şablon inşasına (`build_template`, `BINS` listesi) eklenecek — `curl` zaten jail'de mevcut, ek binary gerekmiyor. Script:

- Argümanları parse eder (`db:export`, `db:import`, `permissions:reset`, `cache:purge`).
- `permissions:reset` için doğrudan `find "$path" -type d -exec chmod "$directories" {} +` / dosyalar için `chmod "$files"`.
- Diğerleri için `~/.sanalpanel/token` dosyasını okuyup ilgili `/api/cli/*` uç noktasına `curl` çağrısı yapar; `db:export` yanıtını `-o "$file"` ile doğrudan diske yazar, `db:import` dosyayı `--data-binary @"$file"` ile gönderir.
- Türkçe, kullanıcı dostu çıktı ve anlamlı exit code'lar (0 başarı, 1 genel hata, kimlik doğrulama/yetki hataları için de 1 ama farklı mesaj — CloudPanel gibi ayrı exit code şeması gerekmiyor, YAGNI).

### 2. Token provisioning

Domain oluşturulurken (`internal/domains/handlers.go`, `dbUser`/`dbName` türetildiği aynı akışta — `pr.SistemKullanici + "_db"` / `"_main"` deseninin hemen yanı) rastgele bir CLI token üretilir:

- Ham token bir kere `/home/<sk>/.sanalpanel/token` dosyasına yazılır (`chmod 600`, sahibi `sk:sk`) — jail gerçek home'u bind-mount ettiği için kullanıcı bunu görür.
- Sunucu tarafında sadece **hash'i** saklanır, yeni bir `cli_tokens` tablosunda (`domain_id`, `token_hash`, `created_at`) — `db_accounts`/`cp_domain_redis` gibi mevcut per-domain yardımcı tablo desenine uygun, ayrı tutulması iptal/rotasyonu domain satırına dokunmadan yapılabilir kılar. Ham token hiçbir yerde tekrar okunmaz/loglanmaz.
- Domain silinirken/sistem kullanıcısı kaldırılırken token da geçersiz kılınır (mevcut `MySQLDropAllForDomain` gibi temizlik akışlarının yanına eklenir).

### 3. Sunucu tarafı `/api/cli/*` uç noktaları

Yeni bir Go paketi (örn. `internal/clicommands/`), ayrı bir `net.Listen("tcp", "127.0.0.1:<cli-port>")` ile **sadece loopback'te** dinleyen ikinci bir HTTP sunucusu üzerinde mount edilir (mevcut `cfg.ListenAddr=":8080"` genel dinleyiciden tamamen ayrı — dışarıdan erişilemez olması güvenlik önlemi olarak listener seviyesinde garanti edilir, sadece firewall kuralına güvenilmez).

Auth middleware: `Authorization: Bearer <token>` header'ını hash'leyip DB'deki kayıtla karşılaştırır, eşleşen `sk`/`domain_id`'yi context'e koyar. Eşleşme yoksa `401`.

- **`POST /api/cli/db/export`** — body: `databaseName`, `file` (sadece dosya adı/uzantı bilgisi için, path olarak kullanılmaz). `databaseName`'in çağıran `sk`'ye ait olduğu `db_accounts` tablosundan doğrulanır (yabancı isim → `403`). Ardından `mysqldump` çalıştırılır, `.sql.gz` uzantısı istenmişse gzip'lenir, response body olarak ham baytlar döner. Sunucu hiçbir dosya yoluna yazmaz — path traversal yüzeyi yok.
- **`POST /api/cli/db/import`** — body: `databaseName` + request body'de ham dosya baytları. Aynı ownership doğrulaması, sonra `mysql <db>` komutuna pipe.
- **`POST /api/cli/cache/purge`** — body: `purge=all|fastcgi|redis`.
  - `redis` veya `all`: mevcut `internal/redis/redis.go`'daki `cli()` helper'ı (admin şifresiyle) kullanılarak `~sk:*` prefix'li key'ler taranıp silinir (kullanıcının kendi ACL şifresine gerek yok, sunucu zaten admin yetkili).
  - `fastcgi` veya `all`: domain başına bir **cache-version sayacı** (nginx `map` dosyasında tek satır, `cacheZoneName`/`cacheZoneDir` sabitlerinin yanına eklenecek yeni bir mekanizma) bir artırılır; fastcgi_cache_key bu sayacı içerecek şekilde vhost template'i güncellenir. Purge = diskte dosya silmek değil, sayacı artırıp o domain'in eski cache girdilerini "görünmez" kılmak (kendiliğinden `inactive=60m` ile temizlenir). Reload öncesi zorunlu `nginx -t` (mevcut `apacheTestReload` deseniyle tutarlı) — test başarısızsa sayaç geri alınır, `500` döner.

## Girdi Doğrulama / Güvenlik

- `databaseName` mevcut `internal/hesaplar/hesaplar.go`'daki `GecerliDBKimlik`/`GecerliDBSonek` regex doğrulayıcılarıyla süzülür (shell/SQL enjeksiyonuna karşı, mevcut desenle tutarlı).
- Token: kriptografik rastgele (≥32 byte), sunucuda sadece hash saklanır, dosyada `chmod 600`.
- `/api/cli/*` sadece `127.0.0.1`'de dinler — dışarıdan (nginx/Cloudflare üzerinden) hiçbir şekilde erişilemez.
- Bir site kullanıcısı yalnızca kendi `sk`'sine ait DB/cache üzerinde işlem yapabilir; token→domain eşlemesi dışında hiçbir cross-tenant erişim yolu yok.

## Hata Yönetimi

- Token yok/geçersiz → `401`, CLI script Türkçe hata basar, exit 1.
- Yabancı `databaseName` → `403`, "Bu veritabanı size ait değil".
- `mysqldump`/`mysql` başarısızlığı → stderr kullanıcıya aktarılır, exit 1.
- `nginx -t` başarısızsa cache-version artışı geri alınır, reload denenmez, `500`.

## Test Planı

- Go unit testler: yeni auth middleware (token→domain eşlemesi, geçersiz/yabancı token senaryoları), `databaseName` ownership doğrulaması, cache-version artırma mantığı.
- Gerçek VPS'te bir jail içinden uçtan uca canlı doğrulama: `sanalpanel db:export`/`import`/`permissions:reset`/`cache:purge` gerçek bir test domain'iyle çalıştırılıp sonuç doğrulanır (mevcut çalışma tarzıyla tutarlı — DB/cache üzerinde doğrudan kontrol, UI'ye güvenmeden).

## Uygulama Notları (sonraki adım: writing-plans)

- `internal/domains/handlers.go` içindeki domain oluşturma akışına token üretimi eklenecek.
- `scripts/sanalpanel-jail` (ve `assets/ops/` altındaki güncel kopyası varsa) jail template'ine dispatcher script eklenecek.
- Yeni `internal/clicommands/` paketi + `cmd/server/main.go`'da ikinci (loopback-only) listener.
- Vhost template'inde (`internal/provisioner/provisioner.go` civarı) cache-version mekanizması için değişiklik.
- Migration: `cli_tokens` tablosu (`migrations/` altına).
