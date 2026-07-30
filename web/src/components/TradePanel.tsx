import { useMemo, useState } from "react";
import { api, ApiError, Portfolio } from "../api";
import { fmtCents, fmt$ } from "../format";
import { useT } from "../i18n";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  instrumentId: string;
  lastClose: number;
  portfolio: Portfolio | null;
  onChanged: () => void;
  afterHours?: boolean;
  disabled?: boolean;   // bankrupt: trading locked
};

export default function TradePanel({ roomId, instrumentId, lastClose, portfolio, onChanged, afterHours, disabled }: Props) {
  const { t } = useT();
  const { toast, node } = useToast();
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);

  const cash = portfolio?.cash_cents ?? 0;
  const heldShares = useMemo(
    () => portfolio?.positions.find(p => p.instrument_id === instrumentId)?.shares ?? 0,
    [portfolio, instrumentId]);
  const pending = (portfolio?.pending ?? []).filter(o => o.instrument_id === instrumentId);

  const value = parseFloat(raw) || 0;
  const maxValue = side === "buy" ? cash / 100 : heldShares;
  const overLimit = value > maxValue + 1e-9;

  function pickFraction(f: number) {
    if (side === "buy") {
      setRaw(String(Math.floor((cash / 100) * f)));
    } else {
      // Round DOWN to 0.1 share, and never exceed the actual holding.
      const target = Math.min(heldShares, Math.floor(heldShares * f * 10) / 10);
      setRaw(target.toFixed(1));
    }
  }

  async function submit() {
    setBusy(true);
    try {
      const body = side === "buy"
        ? { instrument_id: instrumentId, side, amount_cents: Math.round(value * 100) }
        : { instrument_id: instrumentId, side, shares: value };
      await api.post(`/api/rooms/${roomId}/orders`, body);
      toast(t("trade.ordered"));
      setRaw("");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("trade.orderFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function cancel(orderID: number) {
    try {
      await api.del(`/api/rooms/${roomId}/orders/${orderID}`);
      toast(t("trade.cancelled"));
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("trade.cancelFailed"));
    }
  }

  return (
    <div className="card trade">
      <div className="tabs">
        <button className={`buy-tab ${side === "buy" ? "on" : ""}`}
          onClick={() => { setSide("buy"); setRaw(""); }}>{t("side.Buy")}</button>
        <button className={`sell-tab ${side === "sell" ? "on" : ""}`}
          onClick={() => { setSide("sell"); setRaw(""); }}>{t("side.Sell")}</button>
      </div>
      <div className="field-label">{side === "buy" ? t("trade.buyAmount") : t("trade.sellShares")}</div>
      <div className="amt">
        {side === "buy" && <span>$</span>}
        <input inputMode="decimal" placeholder="0" value={raw} disabled={disabled}
          onChange={e => setRaw(e.target.value)} />
      </div>
      <div className="chips">
        {[["25%", 0.25], ["50%", 0.5], ["75%", 0.75], [t("trade.all"), 1]].map(([label, f]) => (
          <button key={label as string} onClick={() => pickFraction(f as number)}>{label as string}</button>
        ))}
      </div>
      <div className="est"><span>{t("trade.available")}</span>
        <b className="num">{side === "buy" ? fmtCents(cash) : t("unit.shares", { n: heldShares.toFixed(1) })}</b></div>
      <div className="est">
        <span>{side === "buy" ? t("trade.estShares") : t("trade.estAmount")}</span>
        <b className="num">
          {value > 0 && lastClose > 0
            ? side === "buy" ? `≈ ${t("unit.shares", { n: (value / lastClose).toFixed(1) })}` : `≈ ${fmt$(value * lastClose)}`
            : "—"}
        </b>
      </div>
      <p className="note">{t("trade.noteA")}<b>{t("trade.noteB")}</b>{t("trade.noteC")}</p>
      {afterHours && <p className="note">{t("trade.afterHours")}</p>}
      {disabled && <p className="note">{t("trade.bankruptNote")}</p>}
      <button className={`submit ${side === "sell" ? "sell" : ""}`}
        disabled={busy || disabled || value <= 0 || overLimit} onClick={submit}>
        {side === "buy" ? t("trade.placeBuy") : t("trade.placeSell")}
      </button>
      {pending.length > 0 && (
        <div className="pending-list">
          <div className="field-label">{t("trade.pending")}</div>
          {pending.map(o => (
            <div key={o.id} className="pending-item">
              <span className="num">
                {o.side === "buy"
                  ? t("trade.pendingBuy", { amount: fmtCents(o.amount_cents) })
                  : t("trade.pendingSell", { shares: o.shares.toFixed(1) })}
                {" · "}{t("trade.pendingExec", { day: o.exec_day })}
              </span>
              <button className="cancel" onClick={() => cancel(o.id)}>{t("trade.cancel")}</button>
            </div>
          ))}
        </div>
      )}
      {node}
    </div>
  );
}
