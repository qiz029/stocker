import { Link, useNavigate, useParams } from "react-router-dom";
import { NewsItem, NewsResponse, fetchNews } from "../api";
import { fmt$, fmtPct, prettifyHeadline } from "../format";
import { LangSwitch, mediaName, pickL, useT } from "../i18n";
import { useRoomData, useSimClock } from "../roomData";
import { useIncrementalFeed } from "../useIncrementalFeed";
import { useState } from "react";
import CandleChart from "../components/CandleChart";
import ActionPanel from "../components/ActionPanel";
import OptionsChain from "../components/OptionsChain";
import OptionPositions from "../components/OptionPositions";
import TradePanel from "../components/TradePanel";
import DocsLink from "../components/DocsLink";

export default function Stock() {
  const { roomId, instrumentId } = useParams<{ roomId: string; instrumentId: string }>();
  const navigate = useNavigate();
  const { t, lang } = useT();
  const { state, portfolio, series, ohlc, reload } = useRoomData(roomId!);
  const clock = useSimClock(state?.room);
  const { items: newsItems } = useIncrementalFeed<NewsItem, NewsResponse>(
    after => fetchNews(roomId!, after), 30_000, roomId!);
  const [openNews, setOpenNews] = useState<number | null>(null);

  if (!state) return null;
  const spectator = state.room.is_member === false;
  const inst = state.instruments.find(i => i.id === instrumentId);
  const closes = series[instrumentId!] ?? [];
  const candles = ohlc[instrumentId!] ?? [];
  if (!inst) return <div className="wrap err-banner">{t("stock.unknown")}</div>;

  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;
  const last = closes[closes.length - 1] ?? 0;
  const prev = closes[closes.length - 2] ?? last;
  const q3m = closes.slice(-63);
  const held = portfolio?.positions.find(p => p.instrument_id === instrumentId)?.shares ?? 0;
  const optionPositions = (portfolio?.options ?? []).filter(o => o.instrument_id === instrumentId);
  const optionsLocked = (portfolio?.bankrupt ?? false) || (state.room.ended ?? false);
  const optionsNote = portfolio?.bankrupt
    ? t("trade.bankruptNote")
    : state.room.ended ? t("option.endedNote") : undefined;
  const actionsNote = portfolio?.bankrupt
    ? t("actions.bankruptNote")
    : state.room.ended ? t("actions.endedNote") : undefined;
  const relatedNews = newsItems
    .filter(n => n.headline.includes(instrumentId!) || n.headline.includes(inst.alias))
    .sort((a, b) => b.id - a.id)
    .slice(0, 5);

  return (
    <div className="wrap">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button className="back-btn" onClick={() => navigate(`/rooms/${roomId}`)}>{t("common.backToRoom")}</button>
        <div className="page-tools"><DocsLink /><LangSwitch /></div>
      </div>
      <div className="stock-grid">
        <div>
          {candles.length > 1 && (
            <CandleChart
              label={`${inst.alias} · ${pickL(lang, inst.desc, inst.desc_en)} · ${inst.id}`}
              days={candles} formatValue={fmt$}
            />
          )}
          <div className="stat-strip num">
            <div><span className="k">{t("stock.todayClose")}</span>{fmt$(last)}</div>
            <div><span className="k">{t("stock.prevClose")}</span>{fmt$(prev)}</div>
            <div><span className="k">{t("stock.high3m")}</span>{q3m.length ? fmt$(Math.max(...q3m)) : "—"}</div>
            <div><span className="k">{t("stock.low3m")}</span>{q3m.length ? fmt$(Math.min(...q3m)) : "—"}</div>
            <div><span className="k">{t("stock.sinceStart")}</span>
              <span className={`delta ${last >= (closes[0] ?? last) ? "up" : "down"}`}>
                {closes[0] ? fmtPct(last / closes[0] - 1) : "—"}
              </span>
            </div>
            {!spectator && <div><span className="k">{t("stock.myHolding")}</span>{held > 0 ? t("unit.shares", { n: held.toFixed(1) }) : "—"}</div>}
          </div>

          {!spectator && optionPositions.length > 0 && (
            <div className="section">
              <h2>{t("option.myPositions")}</h2>
              <OptionPositions roomId={roomId!} positions={optionPositions}
                currentDay={state.room.current_day ?? 0} aliasOf={aliasOf}
                onChanged={reload} disabled={optionsLocked} />
            </div>
          )}

          {!spectator && <OptionsChain roomId={roomId!} instrumentId={instrumentId!} alias={inst.alias}
            lastClose={last} currentDay={state.room.current_day ?? 0}
            portfolio={portfolio} onChanged={reload}
            disabled={optionsLocked} note={optionsNote} />}

          {!spectator && <ActionPanel roomId={roomId!} instrumentId={instrumentId!} alias={inst.alias}
            portfolio={portfolio} onChanged={reload}
            disabled={optionsLocked} note={actionsNote} />}

          <div className="section">
            <h2>{t("stock.profile")}</h2>
            <div className="profile-grid">
              <div className="profile-item"><div className="pk">{t("stock.profileDesc")}</div><p>{pickL(lang, inst.desc, inst.desc_en) || "——"}</p></div>
              {inst.profile && (
                <>
                  <div className="profile-item"><div className="pk">{t("stock.profileBusiness")}</div><p>{pickL(lang, inst.profile.business, inst.profile_en?.business)}</p></div>
                  <div className="profile-item"><div className="pk bull">{t("stock.profileBull")}</div><p>{pickL(lang, inst.profile.bull, inst.profile_en?.bull)}</p></div>
                  <div className="profile-item"><div className="pk bear">{t("stock.profileBear")}</div><p>{pickL(lang, inst.profile.bear, inst.profile_en?.bear)}</p></div>
                </>
              )}
            </div>
          </div>

          <div className="section">
            <h2>{t("stock.relatedNews")}</h2>
            {relatedNews.length === 0 && <div className="feed-item">{t("stock.noNews")}</div>}
            {relatedNews.map(n => {
              const headline = pickL(lang, n.headline, n.headline_en);
              const body = pickL(lang, n.body, n.body_en);
              return (
              <div key={n.id}
                className={`feed-item ${body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
                onClick={body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
                <div className="fi-meta">{mediaName(n.media_id, t)} · <span className="num">{t("common.day", { day: n.day })}</span></div>
                <div className={body ? "fi-title" : ""}>{prettifyHeadline(headline, aliasOf)}</div>
                {body && <div className="fi-body">{body}</div>}
                <div className="fi-actions" onClick={e => e.stopPropagation()}>
                  <Link className="fi-act" to={`/rooms/${roomId}/news/${n.id}`}>{t("news.readFull")}</Link>
                </div>
              </div>
              );
            })}
          </div>
        </div>

        {!spectator && <TradePanel roomId={roomId!} instrumentId={instrumentId!} lastClose={last}
          portfolio={portfolio} onChanged={reload} disabled={portfolio?.bankrupt ?? false}
          afterHours={clock?.phase === "closed" || clock?.phase === "weekend"} />}
      </div>
    </div>
  );
}
