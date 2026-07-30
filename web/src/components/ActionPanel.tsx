import { useState } from "react";
import {
  ApiError, HypeDirection, HypeTier, IntelResponse, Portfolio, postHype, postIntel,
} from "../api";
import { fmtCents } from "../format";
import { useT } from "../i18n";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  instrumentId: string;
  alias: string;
  portfolio: Portfolio | null;
  onChanged: () => void;
  disabled?: boolean;   // bankrupt or room ended
  note?: string;        // why actions are locked (bankrupt / ended)
};

/* Mirrors the server-side tiers: fee / shock size / regulator catch risk. */
const TIERS: { tier: HypeTier; feeCents: number; shockPct: number; riskPct: number }[] = [
  { tier: 1, feeCents: 500_000, shockPct: 1.5, riskPct: 10 },
  { tier: 2, feeCents: 1_500_000, shockPct: 3, riskPct: 20 },
  { tier: 3, feeCents: 4_000_000, shockPct: 5, riskPct: 30 },
];

const INTEL_FEE_CENTS = 300_000;

export default function ActionPanel({ roomId, instrumentId, alias, portfolio, onChanged, disabled, note }: Props) {
  const { t } = useT();
  const { toast, node } = useToast();
  const [direction, setDirection] = useState<HypeDirection>("up");
  const [tier, setTier] = useState<HypeTier>(1);
  const [busy, setBusy] = useState(false);
  const [caughtFine, setCaughtFine] = useState<number | null>(null);
  const [tip, setTip] = useState<IntelResponse | null>(null);

  const sel = TIERS.find(x => x.tier === tier)!;

  async function hype() {
    setBusy(true);
    try {
      const res = await postHype(roomId, instrumentId, direction, tier);
      if (res.caught) {
        // Alarming, persistent banner: fined AND publicly exposed.
        setCaughtFine(res.fine_cents);
      } else {
        setCaughtFine(null);
        toast(t("actions.hypeDone"));
      }
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("actions.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function intel() {
    setBusy(true);
    try {
      const res = await postIntel(roomId, instrumentId);
      setTip(res);
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("actions.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section actions">
      <h2>{t("actions.title")}</h2>

      <div className="field-label">{t("actions.hypeTitle")}</div>
      <div className="seg">
        <button className={direction === "up" ? "on" : ""} disabled={disabled}
          onClick={() => setDirection("up")}>{t("actions.direction.up")}</button>
        <button className={direction === "down" ? "on" : ""} disabled={disabled}
          onClick={() => setDirection("down")}>{t("actions.direction.down")}</button>
      </div>
      <div className="tier-row">
        {TIERS.map(x => (
          <button key={x.tier} className={x.tier === tier ? "on" : ""} disabled={disabled}
            onClick={() => setTier(x.tier)}>
            <b>{t("actions.tier", { n: x.tier })}</b>
            <span>{t("actions.tierMeta", { fee: fmtCents(x.feeCents), shock: x.shockPct, risk: x.riskPct })}</span>
          </button>
        ))}
      </div>
      <p className="note">{t("actions.hypeNote")}</p>
      {note && <p className="note">{note}</p>}
      <div className="est"><span>{t("trade.available")}</span>
        <b className="num">{fmtCents(portfolio?.cash_cents ?? 0)}</b></div>
      {caughtFine !== null && (
        <div className="caught-banner">
          {t("actions.caughtTitle")}
          <div className="cb-sub">{t("actions.caughtBody", { amount: fmtCents(caughtFine), alias })}</div>
        </div>
      )}
      <button className={`submit ${direction === "down" ? "sell" : ""}`}
        disabled={busy || disabled} onClick={hype}>
        {t("actions.hypeSubmit", { fee: fmtCents(sel.feeCents) })}
      </button>

      <div className="intel-block">
        <div className="field-label">{t("actions.intelTitle")}</div>
        <p className="note">{t("actions.intelNote")}</p>
        {tip && (
          <div className="tip-panel">
            <div className="field-label">{t("actions.tipHeading")}</div>
            <div className={`tip-outlook delta ${tip.outlook === "quiet" ? "" : tip.outlook}`}>
              {t(`actions.tip.${tip.outlook}`)}
              {tip.strength && <span className="tip-strength"> · {t(`actions.strength.${tip.strength}`)}</span>}
            </div>
            <p className="note">{t("actions.tipCaveat")}</p>
          </div>
        )}
        <button className="submit" disabled={busy || disabled} onClick={intel}>
          {t("actions.intelBuy", { fee: fmtCents(INTEL_FEE_CENTS) })}
        </button>
      </div>
      {node}
    </div>
  );
}
