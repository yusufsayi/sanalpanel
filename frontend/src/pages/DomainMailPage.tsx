import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; alan_adi: string }
type Mailbox = { id: number; local_part: string; email: string; status: string; created_at: string }
type Durum = { etkin: boolean; dkim_selector?: string }
type Alias = { id: number; source: string; destination: string; catch_all: boolean; status: string; created_at: string }

export default function DomainMailPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [durum, setDurum] = useState<Durum | null>(null)
  const [liste, setListe] = useState<Mailbox[]>([])
  const [aliasListe, setAliasListe] = useState<Alias[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [localPart, setLocalPart] = useState('')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [yeniPw, setYeniPw] = useState<{ email: string; parola: string } | null>(null)
  const [aliasKaynak, setAliasKaynak] = useState('')
  const [aliasCatchAll, setAliasCatchAll] = useState(false)
  const [aliasHedef, setAliasHedef] = useState('')
  const [aliasIsleniyor, setAliasIsleniyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    Promise.all([
      api.get<Durum>(`/domains/${id}/mail/durum`),
      api.get<Mailbox[]>(`/domains/${id}/mail`),
      api.get<Alias[]>(`/domains/${id}/mail/aliases`),
    ])
      .then(([d, m, a]) => { setDurum(d.data); setListe(m.data || []); setAliasListe(a.data || []) })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(e => setHata(apiHata(e)))
    yukle()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  async function etkinlestir() {
    setIsleniyor(true); setHata(null)
    try {
      await api.post(`/domains/${id}/mail/etkinlestir`)
      setOk('E-posta bu domain için etkinleştirildi. MX/SPF/DKIM/DMARC kayıtları DNS bölümüne eklendi.')
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Etkinleştirilemedi'))
    } finally {
      setIsleniyor(false)
    }
  }

  async function ekle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setYeniPw(null); setIsleniyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/mail`, { local_part: localPart, parola })
      setYeniPw({ email: data.email, parola: data.parola })
      setLocalPart(''); setParola('')
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, 'Kutu oluşturulamadı'))
    } finally {
      setIsleniyor(false)
    }
  }

  async function sil(k: Mailbox) {
    if (!confirm(`"${k.email}" kutusunu silmek istediğinize emin misiniz? (Maildir diskte kalır, yalnızca hesap kaldırılır.)`)) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/${k.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
    }
  }

  async function parolaSifirla(k: Mailbox) {
    setHata(null); setOk(null); setYeniPw(null)
    try {
      const { data } = await api.put(`/domains/${id}/mail/${k.id}/parola`, {})
      setYeniPw({ email: k.email, parola: data.parola })
    } catch (e) {
      setHata(apiHata(e, 'Parola sıfırlanamadı'))
    }
  }

  async function aliasEkle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setAliasIsleniyor(true)
    try {
      await api.post(`/domains/${id}/mail/aliases`, {
        local_part: aliasCatchAll ? '' : aliasKaynak,
        destination: aliasHedef,
      })
      setAliasKaynak(''); setAliasHedef(''); setAliasCatchAll(false)
      setOk('Yönlendirme eklendi.')
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, 'Yönlendirme eklenemedi'))
    } finally {
      setAliasIsleniyor(false)
    }
  }

  async function aliasSil(a: Alias) {
    if (!confirm(`"${a.source}" yönlendirmesini silmek istediğinize emin misiniz?`)) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/aliases/${a.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
    }
  }

  async function aliasDurumDegistir(a: Alias) {
    setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/aliases/${a.id}/durum`, { status: a.status === 'active' ? 'suspended' : 'active' })
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Durum değiştirilemedi'))
    }
  }

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { etiket: 'Anasayfa', href: '/' },
          { etiket: 'Domainler', href: '/domainler' },
          { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
          { etiket: 'E-posta' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">E-posta Hesapları</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          Postfix/Dovecot tabanlı posta kutuları. SMTP (587, STARTTLS) uygulamalarınızda (PHPMailer vb.) kimlik doğrulamalı gönderim için kullanılabilir.
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        {yeniPw && (
          <div className="mb-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">✓ {yeniPw.email} parolası</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">Bunu güvenli bir yere kaydedin, sonra tekrar gösterilmez:</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{yeniPw.parola}</code>
              <button onClick={() => navigator.clipboard.writeText(yeniPw.parola)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">Kopyala</button>
            </div>
          </div>
        )}

        {yuk ? (
          <div className="text-sm text-slate-400">Yükleniyor…</div>
        ) : !durum?.etkin ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 text-center">
            <div className="text-3xl mb-2">📧</div>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">Bu domain için e-posta henüz etkin değil.</p>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">Etkinleştirince MX/SPF/DKIM/DMARC kayıtları otomatik olarak DNS'e eklenir.</p>
            <button onClick={etkinlestir} disabled={isleniyor}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {isleniyor ? 'Etkinleştiriliyor…' : 'E-postayı Etkinleştir'}
            </button>
          </div>
        ) : (
          <>
            <form onSubmit={ekle} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Yeni kutu ekle</h3>
              <div className="flex items-center gap-2">
                <input value={localPart} onChange={e => setLocalPart(e.target.value)} required placeholder="bilgi"
                  className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                <input value={parola} onChange={e => setParola(e.target.value)} type="password" placeholder="parola (boşsa üretilir)"
                  className="w-56 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={isleniyor || !localPart} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isleniyor ? 'Ekleniyor…' : 'Ekle'}
                </button>
              </div>
            </form>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Kutular</h3>
              {liste.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-slate-500 dark:text-slate-400">Henüz kutu yok.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {liste.map(k => (
                    <li key={k.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">{k.email}</span>
                        {k.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">askıda</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => parolaSifirla(k)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">Parola sıfırla</button>
                        <button onClick={() => sil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Sil</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">Yönlendirmeler (Forwarder) &amp; Catch-All</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                Gelen postayı bir kutu oluşturmadan başka adres(ler)e yönlendirir. "Bu domaine gelen tüm postayı yönlendir" seçilirse, tanımlı kutusu olmayan her adrese gelen mail bu hedefe gider (catch-all).
              </p>
              <form onSubmit={aliasEkle} className="mb-4 space-y-2">
                <div className="flex items-center gap-2">
                  {aliasCatchAll ? (
                    <span className="flex-1 px-3 py-2 border border-dashed border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-500 dark:text-slate-400 font-mono">*@{domain?.alan_adi}</span>
                  ) : (
                    <>
                      <input value={aliasKaynak} onChange={e => setAliasKaynak(e.target.value)} required={!aliasCatchAll} placeholder="destek"
                        className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                      <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                    </>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                  <input type="checkbox" checked={aliasCatchAll} onChange={e => setAliasCatchAll(e.target.checked)} />
                  Bu domaine gelen tüm postayı yönlendir (catch-all)
                </label>
                <div className="flex items-center gap-2">
                  <input value={aliasHedef} onChange={e => setAliasHedef(e.target.value)} required placeholder="hedef1@ornek.com, hedef2@ornek.com"
                    className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  <button disabled={aliasIsleniyor || !aliasHedef || (!aliasCatchAll && !aliasKaynak)}
                    className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    {aliasIsleniyor ? 'Ekleniyor…' : 'Ekle'}
                  </button>
                </div>
              </form>

              {aliasListe.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-sm text-slate-500 dark:text-slate-400">Henüz yönlendirme yok.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {aliasListe.map(a => (
                    <li key={a.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">
                          {a.catch_all ? `*@${domain?.alan_adi}` : a.source}
                        </span>
                        <span className="mx-1.5 text-slate-400">→</span>
                        <span className="text-sm font-mono text-slate-600 dark:text-slate-400">{a.destination}</span>
                        {a.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">askıda</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => aliasDurumDegistir(a)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {a.status === 'active' ? 'Askıya al' : 'Etkinleştir'}
                        </button>
                        <button onClick={() => aliasSil(a)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Sil</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </>
        )}

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Aboneliğe dön</Link></div>
      </div>
    </div>
  )
}
