import { useMemo, useState } from "react";
import { api, ApiError, Portfolio } from "../api";
import { fmtCents, fmt$ } from "../format";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  instrumentId: string;
  lastClose: number;
  portfolio: Portfolio | null;
  onChanged: () => void;
};

export default function TradePanel({ roomId, instrumentId, lastClose, portfolio, onChanged }: Props) {
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
    if (side === "buy") setRaw(String(Math.floor((cash / 100) * f)));
    else setRaw((heldShares * f).toFixed(1));
  }

  async function submit() {
    setBusy(true);
    try {
      const body = side === "buy"
        ? { instrument_id: instrumentId, side, amount_cents: Math.round(value * 100) }
        : { instrument_id: instrumentId, side, shares: value };
      await api.post(`/api/rooms/${roomId}/orders`, body);
      toast("已下单，开盘成交（已冻结）");
      setRaw("");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "下单失败");
    } finally {
      setBusy(false);
    }
  }

  async function cancel(orderID: number) {
    try {
      await api.del(`/api/rooms/${roomId}/orders/${orderID}`);
      toast("已撤单，资金解冻");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "撤单失败");
    }
  }

  return (
    <div className="card trade">
      <div className="tabs">
        <button className={`buy-tab ${side === "buy" ? "on" : ""}`}
          onClick={() => { setSide("buy"); setRaw(""); }}>买入</button>
        <button className={`sell-tab ${side === "sell" ? "on" : ""}`}
          onClick={() => { setSide("sell"); setRaw(""); }}>卖出</button>
      </div>
      <div className="field-label">{side === "buy" ? "买入金额" : "卖出股数"}</div>
      <div className="amt">
        {side === "buy" && <span>$</span>}
        <input inputMode="decimal" placeholder="0" value={raw}
          onChange={e => setRaw(e.target.value)} />
      </div>
      <div className="chips">
        {[["25%", 0.25], ["50%", 0.5], ["75%", 0.75], ["全部", 1]].map(([label, f]) => (
          <button key={label as string} onClick={() => pickFraction(f as number)}>{label as string}</button>
        ))}
      </div>
      <div className="est"><span>可用</span>
        <b className="num">{side === "buy" ? fmtCents(cash) : `${heldShares.toFixed(1)} 股`}</b></div>
      <div className="est">
        <span>{side === "buy" ? "预估股数（按今日收盘参考）" : "预估金额（按今日收盘参考）"}</span>
        <b className="num">
          {value > 0 && lastClose > 0
            ? side === "buy" ? `≈ ${(value / lastClose).toFixed(1)} 股` : `≈ ${fmt$(value * lastClose)}`
            : "—"}
        </b>
      </div>
      <p className="note">订单将在<b>下一个历史交易日的开盘价</b>成交，成交价此刻未知。下单即冻结，开盘前可撤单。</p>
      <button className={`submit ${side === "sell" ? "sell" : ""}`}
        disabled={busy || value <= 0 || overLimit} onClick={submit}>
        {side === "buy" ? "下单买入" : "下单卖出"}
      </button>
      {pending.length > 0 && (
        <div className="pending-list">
          <div className="field-label">待成交</div>
          {pending.map(o => (
            <div key={o.id} className="pending-item">
              <span className="num">
                {o.side === "buy" ? `买入 ${fmtCents(o.amount_cents)}` : `卖出 ${o.shares} 股`} · 第 {o.exec_day} 日开盘
              </span>
              <button className="cancel" onClick={() => cancel(o.id)}>撤单</button>
            </div>
          ))}
        </div>
      )}
      {node}
    </div>
  );
}
