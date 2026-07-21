<p align="center">
  <a href="https://github.com/yusufsayi/sanalpanel"><b>🌐 GitHub</b></a> &nbsp;·&nbsp;
  <a href="README.md">Türkçe</a> &nbsp;·&nbsp;
  <a href="README.en.md">English</a>
</p>

# SanalPanel

Turns a blank **AlmaLinux 10** server into a complete hosting control panel with a single command — nginx + MariaDB + multi-version PHP + Valkey (Redis) + phpMyAdmin + firewall, all installed and configured automatically.

## One-line install

On a clean AlmaLinux 10 server (min. 2 GB RAM), as **root**:

```bash
curl -fsSL https://raw.githubusercontent.com/yusufsayi/sanalpanel/main/install.sh | bash
```

Installation takes ~5-10 minutes (package downloads). When it finishes, the panel URL and login credentials are printed to the screen.

## After install

- **Panel:** `https://SERVER_IP:8443` (self-signed certificate — click through the browser warning)
- **Login:** user **`root`** · password = **the server's own root password**
  (the panel admin is authenticated against the OS root account via PAM — there is no separate panel password)

## What it installs

| Component | Detail |
|---|---|
| **Web** | nginx (panel on :8443 + customer sites on :80/:443) |
| **PHP** | 7.4 / 8.2 / 8.3 / 8.4 / 8.5 (remi) — each domain picks its own version, per-domain FPM pool |
| **Database** | MariaDB 10.11 (`panel` DB) + phpMyAdmin (`/pma/`) |
| **Cache** | Valkey (Redis) — per-tenant isolated object cache (auto-connects to WordPress) |
| **Email** | Postfix + Dovecot + OpenDKIM — SMTP AUTH (587), IMAP, automatic DKIM/SPF/DMARC; webmail (Roundcube, `/webmail/`) |
| **Security** | nftables firewall, SELinux-compatible, ClamAV |
| **Performance** | Automatic MariaDB + nginx + OPcache tuning (`sanalpanel-optimize`) |

## Panel features

- Domain / subdomain management, DNS editing, bulk operations
- One-click **WordPress** install + WP-CLI
- Per-tenant **Redis object cache** (toggle on/off, auto-wires into WordPress)
- **Email hosting**: a mailbox per domain, authenticated SMTP sending (for PHPMailer / application integrations), automatic DKIM/SPF/DMARC DNS records, webmail — see below for details
- **Custom vhost mode** (admin only): full nginx vhost editing per domain, for routing needs the template's single-root model can't express — see below for details
- **Firewall** UI (IP ban / whitelist / port closing + ready-made templates)
- Backup manager, monitoring/logs, statistics
- Service plans and resource limits (new domains default to the **Starter** plan)

## Email (Mail Hosting)

You can open mailboxes for any domain directly from the panel — a self-hosted email system built on Postfix + Dovecot + OpenDKIM (no dependency on a third-party SMTP provider).

- From the **domain page → Email** tab, first enable mail for the domain (MX/SPF/DKIM/DMARC records are added to DNS automatically), then create mailboxes.
- **SMTP AUTH (587, STARTTLS)** — an authenticated sending endpoint that application libraries like PHPMailer or Nodemailer can connect to directly. It is not an open relay; only your own mailbox credentials can be used to send.
- **DKIM signing is automatic** — outgoing mail is signed the moment a mailbox is created, no extra setup needed.
- **Webmail**: access your mailbox from a browser via `https://SERVER_IP:8443/webmail/` (Roundcube) — log in with your mailbox address and password.
- Abuse-prevention rate limiting (per connection/message) and SASL brute-force protection are included.
- Note: inbound mail (port 25) is blocked at the network level by default on some hosting providers (as an anti-spam measure) — if inbound mail isn't reaching your server, ask your provider to open port 25. Outbound SMTP AUTH (587) is unaffected by this.

## Custom Vhost Mode

The panel's standard settings (security headers, caching, the "extra directives" field) are enough for most sites. But sometimes you need a layout the single-`root` template simply can't express — for example, one application at a domain's root and a different one under a subpath like `/blog`.

**Custom Vhost Mode** (Domain → Hosting & DNS → Apache & nginx → "Custom Vhost Mode", admin only) lets you view and edit the full nginx vhost file for that reason:

- When you open it, you start from the **file that's actually running right now** — not a blank box.
- On save, it's validated with `nginx -t` — an invalid configuration is never applied to the live server; both the database and the running file stay safe.
- **Once you turn it on, the panel never touches that file again** — automatic actions like SSL renewal or PHP version changes will use your saved content instead of the template for that domain. This means if you remove the Let's Encrypt validation block (`/.well-known/acme-challenge/`) from the file, certificate renewal will start failing after 90 days.
- If the domain is suspended, the "suspended" page is always shown, even in custom vhost mode — this safety behavior cannot be bypassed.
- Turning it off does not delete your saved content — turn it back on and you pick up where you left off.

