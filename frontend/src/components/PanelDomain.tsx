import { useCallback, useEffect, useState } from 'react'
import { api } from '@/lib/api'

// Panel için özel alan adı + otomatik Let's Encrypt — CloudPanel'deki gibi paneli
// çıplak IP yerine kendi domaininizle, gerçek sertifikayla açabilmeyi sağlar.
// Panel her zaman 8443'te kalır (bkz. assets/nginx/_panel.conf) — sadece o portun
// arkasındaki sertifika + karşılanan Host değişir.

type Durum = {
  ozel_domain: string
  ssl_durum: 'yok' | 'aktif' | 'basarisiz'
  ssl_hata?: string
  ssl_bitis?: string
  sunucu_ip: string
}

export default function PanelDomain() {
  const [durum, setDurum] = useState<Durum | null>(null)
  const [domain, setDomain] = useState('')
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [uyari, setUyari] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)

  const yukle = useCallback(async () => {
    try {
      const r = await api.get<Durum>('/system/panel-domain')
      setDurum(r.data)
      setDomain(r.data.ozel_domain || '')
    } catch { /* geçici — yut */ }
  }, [])

  useEffect(() => { yukle() }, [yukle])

  async function kaydet() {
    setHata(null); setUyari(null); setBasari(null); setKaydediliyor(true)
    try {
      const r = await api.post('/system/panel-domain', { domain: domain.trim() })
      if (r.data.uyari) setUyari(r.data.uyari)
      else setBasari(`✓ Sertifika kuruldu — https://${domain.trim()} üzerinden erişebilirsiniz`)
      yukle()
    } catch (e: any) {
      setHata(e?.response?.data?.hata || e?.message || 'kaydedilemedi')
    } finally { setKaydediliyor(false) }
  }

  async function kaldir() {
    setHata(null); setUyari(null); setBasari(null); setKaydediliyor(true)
    try {
      await api.delete('/system/panel-domain')
      setDomain('')
      setBasari('✓ Özel domain kaldırıldı, panel sunucu IP + self-signed sertifikaya döndü')
      yukle()
    } catch (e: any) {
      setHata(e?.response?.data?.hata || e?.message || 'kaldırılamadı')
    } finally { setKaydediliyor(false) }
  }

  return (
    <div className="mb-6 p-4 border rounded-2xl bg-violet-50 dark:bg-violet-900/15 border-violet-200 dark:border-violet-800/50">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center text-xl flex-shrink-0 bg-violet-100 dark:bg-violet-900/40">🔗</div>
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-2">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">Panel Alan Adı</span>
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300">Otomatik SSL</span>
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
            Panele çıplak IP yerine kendi alan adınızla, port yazmadan girin. Domainin A kaydı bu sunucuyu
            {durum?.sunucu_ip ? <> (<code className="text-[11px]">{durum.sunucu_ip}</code>)</> : null} göstermelidir — kaydedince
            panel otomatik olarak gerçek bir Let's Encrypt sertifikası kurar. Panel her zaman
            <code className="text-[11px]"> https://{durum?.sunucu_ip || 'sunucu-ip'}:8443</code> üzerinden de erişilebilir kalır (yedek erişim).
          </div>

          {durum?.ssl_durum === 'aktif' && (
            <div className="mt-2 inline-flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-300">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
              SSL aktif{durum.ssl_bitis ? ` — ${durum.ssl_bitis} tarihine kadar geçerli` : ''} — https://{durum.ozel_domain} üzerinden port yazmadan erişebilirsiniz
            </div>
          )}
          {durum?.ssl_durum === 'basarisiz' && durum.ssl_hata && (
            <div className="mt-2 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 text-xs">
              Domain kaydedildi ama sertifika alınamadı: {durum.ssl_hata}
            </div>
          )}
          {hata && <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-xs">{hata}</div>}
          {uyari && <div className="mt-2 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 text-xs">{uyari}</div>}
          {basari && <div className="mt-2 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-xs">{basari}</div>}

          <div className="mt-3 flex items-center gap-2">
            <input
              type="text"
              value={domain}
              onChange={e => setDomain(e.target.value)}
              placeholder="panel.ornekalan.com"
              autoComplete="off"
              spellCheck={false}
              className="flex-1 max-w-xs px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded-lg text-xs font-mono bg-white dark:bg-slate-900 text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-violet-500"
            />
            <button onClick={kaydet} disabled={kaydediliyor || !domain.trim()}
              className="text-xs px-3 py-1.5 rounded-lg bg-violet-600 text-white hover:bg-violet-700 transition font-medium disabled:opacity-40 disabled:cursor-not-allowed">
              {kaydediliyor ? 'Kaydediliyor…' : 'Kaydet ve SSL kur'}
            </button>
            {durum?.ozel_domain && (
              <button onClick={kaldir} disabled={kaydediliyor}
                className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition disabled:opacity-40">
                Kaldır
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
