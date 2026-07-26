import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, ApiError, RevealData } from "../api";
import { fmt$, fmtCents, fmtPct } from "../format";

export default function Reveal() {
  const { roomId } = useParams<{ roomId: string }>();
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

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (notReady) {
    return (
      <div className="wrap lobby">
        <h1>尚未揭晓</h1>
        <p className="sub">时间轴还没走完——回到房间继续操作，终点见分晓。</p>
        <Link className="link-btn" to={`/rooms/${roomId}`}>← 返回房间</Link>
      </div>
    );
  }
  if (!data) return null;

  const aliasOf = (id: string) => data.instruments.find(i => i.id === id)?.alias ?? id;
  const hasRealNames = data.instruments.some(i => i.real_name !== "");

  return (
    <div className="wrap lobby">
      <h1>揭晓时刻</h1>
      <p className="sub">盲盒打开：这段历史的真身，和每个人的全部操作。</p>

      <div className="card">
        <h2>最终排行</h2>
        {data.leaderboard.map((row, i) => (
          <div key={row.username} className="lb-row">
            <span className="rank num">{i === 0 ? "🏆" : i + 1}</span>
            <span className="who">{row.username}{row.late_join && <small>晚入场</small>}</span>
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>身份揭晓</h2>
        {!hasRealNames && (
          <p className="rc-meta">本局使用合成剧本，标的没有真实历史身份；真实剧本（如 2000 年互联网泡沫）会在这里揭晓每只股票的真名与真实日期区间。</p>
        )}
        <table className="reveal-table">
          <thead><tr><th>化名</th><th>真实身份</th></tr></thead>
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
        <h2>全场成交回放</h2>
        <table className="reveal-table">
          <thead>
            <tr><th>日</th><th>玩家</th><th>方向</th><th>标的</th>
              <th className="num">成交价</th><th className="num">股数</th><th className="num">金额</th></tr>
          </thead>
          <tbody>
            {data.trades.map((t, i) => (
              <tr key={i}>
                <td className="num">{t.day}</td>
                <td>{t.username}</td>
                <td className={`delta ${t.side === "buy" ? "up" : "down"}`}>{t.side === "buy" ? "买入" : "卖出"}</td>
                <td>{aliasOf(t.instrument_id)}</td>
                <td className="num">{fmt$(t.price)}</td>
                <td className="num">{t.shares.toFixed(1)}</td>
                <td className="num">{fmtCents(t.amount_cents)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
