import { useNavigate, useParams } from "react-router-dom";
import { MEDIA_NAMES, NewsItem } from "../api";
import { fmt$, fmtPct, prettifyHeadline } from "../format";
import { useRoomData, useSimClock } from "../roomData";
import { useIncrementalFeed } from "../useIncrementalFeed";
import { useState } from "react";
import HeroChart from "../components/HeroChart";
import TradePanel from "../components/TradePanel";

export default function Stock() {
  const { roomId, instrumentId } = useParams<{ roomId: string; instrumentId: string }>();
  const navigate = useNavigate();
  const { state, portfolio, series, reload } = useRoomData(roomId!);
  const clock = useSimClock(state?.room);
  const { items: newsItems } = useIncrementalFeed<NewsItem>(
    after => `/api/rooms/${roomId}/news?after=${after}`, 30_000, roomId!);
  const [openNews, setOpenNews] = useState<number | null>(null);

  if (!state) return null;
  const inst = state.instruments.find(i => i.id === instrumentId);
  const closes = series[instrumentId!] ?? [];
  if (!inst) return <div className="wrap err-banner">未知标的</div>;

  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;
  const last = closes[closes.length - 1] ?? 0;
  const prev = closes[closes.length - 2] ?? last;
  const q3m = closes.slice(-63);
  const held = portfolio?.positions.find(p => p.instrument_id === instrumentId)?.shares ?? 0;
  const relatedNews = newsItems
    .filter(n => n.headline.includes(instrumentId!) || n.headline.includes(inst.alias))
    .sort((a, b) => b.id - a.id)
    .slice(0, 5);

  return (
    <div className="wrap">
      <button className="back-btn" onClick={() => navigate(`/rooms/${roomId}`)}>← 返回房间</button>
      <div className="stock-grid">
        <div>
          {closes.length > 1 && (
            <HeroChart
              label={`${inst.alias} · ${inst.desc} · ${inst.id}`}
              series={closes} startDay={0} formatValue={fmt$}
            />
          )}
          <div className="stat-strip num">
            <div><span className="k">今日收盘</span>{fmt$(last)}</div>
            <div><span className="k">昨收</span>{fmt$(prev)}</div>
            <div><span className="k">3月最高</span>{q3m.length ? fmt$(Math.max(...q3m)) : "—"}</div>
            <div><span className="k">3月最低</span>{q3m.length ? fmt$(Math.min(...q3m)) : "—"}</div>
            <div><span className="k">开局至今</span>
              <span className={`delta ${last >= (closes[0] ?? last) ? "up" : "down"}`}>
                {closes[0] ? fmtPct(last / closes[0] - 1) : "—"}
              </span>
            </div>
            <div><span className="k">我的持仓</span>{held > 0 ? `${held.toFixed(1)} 股` : "—"}</div>
          </div>

          <div className="section">
            <h2>档案</h2>
            <div className="profile-grid">
              <div className="profile-item"><div className="pk">简介</div><p>{inst.desc || "——"}</p></div>
              {inst.profile && (
                <>
                  <div className="profile-item"><div className="pk">主营业务</div><p>{inst.profile.business}</p></div>
                  <div className="profile-item"><div className="pk bull">多头故事</div><p>{inst.profile.bull}</p></div>
                  <div className="profile-item"><div className="pk bear">风险提示</div><p>{inst.profile.bear}</p></div>
                </>
              )}
            </div>
          </div>

          <div className="section">
            <h2>相关新闻</h2>
            {relatedNews.length === 0 && <div className="feed-item">暂无相关新闻</div>}
            {relatedNews.map(n => (
              <div key={n.id}
                className={`feed-item ${n.body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
                onClick={n.body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
                <div className="fi-meta">{MEDIA_NAMES[n.media_id] ?? n.media_id} · <span className="num">第 {n.day} 日</span></div>
                <div className={n.body ? "fi-title" : ""}>{prettifyHeadline(n.headline, aliasOf)}</div>
                {n.body && <div className="fi-body">{n.body}</div>}
              </div>
            ))}
          </div>
        </div>

        <TradePanel roomId={roomId!} instrumentId={instrumentId!} lastClose={last}
          portfolio={portfolio} onChanged={reload}
          afterHours={clock?.phase === "closed" || clock?.phase === "weekend"} />
      </div>
    </div>
  );
}
