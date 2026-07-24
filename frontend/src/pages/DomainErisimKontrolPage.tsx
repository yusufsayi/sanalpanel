import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Hotlink = { aktif: boolean; izinli?: string[] }
type IPKural = { id: number; ip_cidr: string; created_at: string }
type IPMod = 'kapali' | 'engelle' | 'izin_ver'

export default function DomainErisimKontrolPage() {
  const { id } = useParams()
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)

  const [hotlink, setHotlink] = useState<Hotlink>({ aktif: false, izinli: [] })
  const [hotlinkIzinliMetin, setHotlinkIzinliMetin] = useState('')
  const [hotlinkKaydediyor, setHotlinkKaydediyor] = useState(false)

  const [ipMod, setIpMod] = useState<IPMod>('kapali')
  const [ipListe, setIpListe] = useState<IPKural[]>([])
  const [ipYeni, setIpYeni] = useState('')
  const [ipKaydediyor, setIpKaydediyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    Promise.all([
      api.get<Hotlink>(`/domains/${id}/hotlink`),
      api.get<{ mod: IPMod; kurallar: IPKural[] }>(`/domains/${id}/ip-kurallari`),
    ])
      .then(([h, i]) => {
        setHotlink(h.data)
        setHotlinkIzinliMetin((h.data.izinli || []).join(', '))
        setIpMod(i.data.mod || 'kapali')
        setIpListe(i.data.kurallar || [])
      })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function hotlinkKaydet(aktif: boolean) {
    setHata(null); setOk(null); setHotlinkKaydediyor(true)
    try {
      const izinli = hotlinkIzinliMetin.split(',').map(s => s.trim()).filter(Boolean)
      await api.put(`/domains/${id}/hotlink`, { aktif, izinli })
      setOk(aktif ? 'Hotlink koruması etkinleştirildi.' : 'Hotlink koruması kapatıldı.')
      yukle()
    } catch (e) { setHata(apiHata(e, 'Kaydedilemedi')) }
    finally { setHotlinkKaydediyor(false) }
  }

  async function ipModKaydet(mod: IPMod) {
    setHata(null); setOk(null); setIpKaydediyor(true)
    try {
      await api.put(`/domains/${id}/ip-kurallari/mod`, { mod })
      setIpMod(mod)
      setOk('Mod güncellendi.')
      yukle()
    } catch (e) { setHata(apiHata(e, 'Kaydedilemedi')) }
    finally { setIpKaydediyor(false) }
  }

  async function ipEkle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setIpKaydediyor(true)
    try {
      await api.post(`/domains/${id}/ip-kurallari`, { ip_cidr: ipYeni.trim() })
      setIpYeni('')
      yukle()
    } catch (e2) { setHata(apiHata(e2, 'Eklenemedi')) }
    finally { setIpKaydediyor(false) }
  }

  async function ipSil(k: IPKural) {
    setHata(null); setOk(null)
    try { await api.delete(`/domains/${id}/ip-kurallari/${k.id}`); yukle() }
    catch (e) { setHata(apiHata(e, 'Silinemedi')) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: 'Anasayfa', href: '/' },
        { etiket: 'Domainler', href: '/domainler' },
        { etiket: 'Erişim Kontrolü' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🚧</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Erişim Kontrolü</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">Hotlink koruması ve IP bazlı izin/engel listesi.</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

      {yuk ? <div className="text-sm text-slate-400 py-2">Yükleniyor…</div> : (
        <>
          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
            <div className="flex items-center justify-between mb-1">
              <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Hotlink Koruması</h3>
              <span className={`text-[10px] font-semibold uppercase tracking-wider px-1.5 py-0.5 rounded ${hotlink.aktif ? 'text-emerald-700 dark:text-emerald-300 bg-emerald-100 dark:bg-emerald-900/30' : 'text-slate-500 bg-slate-100 dark:bg-slate-700/50'}`}>
                {hotlink.aktif ? 'aktif' : 'kapalı'}
              </span>
            </div>
            <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
              Resimleriniz (jpg, png, gif, webp, svg…) başka sitelerden doğrudan bağlantıyla (hotlink) gösterilemez; yalnızca kendi domaininizden ve aşağıda izin verdiğiniz adreslerden gelen istekler resimlere erişebilir.
            </p>
            <label className="block mb-3">
              <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">İzinli ekstra domainler (virgülle ayrık, opsiyonel)</span>
              <input value={hotlinkIzinliMetin} onChange={e => setHotlinkIzinliMetin(e.target.value)} placeholder="cdn.baskadomain.com, *.ortak.com"
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <button onClick={() => hotlinkKaydet(!hotlink.aktif)} disabled={hotlinkKaydediyor}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {hotlinkKaydediyor ? 'Kaydediliyor…' : hotlink.aktif ? 'Kapat' : 'Etkinleştir'}
            </button>
            {hotlink.aktif && (
              <button onClick={() => hotlinkKaydet(true)} disabled={hotlinkKaydediyor} className="ml-2 px-4 py-2 border border-slate-300 dark:border-slate-600 text-sm rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">
                İzinli Listeyi Güncelle
              </button>
            )}
          </div>

          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">IP İzin/Engel Listesi</h3>
            <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">Bu domaine erişimi belirli IP adreslerine kısıtlayın ya da kötü niyetli IP'leri engelleyin.</p>
            <div className="flex flex-wrap gap-2 mb-3">
              {(['kapali', 'engelle', 'izin_ver'] as IPMod[]).map(m => (
                <button key={m} onClick={() => ipModKaydet(m)} disabled={ipKaydediyor}
                  className={`px-3 py-1.5 text-xs rounded-lg border disabled:opacity-50 ${ipMod === m
                    ? 'bg-slate-900 dark:bg-white text-white dark:text-slate-900 border-slate-900 dark:border-white'
                    : 'border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                  {m === 'kapali' ? 'Kapalı' : m === 'engelle' ? 'Listedekileri Engelle' : 'Yalnızca Listedekilere İzin Ver'}
                </button>
              ))}
            </div>

            {ipMod !== 'kapali' && (
              <>
                <form onSubmit={ipEkle} className="flex items-end gap-2 mb-3">
                  <label className="block flex-1">
                    <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">IP veya CIDR</span>
                    <input value={ipYeni} onChange={e => setIpYeni(e.target.value)} required placeholder="1.2.3.4 veya 1.2.3.0/24"
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  </label>
                  <button disabled={ipKaydediyor || !ipYeni.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    Ekle
                  </button>
                </form>
                {ipListe.length === 0 ? (
                  <p className="text-sm text-slate-500 dark:text-slate-400 py-2">
                    {ipMod === 'izin_ver' ? 'Henüz kural yok — liste boşken mod devre dışı sayılır (site kilitlenmez).' : 'Henüz engellenen IP yok.'}
                  </p>
                ) : (
                  <ul className="divide-y divide-slate-100 dark:divide-slate-700/60">
                    {ipListe.map(k => (
                      <li key={k.id} className="flex items-center justify-between py-2">
                        <span className="font-mono text-sm text-slate-800 dark:text-slate-200">{k.ip_cidr}</span>
                        <button onClick={() => ipSil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Sil</button>
                      </li>
                    ))}
                  </ul>
                )}
              </>
            )}
          </div>
        </>
      )}

      <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Aboneliğe dön</Link></div>
    </div>
  )
}
