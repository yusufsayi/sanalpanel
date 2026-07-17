package domains

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"girginospanel/internal/httpx"
	"girginospanel/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// AskiyaAl: POST /domains/{id}/askiya-al — hesabı askıya al.
// domains.askida=1 + durum=pasif işaretlenir, vhost 503 "askıya alındı" olarak yeniden render edilir.
// (Askıda durumu DB'de kalıcıdır; SetPHP/SSL gibi her yeniden render'da tekrar uygulanır.)
func (h *Handlers) AskiyaAl(w http.ResponseWriter, r *http.Request) {
	h.askiToggle(w, r, true)
}

// AskidanAl: POST /domains/{id}/askidan-al — askıyı kaldır, siteyi geri getir.
func (h *Handlers) AskidanAl(w http.ResponseWriter, r *http.Request) {
	h.askiToggle(w, r, false)
}

func (h *Handlers) askiToggle(w http.ResponseWriter, r *http.Request, askida bool) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, is_demo FROM domains WHERE id=?`, id).Scan(&alanAdi, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo abonelik askıya alınamaz")
		return
	}

	ak := 0
	durum := "aktif"
	if askida {
		ak = 1
		durum = "pasif"
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET askida=?, durum=? WHERE id=?`, ak, durum, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}

	// vhost'u yeniden render et (askida durumu DB'den okunur)
	if err := provisioner.RerenderVhost(h.DB, id); err != nil {
		// DB güncellendi ama vhost render başarısız → geri al ki tutarlı kalsın
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE domains SET askida=?, durum=? WHERE id=?`, 1-ak, map[bool]string{true: "aktif", false: "pasif"}[askida], id)
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "alan_adi": alanAdi, "askida": askida,
	})
}
