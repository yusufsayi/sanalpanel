package cliapi

import (
	"fmt"
	"net/http"
	"strings"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"
	"sanalpanel/internal/redis"
)

// POST /cache/purge  (form body: purge=all|fastcgi|redis, varsayılan "all")
func (h *Handlers) Purge(w http.ResponseWriter, r *http.Request) {
	domainID, sk, ok := DomainFrom(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
		return
	}
	_ = r.ParseForm()
	purge := r.FormValue("purge")
	if purge == "" {
		purge = "all"
	}
	if purge != "all" && purge != "fastcgi" && purge != "redis" {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz purge değeri (all|fastcgi|redis)")
		return
	}

	var mesajlar []string

	if purge == "redis" || purge == "all" {
		n, err := redis.PurgeSK(sk)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "redis purge: "+err.Error())
			return
		}
		mesajlar = append(mesajlar, fmt.Sprintf("redis: %d key silindi", n))
	}

	if purge == "fastcgi" || purge == "all" {
		if _, err := h.DB.Exec(`UPDATE domains SET cache_version = cache_version + 1 WHERE id=?`, domainID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "cache_version güncelle: "+err.Error())
			return
		}
		var php string
		if err := h.DB.QueryRow(`SELECT php_surum FROM domains WHERE id=?`, domainID).Scan(&php); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "domain oku: "+err.Error())
			return
		}
		socket, err := provisioner.PHPSocketFor(sk, php)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "socket: "+err.Error())
			return
		}
		if err := provisioner.ApplyVhostForDomain(h.DB, domainID, socket, php); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "vhost yeniden render: "+err.Error())
			return
		}
		mesajlar = append(mesajlar, "fastcgi: cache-version artırıldı, nginx reload edildi")
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "mesaj": strings.Join(mesajlar, "; ")})
}