## System requirements

- **AlmaLinux 10** (RHEL 10 / Rocky 10 also work)
- At least **2 GB RAM**, 2 vCPUs (for 5 PHP versions + MariaDB + Valkey)
- Root access + an internet connection

## Post-install helper tools

Installation places these tools in `/usr/local/bin`:

```bash
sanalpanel-update        # safely update the panel from GitHub (see below)
sanalpanel-optimize      # re-tune MariaDB/nginx/PHP for the server's resources
sanalpanel-redis-setup   # install/repair the Valkey (Redis) infrastructure
sanalpanel-wp-redis <sk> # connect/disconnect Redis cache for a domain's WordPress
sanalpanel-repair        # permission / SELinux / ownership repair (idempotent)
sanalpanel-db-backup     # take a compressed dump of the panel DB (see below)
```

## Backups

### Panel database (`panel`)

A **daily automatic backup** is included from install — you don't need to set anything up:

| | |
|---|---|
| **When** | Every day at **03:30** (`sanalpanel-db-backup.timer`, ±5 min random jitter) |
| **Where** | `/var/backups/sanalpanel/db/panel-<DATE>.sql.gz` (directory `0700`, dump `0600`) |
| **Retention** | **14 days** — older backups are removed automatically |
| **Scope** | the `panel` schema + routines/triggers/events (`mysqldump --single-transaction` → lock-free consistent snapshot) |

To take a manual backup (prints the path of the resulting file):

```bash
sanalpanel-db-backup
# /var/backups/sanalpanel/db/panel-2026-07-17-143052.sql.gz
```

To check the timer's status / see when it next runs:

```bash
systemctl list-timers sanalpanel-db-backup.timer
systemctl status sanalpanel-db-backup.timer
journalctl -u sanalpanel-db-backup -n 20    # log of recent backups
```

To restore a backup:

```bash
systemctl stop sanalpanel
zcat /var/backups/sanalpanel/db/panel-2026-07-17-143052.sql.gz | mysql
systemctl start sanalpanel
```

> Backups are **fail-closed**: if gzip integrity can't be verified, or the file is suspiciously small, the dump does **not** get named `panel-*.sql.gz` — a half-written dump never looks like a valid backup.

### Automatic pre-update backup

`sanalpanel-update` takes a full dump of the panel DB **before applying migrations**. If the dump fails, **the update never starts** (a migration without a backup is refused). See the "Update" section below for details.

### Customer sites

Customer sites + databases are backed up by a separate process: `sanalpanel-backup-all` (cron, daily at 03:00 UTC, `/var/backups/sanalpanel/<system_user>/`, 14-day retention). The panel DB backup never touches these directories.

## Update (SSH / CLI)

On an installed panel, over SSH as root, a single command:

```bash
sanalpanel-update            # pull the latest release from GitHub → swap binary+frontend+migrations → restart
sanalpanel-update --dry-run  # show what it would do first (no changes)
sanalpanel-update --force    # re-apply even if the binary is unchanged
sanalpanel-update --branch X # use a different branch
```

- **Safe and data-preserving:** `/etc/sanalpanel/env` (JWT/DB/Redis secrets), the MariaDB `panel` database, and `/home/c_*` customer sites are **never deleted**. Unlike `install.sh`, it does not generate new secrets.
- New migrations are applied **automatically and idempotently** when the service restarts.
- If the binary hasn't changed (SHA matches), nothing happens.
- **A full dump of the panel DB is taken before migrations run** → `/var/backups/sanalpanel/db/`.
- **Fail-closed:** if the dump fails, the update **never starts** — the binary, frontend, and migrations are left untouched. A migration without a backup is never accepted.
- If the new version doesn't start up healthy, it **automatically rolls back to the previous binary _and_ the pre-update DB** (no write loss, since the panel is already stopped at that point).

> Deploying your own fork: build from source (`GOAMD64=v1 go build` + `npm run build`), update `assets/sanalpanel-server` + `assets/frontend-dist.tar.gz`, push to your repo — `sanalpanel-update` on your servers will pull the new release. **Always build the binary with `GOAMD64=v1`** (see the warning under "Backend (Go)" below) — otherwise the panel won't start on older customer-server CPUs.

## Notes

- Installation is **not idempotent** — every run generates new secrets (JWT/DB password). Use `sanalpanel-repair` / `sanalpanel-optimize` instead of re-running it.
- The panel is served over HTTP/2 + self-signed SSL on :8443; a real domain with Let's Encrypt can be added through the panel itself.

