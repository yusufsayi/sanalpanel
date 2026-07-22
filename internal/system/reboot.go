package system

// Sunucuyu Yeniden Başlat — /araclar-ayarlar sayfasındaki kırmızı buton.
//
// GÜVENLİK: argüman YOK, komut tamamen SABİT — hiçbir kullanıcı girdisi geçmez.
// Reboot, HTTP yanıtı istemciye ulaştıktan SONRA gerçekleşsin diye systemd-run ile
// birkaç saniye geciktirilerek zamanlanır (aynı desen: internal/system/optimize.go'daki
// OptimizeBaslat — transient unit PID 1 altında, panelin kendi cgroup'unda DEĞİL, bu
// yüzden panel süreci öldüğünde iş yarıda kesilmez).

import (
	"net/http"
	"os/exec"
	"strings"

	"sanalpanel/internal/httpx"
)

// Reboot — POST /system/reboot: sunucuyu ~5sn sonra yeniden başlatır.
func Reboot(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("systemd-run",
		"--on-active=5",
		"--unit=sanalpanel-reboot",
		"--description=SanalPanel: sunucu yeniden başlatma",
		"--", "systemctl", "reboot")
	if out, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "başlatılamadı: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"ok":    true,
		"mesaj": "Sunucu birkaç saniye içinde yeniden başlatılacak.",
	})
}
