import { useState } from "react";
import { ApiError, OptionPosition, postOptionOrder } from "../api";
import { fmt$, fmtCents, fmtPct, fmtSignedCents } from "../format";
import { useT } from "../i18n";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  positions: OptionPosition[];
  currentDay: number;
  aliasOf: (instrumentID: string) => string;
  onChanged: () => void;
  disabled?: boolean;   // bankrupt or room ended
};

/** Held option contracts with a sell-to-close action; rows for one section. */
export default function OptionPositions({ roomId, positions, currentDay, aliasOf, onChanged, disabled }: Props) {
  const { t } = useT();
  const { toast, node } = useToast();
  const [raw, setRaw] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState<number | null>(null);

  async function sell(p: OptionPosition) {
    const n = Math.min(p.contracts, parseInt(raw[p.option_id] ?? "", 10) || p.contracts);
    if (n <= 0) return;
    setBusy(p.option_id);
    try {
      await postOptionOrder(roomId, p.option_id, "sell", n);
      toast(t("option.sold"));
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("option.orderFailed"));
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      {positions.map(p => (
        <div key={p.option_id} className="opt-row">
          <div className="op-name">
            {t("option.positionDesc", {
              alias: aliasOf(p.instrument_id),
              kind: t(p.kind === "call" ? "option.kind.call" : "option.kind.put"),
              strike: fmt$(p.strike), day: p.expiry_day, left: p.expiry_day - currentDay,
            })}
          </div>
          <div className="op-sub num">
            {t("option.positionSub", { n: p.contracts, value: fmtCents(p.value_cents) })}
          </div>
          <div className={`op-sub num delta ${p.pnl_cents >= 0 ? "up" : "down"}`}>
            {t("option.positionPnl", {
              avg: fmt$(p.avg_cost), amount: fmtSignedCents(p.pnl_cents), pct: fmtPct(p.pnl_pct),
            })}
          </div>
          <div className="op-sell">
            <input inputMode="numeric" placeholder={String(p.contracts)}
              value={raw[p.option_id] ?? ""} disabled={disabled}
              onChange={e => setRaw({ ...raw, [p.option_id]: e.target.value })} />
            <button disabled={disabled || busy === p.option_id} onClick={() => sell(p)}>
              {t("option.sellClose")}
            </button>
          </div>
        </div>
      ))}
      {node}
    </>
  );
}
