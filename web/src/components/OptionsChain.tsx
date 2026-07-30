import { useEffect, useMemo, useState } from "react";
import { ApiError, fetchOptions, OptionContract, Portfolio, postOptionOrder } from "../api";
import { fmt$, fmtCents } from "../format";
import { useT } from "../i18n";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  instrumentId: string;
  alias: string;
  lastClose: number;        // current underlying close: ITM/OTM reference
  currentDay: number;
  portfolio: Portfolio | null;
  onChanged: () => void;
  disabled?: boolean;       // bankrupt or room ended
  note?: string;            // why trading is locked (bankrupt / ended)
};

export default function OptionsChain({ roomId, instrumentId, alias, lastClose, currentDay, portfolio, onChanged, disabled, note }: Props) {
  const { t } = useT();
  const { toast, node } = useToast();
  const [chain, setChain] = useState<OptionContract[]>([]);
  const [expiry, setExpiry] = useState<number | null>(null);
  const [sel, setSel] = useState<OptionContract | null>(null);
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    setSel(null);
    setExpiry(null);
    setRaw("");
    fetchOptions(roomId, instrumentId)
      .then(cs => { if (live) setChain(cs); })
      .catch(() => { if (live) setChain([]); });
    return () => { live = false; };
  }, [roomId, instrumentId, currentDay]);

  const expiries = useMemo(
    () => [...new Set(chain.map(c => c.expiry_day))].sort((a, b) => a - b), [chain]);
  const activeExpiry = expiry ?? expiries[0] ?? null;
  const rows = useMemo(() => {
    const byStrike = new Map<number, { call?: OptionContract; put?: OptionContract }>();
    for (const c of chain) {
      if (c.expiry_day !== activeExpiry) continue;
      const r = byStrike.get(c.strike) ?? {};
      r[c.kind] = c;
      byStrike.set(c.strike, r);
    }
    return [...byStrike.entries()].sort((a, b) => a[0] - b[0]);
  }, [chain, activeExpiry]);

  const cash = portfolio?.cash_cents ?? 0;
  const contracts = parseInt(raw, 10) || 0;
  // Server prices the premium as round(price * contracts * 100) cents.
  const maxByCash = sel && sel.price > 0 ? Math.floor(cash / (sel.price * 100)) : 0;
  const overLimit = contracts > maxByCash;
  const premiumCents = sel ? Math.round(sel.price * contracts * 100) : 0;

  function select(c: OptionContract) {
    setSel(sel?.option_id === c.option_id ? null : c);
    setRaw("");
  }

  async function buy() {
    if (!sel) return;
    setBusy(true);
    try {
      await postOptionOrder(roomId, sel.option_id, "buy", contracts);
      toast(t("option.bought", { n: contracts }));
      setSel(null);
      setRaw("");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("option.orderFailed"));
    } finally {
      setBusy(false);
    }
  }

  const selDesc = sel
    ? t("option.positionDesc", {
        alias, kind: t(sel.kind === "call" ? "option.kind.call" : "option.kind.put"),
        strike: fmt$(sel.strike), day: sel.expiry_day, left: sel.expiry_day - currentDay,
      })
    : "";

  return (
    <div className="section options-chain">
      <h2>{t("option.chainTitle")}</h2>
      {chain.length === 0 ? (
        <div className="chain-empty">{t("option.empty")}</div>
      ) : (
        <>
          <div className="expiry-pills">
            {expiries.map(d => (
              <button key={d} className={d === activeExpiry ? "on" : ""}
                onClick={() => { setExpiry(d); setSel(null); setRaw(""); }}>
                {t("option.expiryPill", { day: d, left: d - currentDay })}
              </button>
            ))}
          </div>
          <table className="chain-table">
            <thead>
              <tr><th>{t("option.thCalls")}</th><th>{t("option.thStrike")}</th><th>{t("option.thPuts")}</th></tr>
            </thead>
            <tbody>
              {rows.map(([strike, r]) => (
                <tr key={strike}>
                  <td>
                    {r.call && (
                      <button
                        className={`chain-px call ${strike < lastClose ? "itm" : ""} ${sel?.option_id === r.call.option_id ? "on" : ""}`}
                        disabled={disabled} onClick={() => select(r.call!)}>
                        {fmt$(r.call.price)}
                      </button>
                    )}
                  </td>
                  <td className="strike num">{fmt$(strike)}</td>
                  <td>
                    {r.put && (
                      <button
                        className={`chain-px put ${strike > lastClose ? "itm" : ""} ${sel?.option_id === r.put.option_id ? "on" : ""}`}
                        disabled={disabled} onClick={() => select(r.put!)}>
                        {fmt$(r.put.price)}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {note && <p className="note">{note}</p>}
          {sel && (
            <div className="opt-buy">
              <div className="field-label">{selDesc}</div>
              <div className="field-label">{t("option.contracts")}</div>
              <div className="amt">
                <input inputMode="numeric" placeholder="0" value={raw} disabled={disabled}
                  onChange={e => setRaw(e.target.value)} />
              </div>
              <div className="chips">
                {[1, 5, 10].map(n => (
                  <button key={n} onClick={() => setRaw(String(n))}>{n}</button>
                ))}
                <button onClick={() => setRaw(String(maxByCash))}>{t("trade.all")}</button>
              </div>
              <div className="est"><span>{t("trade.available")}</span><b className="num">{fmtCents(cash)}</b></div>
              <div className="est"><span>{t("option.estPremium")}</span>
                <b className="num">{contracts > 0 ? `≈ ${fmtCents(premiumCents)}` : "—"}</b></div>
              <button className="submit"
                disabled={busy || disabled || contracts <= 0 || overLimit} onClick={buy}>
                {t("option.buyToOpen")}
              </button>
            </div>
          )}
        </>
      )}
      {node}
    </div>
  );
}
