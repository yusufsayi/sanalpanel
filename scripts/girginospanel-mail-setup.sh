#!/usr/bin/env bash
# girginospanel-mail-setup — Postfix + Dovecot + OpenDKIM sanal posta kutusu altyapısını
# kurar/onarır. Idempotent. Kurulumda çalıştırılır; panelin "E-posta" özelliği bunu gerektirir.
#
# Postfix/Dovecot panel DB'sini (mail_domains/mailboxes/mail_aliases) CANLI MySQL sorgusuyla
# okur — bu yüzden panelde kutu ekleme/silme/askıya-alma servis restart'sız anında etkilidir.
# Webmail (Roundcube) BU betiğin kapsamında DEĞİL — ayrı bir aşamada eklenir.
set -uo pipefail
log(){ printf '  %s\n' "$*"; }

# TMPL: config template'lerinin okunacağı yer. Üretimde /usr/local/bin'e install edilen
# betiğin yanında assets/ YOKTUR — install.sh assets/mail/*'i kalıcı olarak
# /opt/girginospanel/src/mail-templates'e kopyalar (migrations/scripts ile aynı desen).
# Repo checkout'undan doğrudan çalıştırılıyorsa (yerel geliştirme/test) yanındaki assets/'e düşer.
TMPL="/opt/girginospanel/src/mail-templates"
if [ ! -d "$TMPL" ]; then
  HERE="$(cd "$(dirname "$0")" && pwd)"
  for cand in "$HERE/../assets/mail" "$HERE/assets/mail"; do
    [ -d "$cand" ] && TMPL="$cand" && break
  done
fi
[ -d "$TMPL" ] || { log "✗ mail template dizini bulunamadı ($TMPL)"; exit 1; }
ENV=/etc/girginospanel/env

echo "════ Postfix + Dovecot + OpenDKIM paketleri ════"
dnf install -y postfix dovecot opendkim >/tmp/mail-setup.log 2>&1 \
  && log "postfix + dovecot + opendkim kuruldu" || { log "kurulum uyarı (bazı paketler zaten olabilir)"; }

echo "════ mailro (salt-okunur) DB parolası ════"
DBPASS=$(grep -oP '^PANEL_MAIL_DB_PASS=\K.*' "$ENV" 2>/dev/null)
if [ -z "$DBPASS" ]; then
  DBPASS=$(openssl rand -hex 24)
  echo "PANEL_MAIL_DB_PASS=${DBPASS}" >> "$ENV"
  log "mailro parolası üretildi ve env'e eklendi"
fi

echo "════ mailro DB kullanıcısı (yalnızca SELECT — Postfix/Dovecot panel DB'sine yazamaz) ════"
mysql -u root <<SQL 2>/dev/null
CREATE USER IF NOT EXISTS 'mailro'@'127.0.0.1' IDENTIFIED BY '${DBPASS}';
ALTER USER 'mailro'@'127.0.0.1' IDENTIFIED BY '${DBPASS}';
GRANT SELECT ON panel.mail_domains TO 'mailro'@'127.0.0.1';
GRANT SELECT ON panel.mailboxes TO 'mailro'@'127.0.0.1';
GRANT SELECT ON panel.mail_aliases TO 'mailro'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
log "mailro kullanıcı + SELECT izinleri"

echo "════ Postfix: virtual-mailbox MySQL harita dosyaları ════"
for f in mysql-virtual-domains.cf mysql-virtual-mailboxes.cf mysql-virtual-uid.cf mysql-virtual-gid.cf mysql-virtual-aliases.cf; do
  sed "s/__PANEL_MAIL_DB_PASS__/${DBPASS}/" "$TMPL/postfix/${f}.tmpl" > "/etc/postfix/${f}"
  chown root:postfix "/etc/postfix/${f}"
  chmod 640 "/etc/postfix/${f}"
done
log "5 mysql-virtual-*.cf dosyası yazıldı (root:postfix 0640)"

echo "════ Postfix: main.cf / master.cf ════"
if ! grep -q 'girginospanel-mail' /etc/postfix/main.cf; then
  cat "$TMPL/postfix/main.cf.append" >> /etc/postfix/main.cf
  log "main.cf'e girginospanel-mail bloğu eklendi"
