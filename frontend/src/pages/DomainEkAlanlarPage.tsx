import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Ek = { id: number; alan_adi: string; parked: boolean; docroot: string; php_surum: string; ssl_aktif: boolean; created_at: string }
type Yonlendirme = { aktif: boolean; hedef_url?: string; kod?: number }

export default function DomainEkAlanlarPage() {
  const { id } = useParams()
  const [liste, setListe] = useState<Ek[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [alanAdi, setAlanAdi] = useState('')
  const [parked, setParked] = useState(false)
  const [kaydediyor, setKaydediyor] = useState(false)

  const [yonlendirme, setYonlendirme] = useState<Yonlendirme | null>(null)
  const [hedefUrl, setHedefUrl] = useState('')
  const [kod, setKod] = useState(301)
  const [yonKaydediyor, setYonKaydediyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    Promise.all([
      api.get<Ek[]>(`/domains/${id}/ek`),
      api.get<Yonlendirme>(`/domains/${id}/yonlendirme`),
    ])
      .then(([e, y]) => {
        setListe(e.data || [])
        setYonlendirme(y.data)
        if (y.data.aktif) { setHedefUrl(y.data.hedef_url || ''); setKod(y.data.kod || 301) }
      })
      .catch(err => setHata(apiHata(err)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function olustur(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setKaydediyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/ek`, { alan_adi: alanAdi.trim().toLowerCase(), parked })
      setOk(`${data.alan_adi} eklendi (${parked ? 'parked — aynı içerik' : 'addon — kendi dizini'}). DNS zone oluşturuldu.`)
      setAlanAdi(''); setParked(false)
      yukle()
    } catch (err) { setHata(apiHata(err, 'Eklenemedi')) }
    finally { setKaydediyor(false) }
  }

  async function sil(ek: Ek) {
    if (!confirm(`"${ek.alan_adi}" silinsin mi?\n${ek.parked ? '' : 'Docroot dosyaları da silinir. '}DNS zone kaldırılır. Geri alınamaz.`)) return
    setHata(null); setOk(null)
    try { await api.delete(`/domains/${id}/ek/${ek.id}`); yukle() }
    catch (err) { setHata(apiHata(err, 'Silinemedi')) }
  }

  async function yonlendirmeKaydet(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setYonKaydediyor(true)
    try {
      await api.put(`/domains/${id}/yonlendirme`, { hedef_url: hedefUrl.trim(), kod })
      setOk('Yönlendirme kaydedildi ve vhost güncellendi.')
      yukle()
    } catch (err) { setHata(apiHata(err, 'Kaydedilemedi')) }
    finally { setYonKaydediyor(false) }
  }

  async function yonlendirmeKaldir() {
    if (!confirm('Yönlendirme kaldırılsın mı? Domain normal içeriğini göstermeye döner.')) return
    setHata(null); setOk(null)
    try { await api.delete(`/domains/${id}/yonlendirme`); setHedefUrl(''); yukle() }
    catch (err) { setHata(apiHata(err, 'Kaldırılamadı')) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: 'Anasayfa', href: '/' },
        { etiket: 'Domainler', href: '/domainler' },
        { etiket: 'Ek Alan Adları ve Yönlendirme' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🧩</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Ek Alan Adları ve Yönlendirme</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">
        Bu hesap altında tamamen farklı, bağımsız kayıtlı alan adları barındırın (addon) ya da bu domainin içeriğini aynen gösteren bir alias ekleyin (parked). Ayrıca bu domaini başka bir adrese yönlendirebilirsiniz.
      </p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

      <form onSubmit={olustur} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">Yeni Ek Alan Adı</h3>
        <div className="flex flex-wrap items-end gap-2">
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Alan Adı</span>
            <input value={alanAdi} onChange={e => setAlanAdi(e.target.value)} required placeholder="baskadomain.com"
              className="mt-1 w-56 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300 pb-2.5">
            <input type="checkbox" checked={parked} onChange={e => setParked(e.target.checked)} />
            Parked (aynı içeriği göster, ayrı dizin yok)
          </label>
          <button disabled={kaydediyor || !alanAdi.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {kaydediyor ? 'Ekleniyor…' : 'Ekle'}
          </button>
        </div>
        <p className="text-[11px] text-slate-400 mt-2">Bu hesabın plan kotasından (max domain) düşer. SSL şu an için desteklenmiyor (yalnızca HTTP).</p>
      </form>

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">Mevcut Ek Alan Adları</h3>
        {yuk ? <div className="text-sm text-slate-400 py-2">Yükleniyor…</div>
          : liste.length === 0 ? (
            <div className="text-center py-6">
              <p className="text-sm text-slate-500 dark:text-slate-400">Henüz ek alan adı yok.</p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-700/60">
              {liste.map(ek => (
                <li key={ek.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="min-w-0">
                    <a href={`http://${ek.alan_adi}`} target="_blank" rel="noreferrer" className="font-mono text-sm text-brand-600 dark:text-brand-400 hover:underline">{ek.alan_adi}</a>
                    <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-700/50 px-1.5 py-0.5 rounded">
                      {ek.parked ? 'parked' : 'addon'}
                    </span>
                    <div className="text-[11px] text-slate-400 font-mono truncate">{ek.docroot} · PHP {ek.php_surum}</div>
                  </div>
                  <button onClick={() => sil(ek)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 shrink-0">Sil</button>
                </li>
              ))}
            </ul>
          )}
      </div>

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">Tüm-Domain Yönlendirme</h3>
        <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">Bu domaine gelen TÜM istekler belirttiğiniz adrese yönlendirilir (301/302). Path bazlı yönlendirme desteklenmiyor.</p>
        <form onSubmit={yonlendirmeKaydet} className="flex flex-wrap items-end gap-2">
          <label className="block flex-1 min-w-[220px]">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Hedef URL</span>
            <input value={hedefUrl} onChange={e => setHedefUrl(e.target.value)} required placeholder="https://yenidomain.com"
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Kod</span>
            <select value={kod} onChange={e => setKod(Number(e.target.value))}
              className="mt-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
              <option value={301}>301 (kalıcı)</option>
              <option value={302}>302 (geçici)</option>
            </select>
          </label>
          <button disabled={yonKaydediyor || !hedefUrl.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {yonKaydediyor ? 'Kaydediliyor…' : yonlendirme?.aktif ? 'Güncelle' : 'Yönlendirmeyi Etkinleştir'}
          </button>
          {yonlendirme?.aktif && (
            <button type="button" onClick={yonlendirmeKaldir} className="px-3 py-2 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 text-sm rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20">
              Kaldır
            </button>
          )}
        </form>
      </div>

      <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Aboneliğe dön</Link></div>
    </div>
  )
}