---

## Building from source & development

This project is **fully open source** (MIT). Instead of installing the prebuilt binary, you can build and develop from source yourself — contributions are welcome.

### Requirements

- **Go 1.23+** (backend)
- **Node.js 20+** and **npm** (frontend)
- MariaDB/MySQL access to run it (the backend applies migrations + seeds the admin on startup)

### Backend (Go)

> ⚠️ **The release binary must be built with `GOAMD64=v1`.** AlmaLinux 10 (go1.26+) defaults to producing `GOAMD64=v3`; a binary built with v3 will simply **not run** on older/common customer CPUs that lack v3 microarchitecture support (AVX2 etc.), failing with `"This program can only be run on AMD64 processors with v3 microarchitecture support"`. `assets/sanalpanel-server` must always be built with `GOAMD64=v1`
> (use `scripts/build-assets.sh` for convenience — it already pins this).

```bash
# build a single static binary (GOAMD64=v1 is REQUIRED for old-CPU compatibility)
CGO_ENABLED=0 GOAMD64=v1 go build -o sanalpanel-server ./cmd/server

# run it (with environment variables)
PANEL_JWT_SECRET="$(openssl rand -hex 32)" \
PANEL_DB_DSN="root@unix(/var/lib/mysql/mysql.sock)/panel" \
./sanalpanel-server
```

The backend API lives under `/api/v1`; health check at `/healthz`. Admin login is authenticated against the OS root account via PAM in production; in development you can seed a separate admin with `scripts/seed_admin.go`:

```bash
go run scripts/seed_admin.go -dsn '<DSN>' -kullanici admin -parola 'YOUR_CHOSEN_PASSWORD'
# or: the PANEL_SEED_PAROLA env var
```

### Frontend (React + Vite + TypeScript)

```bash
cd frontend
npm install
npm run dev        # dev server on :5185 (proxies /api → VITE_API_PROXY)
npm run build      # production build → frontend/dist/
```

Set where the dev server proxies the backend to via `VITE_API_PROXY` (default `http://localhost:8080`):

```bash
VITE_API_PROXY=http://localhost:8080 npm run dev
```

### Repository layout

```
cmd/server/       Go entry point (main)
internal/         Backend packages (domains, wordpress, dns, redis, guvenlikduvari, github, backups, ...)
frontend/src/     React UI (pages/, components/, lib/)
migrations/       SQL schema migrations (applied on startup)
scripts/          Ops helper scripts (optimize, repair, redis-setup, seed_admin, ...)
assets/           Prebuilt release artifacts used by the installer
install.sh        One-line bootstrap (downloads the repo → runs sanalpanel-install.sh)
```

> The prebuilt binary and `frontend-dist.tar.gz` under `assets/` exist so the `curl | bash` install works without building from source. When you publish your own changes, update these from the `go build` / `npm run build` output above.

## Contributing & license

- Contributions (issues / PRs) are welcome.
- License: **MIT** — see [LICENSE](LICENSE). You may use, modify, distribute, and use it in your own product.

## Updating

To update the panel to the latest release, on the server:

```bash
sanalpanel-update              # install the latest release
sanalpanel-update --dry-run    # only show what it would do
sanalpanel-update --force      # re-apply even if it's the same version
```

You can also update from inside the panel: **Tools & Settings → Panel Update → "Check for and install updates"**.

The update **preserves** (never touches): `/etc/sanalpanel/env` (JWT/DB/Redis secrets), the MariaDB `panel` database + all customer data, and `/home/c_*` sites.

Before applying migrations, the update takes a full dump of the panel DB into `/var/backups/sanalpanel/db/`. If the dump fails, the update **never starts** (a migration without a backup is refused). If the new version doesn't come up healthy, it automatically **rolls back to the previous binary + the pre-update DB**.

### If you get "sanalpanel-update: command not found"

If you installed your panel before the update tool was added to the distribution, the command won't exist on your server yet. Since the only way to get the tool is the tool itself, this is a one-time chicken-and-egg problem — fix it with:

```bash
curl -fsSL https://raw.githubusercontent.com/yusufsayi/sanalpanel/main/assets/ops/sanalpanel-update \
  -o /usr/local/bin/sanalpanel-update && chmod +x /usr/local/bin/sanalpanel-update

sanalpanel-update
```

You only need to do this **once**: every time `sanalpanel-update` runs, it reinstalls all the tools under `assets/ops/` into `/usr/local/bin`, keeping itself up to date too. After this, you can also use the **Panel Update** button inside the panel.

> The in-panel update button **downloads the tool automatically** if it's missing — so clicking it is enough on its own; the command above is only for cases where you can't reach the panel at all.