fi
if ! grep -qE '^submission\s+inet' /etc/postfix/master.cf; then
  cat "$TMPL/postfix/master.cf.append" >> /etc/postfix/master.cf
  log "master.cf'e submission (587) servisi eklendi"
fi

echo "════ Dovecot: SQL auth + drop-in config ════"
sed "s/__PANEL_MAIL_DB_PASS__/${DBPASS}/" "$TMPL/dovecot/dovecot-sql.conf.ext.tmpl" > /etc/dovecot/dovecot-sql.conf.ext
chown root:dovecot /etc/dovecot/dovecot-sql.conf.ext
chmod 640 /etc/dovecot/dovecot-sql.conf.ext
cp "$TMPL/dovecot/10-girginospanel-mail.conf.tmpl" /etc/dovecot/conf.d/10-girginospanel-mail.conf
log "dovecot-sql.conf.ext + conf.d/10-girginospanel-mail.conf"

echo "════ OpenDKIM ════"
mkdir -p /etc/opendkim/keys
touch /etc/opendkim/KeyTable /etc/opendkim/SigningTable
if [ ! -f /etc/opendkim/TrustedHosts ]; then
  printf '127.0.0.1\nlocalhost\n' > /etc/opendkim/TrustedHosts
fi
chown -R opendkim:opendkim /etc/opendkim
cp "$TMPL/opendkim/opendkim.conf.tmpl" /etc/opendkim/opendkim.conf
chown root:opendkim /etc/opendkim/opendkim.conf
log "opendkim.conf + KeyTable/SigningTable/TrustedHosts (boş, panel DKIM ürettikçe dolar)"

echo "════ Maildir kök dizinleri (mevcut aktif mail_domains için, varsa) ════"
# GÜVENLİK/SIRALAMA: bu betik girginospanel-install.sh'ta panel İLK KEZ başlatıldıktan
# (migration'lar uygulandıktan) SONRA çağrılır — ftp-setup ile birebir aynı sebep: aşağıdaki
# GRANT SELECT ifadeleri mail_domains/mailboxes/mail_aliases tabloları yokken ERROR 1146
# ile patlar (gerçek MariaDB'ye karşı doğrulandı). Elle/farklı sırada çalıştırırsan önce
# panelin en az bir kez ayağa kalkıp migration'ları uygulamış olduğundan emin ol.
mysql -u root -N -e "SELECT sistem_kullanici FROM panel.mail_domains WHERE durum='active'" 2>/dev/null | while read -r sk; do
  [ -n "$sk" ] || continue
  mkdir -p "/home/${sk}/mail"
  chown "${sk}:${sk}" "/home/${sk}/mail" 2>/dev/null
done

echo "════ SELinux ════"
setsebool -P httpd_can_network_connect_db 1 2>/dev/null && log "httpd_can_network_connect_db=1"
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" != "Disabled" ]; then
  log "UYARI: SELinux enforcing — postfix_t/dovecot_t'nin /etc/pki/girginospanel ve /home/*/mail" \
      "okuma/yazmasında AVC red'i olabilir. 'ausearch -m avc -ts recent' ile kontrol et; gerekirse" \
      "'girginospanel-repair --only mail' veya elle semanage/setsebool ile düzelt."
fi

echo "════ postfix + dovecot + opendkim enable + (re)start ════"
systemctl enable postfix dovecot opendkim >/dev/null 2>&1
if ! postfix check >/tmp/mail-postfix-check.log 2>&1; then
  log "✗ postfix check başarısız — /tmp/mail-postfix-check.log"; cat /tmp/mail-postfix-check.log
  exit 1
fi
systemctl restart opendkim; sleep 1
systemctl restart dovecot; sleep 1
systemctl restart postfix; sleep 2

OK=1
systemctl is-active --quiet postfix  || { log "✗ postfix başlatılamadı — journalctl -u postfix"; OK=0; }
systemctl is-active --quiet dovecot  || { log "✗ dovecot başlatılamadı — journalctl -u dovecot"; OK=0; }
systemctl is-active --quiet opendkim || { log "✗ opendkim başlatılamadı — journalctl -u opendkim"; OK=0; }
if [ "$OK" = 1 ]; then
  log "✓ postfix + dovecot + opendkim ACTIVE"
else
  exit 1
fi
echo "════════ ✓ Mail altyapısı hazır (webmail AYRI kurulur) ════════"
