import { useState } from "react";
import { ApiError, LoanAction, Portfolio, postLoan } from "../api";
import { fmtCents } from "../format";
import { useT } from "../i18n";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  portfolio: Portfolio | null;
  onChanged: () => void;
};

const fmtRate = (bp: number) => `${(bp / 100).toFixed(2)}%`;

export default function LoanPanel({ roomId, portfolio, onChanged }: Props) {
  const { t } = useT();
  const { toast, node } = useToast();
  const [action, setAction] = useState<LoanAction>("borrow");
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);

  const cash = portfolio?.cash_cents ?? 0;
  const debt = portfolio?.debt_cents ?? 0;
  const maxDebt = portfolio?.max_debt_cents ?? 0;
  const rateBp = portfolio?.interest_rate_annual_bp ?? 0;
  const bankrupt = portfolio?.bankrupt ?? false;

  // borrow: % of remaining cap headroom; repay: % of min(cash, debt)
  const headroom = Math.max(0, maxDebt - debt);
  const repayable = Math.min(cash, debt);
  const maxCents = action === "borrow" ? headroom : repayable;

  const value = parseFloat(raw) || 0;
  const overLimit = value * 100 > maxCents + 1e-9;
  const capPct = maxDebt > 0 ? debt / maxDebt : 0;

  function pickFraction(f: number) {
    setRaw(String(Math.floor((maxCents / 100) * f)));
  }

  async function submit() {
    setBusy(true);
    try {
      await postLoan(roomId, action, Math.round(value * 100));
      toast(t("loan.done"));
      setRaw("");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("loan.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card trade loan">
      <h2>{t("loan.title")}</h2>
      <div className="est"><span>{t("loan.debt")}</span><b className="num">{fmtCents(debt)}</b></div>
      <div className="est"><span>{t("loan.rate")}</span><b className="num">{fmtRate(rateBp)}</b></div>
      <div className="est"><span>{t("loan.capUsage", { used: fmtCents(debt), cap: fmtCents(maxDebt) })}</span>
        <b className="num">{(capPct * 100).toFixed(0)}%</b></div>
      <div className="progress cap"><i style={{ width: `${Math.min(100, capPct * 100)}%` }} /></div>
      {capPct >= 0.8 && !bankrupt && <p className="note cap-warn">{t("loan.capWarning")}</p>}

      <div className="tabs">
        <button className={`buy-tab ${action === "borrow" ? "on" : ""}`}
          onClick={() => { setAction("borrow"); setRaw(""); }}>{t("loan.borrow")}</button>
        <button className={`sell-tab ${action === "repay" ? "on" : ""}`}
          onClick={() => { setAction("repay"); setRaw(""); }}>{t("loan.repay")}</button>
      </div>
      <div className="field-label">{t("loan.amount")}</div>
      <div className="amt">
        <span>$</span>
        <input inputMode="decimal" placeholder="0" value={raw} disabled={bankrupt}
          onChange={e => setRaw(e.target.value)} />
      </div>
      <div className="chips">
        {[["25%", 0.25], ["50%", 0.5], ["75%", 0.75], [t("trade.all"), 1]].map(([label, f]) => (
          <button key={label as string} disabled={bankrupt}
            onClick={() => pickFraction(f as number)}>{label as string}</button>
        ))}
      </div>
      <button className={`submit ${action === "repay" ? "sell" : ""}`}
        disabled={busy || bankrupt || value <= 0 || overLimit} onClick={submit}>
        {action === "borrow" ? t("loan.placeBorrow") : t("loan.placeRepay")}
      </button>
      {node}
    </div>
  );
}
