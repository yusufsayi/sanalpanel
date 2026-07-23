package cliapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"sanalpanel/internal/httpx"
)

type ctxKey int

const (
	ctxDomainID ctxKey = iota
	ctxSK
)

// RequireToken: "Authorization: Bearer <token>" header'ını doğrular, geçerliyse
// domain_id + sistem_kullanici'yi request context'ine koyar.
func RequireToken(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, prefix) {
				httpx.WriteError(w, http.StatusUnauthorized, "Authorization header eksik")
				return
			}
			raw := strings.TrimPrefix(auth, prefix)
			domainID, sk, ok := Lookup(db, raw)
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, "geçersiz token")
				return
			}
			ctx := context.WithValue(r.Context(), ctxDomainID, domainID)
			ctx = context.WithValue(ctx, ctxSK, sk)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DomainFrom: RequireToken middleware'inin context'e koyduğu domain bilgisini okur.
func DomainFrom(r *http.Request) (domainID int64, sk string, ok bool) {
	domainID, ok1 := r.Context().Value(ctxDomainID).(int64)
	sk, ok2 := r.Context().Value(ctxSK).(string)
	return domainID, sk, ok1 && ok2
}
