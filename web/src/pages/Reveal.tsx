import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, ApiError, RevealData } from "../api";
import { fmt$, fmtCents, fmtPct } from "../format";
import { LangSwitch, pickL, useT } from "../i18n";
import DocsLink from "../components/DocsLink";

export default function Reveal() {
  const { roomId } = useParams<{ roomId: string }>();
  const { t, lang } = useT();
  const [data, setData] = useState<RevealData | null>(null);
  const [notReady, setNotReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<RevealData>(`/api/rooms/${roomId}/reveal`)
      .then(setData)
      .catch(e => {
        if (e instanceof ApiError && e.status === 409) setNotReady(true);
        else setError(e instanceof Error ? e.message : String(e));
      });
  }, [roomId]);

  const langRow = (
    <div className="page-tools" style={{ justifyContent: "flex-end" }}><DocsLink /><LangSwitch /></div>
  );

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (notReady) {
    return (
      <div className="wrap lobby">
        {langRow}
        <h1>{t("reveal.notReadyTitle")}</h1>
        <p className="sub">{t("reveal.notReadySub")}</p>
        <Link className="link-btn" to={`/rooms/${roomId}`}>{t("common.backToRoom")}</Link>
      </div>
    );
  }
  if (!data) return null;

  const aliasOf = (id: string) => data.instruments.find(i => i.id === id)?.alias ?? id;
  const hasRealNames = data.instruments.some(i => i.real_name !== "");

  return (
    <div className="wrap lobby">
      {langRow}
      <h1>{t("reveal.title")}</h1>
      <p className="sub">{t("reveal.sub")}</p>

      <div className="card">
        <h2>{t("reveal.finalBoard")}</h2>
        {data.leaderboard.map((row, i) => (
          <div key={row.username} className="lb-row">
            <span className="rank num">{i === 0 ? "🏆" : i + 1}</span>
            <span className="who">{pickL(lang, row.username, row.username_en)}
              {row.is_agent && <small className="agent-badge">{t("common.agent")}</small>}
              {row.late_join && <small>{t("reveal.lateJoin")}</small>}
            </span>
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>{t("reveal.identities")}</h2>
        {data.real_period && (
          <p className="rc-meta">{t("reveal.realPeriod")}<b className="num">{data.real_period}</b></p>
        )}
        {!hasRealNames && (
          <p className="rc-meta">{t("reveal.syntheticNote")}</p>
        )}
        <table className="reveal-table">
          <thead><tr><th>{t("reveal.alias")}</th><th>{t("reveal.realName")}</th></tr></thead>
          <tbody>
            {data.instruments.map(inst => (
              <tr key={inst.id}>
                <td>{inst.alias} <span className="num" style={{ color: "var(--ink3)" }}>{inst.id}</span></td>
                <td>{inst.real_name || "——"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h2>{t("reveal.tradesReplay")}</h2>
        <table className="reveal-table">
          <thead>
            <tr><th>{t("reveal.thDay")}</th><th>{t("reveal.thPlayer")}</th><th>{t("reveal.thSide")}</th><th>{t("reveal.thTicker")}</th>
              <th className="num">{t("reveal.thPrice")}</th><th className="num">{t("reveal.thShares")}</th><th className="num">{t("reveal.thAmount")}</th></tr>
          </thead>
          <tbody>
            {data.trades.map((tr, i) => (
              <tr key={i}>
                <td className="num">{tr.day}</td>
                <td>{pickL(lang, tr.username, tr.username_en)}{tr.is_agent && <small className="agent-badge">{t("common.agent")}</small>}</td>
                <td className={`delta ${tr.side === "buy" ? "up" : "down"}`}>{tr.side === "buy" ? t("side.Buy") : t("side.Sell")}</td>
                <td>{aliasOf(tr.instrument_id)}</td>
                <td className="num">{fmt$(tr.price)}</td>
                <td className="num">{tr.shares.toFixed(1)}</td>
                <td className="num">{fmtCents(tr.amount_cents)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
