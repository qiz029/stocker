import { useState } from "react";
import { EventItem, MEDIA_NAMES, NewsItem, RoomState } from "../api";
import { fmtCents, fmtPct, prettifyHeadline } from "../format";
import { useIncrementalFeed } from "../useIncrementalFeed";
import { useUser } from "../App";
import Chat from "./Chat";

type Props = { roomId: string; state: RoomState; aliasOf: (id: string) => string };

export default function RightRail({ roomId, state, aliasOf }: Props) {
  const user = useUser();
  const [newsShown, setNewsShown] = useState(8);
  const [openNews, setOpenNews] = useState<number | null>(null);
  const { items: newsItems } = useIncrementalFeed<NewsItem>(
    after => `/api/rooms/${roomId}/news?after=${after}`, 30_000, roomId);
  const { items: eventsItems } = useIncrementalFeed<EventItem>(
    after => `/api/rooms/${roomId}/events?after=${after}`, 30_000, roomId);

  const news = [...newsItems].sort((a, b) => b.id - a.id).slice(0, newsShown);
  const events = [...eventsItems].sort((a, b) => b.id - a.id).slice(0, 6);

  return (
    <div>
      <div className="card">
        <h2>排行榜</h2>
        {state.leaderboard.map((row, i) => (
          <div key={row.username} className={`lb-row ${row.username === user.username ? "me" : ""}`}>
            <span className="rank num">{i + 1}</span>
            <span className="who">{row.username}{row.late_join && <small>晚入场</small>}</span>
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <Chat roomId={roomId} />

      <div className="card">
        <h2>房间动态</h2>
        {events.length === 0 && <div className="feed-item">暂无动态</div>}
        {events.map(ev => (
          <div key={ev.id} className={`feed-item whale ${ev.payload.side === "sell" ? "sell" : ""}`}>
            <div className="fi-meta num">第 {ev.day} 个交易日</div>
            <span className="whale-txt">
              🐳 有玩家大额{ev.payload.side === "buy" ? "买入" : "卖出"} {aliasOf(ev.payload.instrument_id)}
            </span>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>今日新闻</h2>
        {news.length === 0 && <div className="feed-item">暂无新闻</div>}
        {news.map(n => (
          <div key={n.id}
            className={`feed-item ${n.body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
            onClick={n.body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
            <div className="fi-meta">{MEDIA_NAMES[n.media_id] ?? n.media_id} · <span className="num">第 {n.day} 日</span></div>
            <div className={n.body ? "fi-title" : ""}>{prettifyHeadline(n.headline, aliasOf)}</div>
            {n.body && <div className="fi-body">{n.body}</div>}
          </div>
        ))}
        {newsItems.length > newsShown && (
          <button className="feed-more" onClick={() => setNewsShown(n => n + 8)}>更早的新闻 ↓</button>
        )}
      </div>
    </div>
  );
}
