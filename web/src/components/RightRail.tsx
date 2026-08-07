import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { ApiError, DebunkVerdict, EventItem, ForumItem, MediaAccuracy, NewsItem, NewsResponse, RoomState, api, fetchForum, fetchNews, postDebunk } from "../api";
import { fmtCents, fmtPct, prettifyHeadline } from "../format";
import { MsgKey, TFunc, mediaName, pickL, useT } from "../i18n";
import { useIncrementalFeed } from "../useIncrementalFeed";
import { useToast } from "../Toast";
import { useUser } from "../App";
import Chat from "./Chat";
import Sparkline from "./Sparkline";
import { loadDebunkVerdicts, saveDebunkVerdict } from "../debunkVerdicts";
import { avatarGlyph } from "../avatar";

type Props = { roomId: string; state: RoomState; aliasOf: (id: string) => string; readOnly?: boolean };

type NewsGroup = { key: string; items: NewsItem[] };

/** Group the feed into story chains: items sharing a cluster_id form one
    visual group (positioned by its latest item); standalone items stay
    singletons. Chain items are displayed in day order (rumor → follow-up). */
function groupNews(items: NewsItem[]): NewsGroup[] {
  const groups: NewsGroup[] = [];
  const chains = new Map<number, NewsGroup>();
  for (const n of [...items].sort((a, b) => b.id - a.id)) {
    if (n.cluster_id == null) {
      groups.push({ key: `n${n.id}`, items: [n] });
      continue;
    }
    let g = chains.get(n.cluster_id);
    if (!g) {
      g = { key: `c${n.cluster_id}`, items: [] };
      chains.set(n.cluster_id, g);
      groups.push(g);
    }
    g.items.push(n);
  }
  for (const g of groups) g.items.sort((a, b) => a.day - b.day || a.id - b.id);
  return groups;
}

/** Role label by position in the chain: earliest = rumor, latest = follow-up
    (only once the chain has all three parts), everything between = report.
    Partially published chains label what exists. */
function chainRole(index: number, length: number): MsgKey {
  if (index === 0) return "news.chain.rumor";
  if (index === length - 1 && length >= 3) return "news.chain.followup";
  return "news.chain.report";
}

/** Recent-accuracy badge text for an outlet, or null when the server has no
    stats for it. Outlets with < 3 reports get an insufficient-data hint. */
function accuracyText(acc: MediaAccuracy | undefined, mediaID: string, t: TFunc): string | null {
  const s = acc?.[mediaID];
  if (!s || s.reports <= 0) return null;
  if (s.reports < 3) return t("news.accuracyNA");
  return t("news.accuracy", { pct: Math.round((s.hits / s.reports) * 100) });
}

const DEBUNK_FEE_CENTS = 200_000;

export default function RightRail({ roomId, state, aliasOf, readOnly = false }: Props) {
  const user = useUser();
  const { t, lang } = useT();
  const { toast, node } = useToast();
  const [newsShown, setNewsShown] = useState(8);
  const [openNews, setOpenNews] = useState<number | null>(null);
  const [railTab, setRailTab] = useState<"news" | "forum">("news");
  // Verdicts from this session's investigations: private to the acting user.
  const [verdicts, setVerdicts] = useState<Record<number, DebunkVerdict>>(
    () => loadDebunkVerdicts(user.id, roomId),
  );
  useEffect(() => {
    setVerdicts(loadDebunkVerdicts(user.id, roomId));
  }, [roomId, user.id]);
  const { items: newsItems, extra: newsExtra } = useIncrementalFeed<NewsItem, NewsResponse>(
    after => fetchNews(roomId, after), 30_000, roomId);
  const { items: eventsItems } = useIncrementalFeed<EventItem, { items: EventItem[] }>(
    after => api.get<{ items: EventItem[] }>(`/api/rooms/${roomId}/events?after=${after}`), 30_000, roomId);
  const { items: forumItems } = useIncrementalFeed<ForumItem, { items: ForumItem[] }>(
    after => fetchForum(roomId, after), 30_000, roomId);

  const accuracy = newsExtra?.media_accuracy;
  const groups = groupNews(newsItems);
  const events = [...eventsItems].sort((a, b) => b.id - a.id).slice(0, 6);
  const forum = [...forumItems].sort((a, b) => b.id - a.id).slice(0, 20);

  async function investigate(newsID: number) {
    try {
      const res = await postDebunk(roomId, newsID);
      setVerdicts(v => ({ ...v, [newsID]: res.verdict }));
      saveDebunkVerdict(user.id, roomId, newsID, res.verdict);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("news.investigateFailed"));
    }
  }

  const renderNewsItem = (n: NewsItem, role?: MsgKey) => {
    const acc = accuracyText(accuracy, n.media_id, t);
    const verdict = verdicts[n.id];
    const headline = pickL(lang, n.headline, n.headline_en);
    const body = pickL(lang, n.body, n.body_en);
    return (
      <div key={n.id}
        className={`feed-item ${body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
        onClick={body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
        <div className="fi-meta">
          {role && <span className={`chain-role ${role.split(".").pop()}`}>{t(role)}</span>}
          {mediaName(n.media_id, t)}
          {acc && <span className="acc"> · {acc}</span>}
          {" · "}<span className="num">{t("common.day", { day: n.day })}</span>
          {(n.disputed || verdict) && <span className="fi-badge disputed">{t("news.disputed")}</span>}
          {n.exposed && <span className="fi-badge exposed">{t("news.exposed")}</span>}
        </div>
        <div className={body ? "fi-title" : ""}>{prettifyHeadline(headline, aliasOf)}</div>
        {body && <div className="fi-body">{body}</div>}
        <div className="fi-actions" onClick={e => e.stopPropagation()}>
          <Link className="fi-act" to={`/rooms/${roomId}/news/${n.id}`}>{t("news.readFull")}</Link>
          {!readOnly && !n.disputed && !verdict && (
            <button className="fi-act" onClick={() => investigate(n.id)}>
              {t("news.investigate", { fee: fmtCents(DEBUNK_FEE_CENTS) })}
            </button>
          )}
          {verdict && (
            <span className="fi-verdict">
              {t(`news.verdict.${verdict}`)} <small>{t("news.verdictPrivate")}</small>
            </span>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="right-rail">
      <div className="card rail-leaderboard">
        <h2>{t("rail.leaderboard")}</h2>
        {state.leaderboard.map((row, i) => (
          <div key={row.username}
            className={`lb-row ${row.username === user.username ? "me" : ""} ${row.bankrupt ? "bankrupt" : ""}`}>
            <span className="rank num">{i + 1}</span>
            {!row.is_agent && <span className="lb-player-avatar">{avatarGlyph(row.avatar_id, row.username)}</span>}
            <span className="who"><span className="lb-name">{row.is_agent ? pickL(lang, row.username, row.username_en) : row.username}</span>
              {row.is_agent && <small className="agent-badge">{t("common.agent")}</small>}
              {row.late_join && <small>{t("reveal.lateJoin")}</small>}
              {row.bankrupt && <small className="lb-badge">{t("rail.bankrupt")}</small>}
            </span>
            {(row.curve?.length ?? 0) > 1 && <Sparkline series={row.curve} />}
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <div className="rail-chat"><Chat roomId={roomId} readOnly={readOnly} /></div>

      <div className="card rail-events">
        <h2>{t("rail.events")}</h2>
        {events.length === 0 && <div className="feed-item">{t("rail.noEvents")}</div>}
        {events.map(ev => ev.kind === "agent_order" ? (
          <div key={ev.id} className="feed-item agent-action">
            <div className="fi-meta">
              <span className="forum-npc">{ev.payload.is_agent ? pickL(lang, ev.payload.username ?? "?", ev.payload.username_en) : ev.payload.username ?? "?"}</span>
              {ev.payload.is_agent && <small className="agent-badge">{t("common.agent")}</small>}
              {" · "}<span className="num">{t("rail.tradingDay", { day: ev.day })}</span>
            </div>
            <span className="whale-txt">
              {t(ev.payload.side === "buy" ? "rail.orderBuy" : "rail.orderSell", {
                alias: aliasOf(ev.payload.instrument_id ?? ""),
              })}
            </span>
          </div>
        ) : ev.kind === "manipulation_bust" ? (
          <div key={ev.id} className="feed-item bust">
            <div className="fi-meta num">{t("rail.tradingDay", { day: ev.day })}</div>
            <span className="whale-txt">
              {t("rail.bust", {
                username: ev.payload.username ?? "?",
                amount: fmtCents(ev.payload.fine_cents ?? 0),
                alias: aliasOf(ev.payload.instrument_id ?? ""),
              })}
            </span>
          </div>
        ) : ev.kind === "bankrupt" ? (
          <div key={ev.id} className="feed-item bust">
            <div className="fi-meta num">{t("rail.tradingDay", { day: ev.day })}</div>
            <span className="whale-txt">
              {t("rail.bankruptEvent", { username: ev.payload.username ?? "?" })}
            </span>
          </div>
        ) : (
          <div key={ev.id} className={`feed-item whale ${ev.payload.side === "sell" ? "sell" : ""}`}>
            <div className="fi-meta num">{t("rail.tradingDay", { day: ev.day })}</div>
            <span className="whale-txt">
              {t("rail.whale", {
                side: t(ev.payload.side === "buy" ? "side.buy" : "side.sell"),
                alias: aliasOf(ev.payload.instrument_id ?? ""),
              })}
            </span>
          </div>
        ))}
      </div>

      <div className="card rail-news-forum">
        <div className="seg">
          <button className={railTab === "news" ? "on" : ""} onClick={() => setRailTab("news")}>{t("rail.tabNews")}</button>
          <button className={railTab === "forum" ? "on" : ""} onClick={() => setRailTab("forum")}>{t("rail.tabForum")}</button>
        </div>
        {railTab === "news" ? (
          <>
            {groups.length === 0 && <div className="feed-item">{t("rail.noNews")}</div>}
            {groups.slice(0, newsShown).map(g =>
              g.items.length === 1 && g.items[0]!.cluster_id == null
                ? renderNewsItem(g.items[0]!)
                : (
                  <div className="news-chain" key={g.key}>
                    {g.items.map((n, i) => renderNewsItem(n, chainRole(i, g.items.length)))}
                  </div>
                ))}
            {groups.length > newsShown && (
              <button className="feed-more" onClick={() => setNewsShown(n => n + 8)}>{t("rail.moreNews")}</button>
            )}
          </>
        ) : (
          <>
            {forum.length === 0 && <div className="feed-item">{t("rail.noForum")}</div>}
            {forum.map(p => (
              <div key={p.id} className="feed-item">
                <div className="fi-meta">
                  <span className="forum-npc">{pickL(lang, p.npc_name, p.npc_name_en)}</span>
                  {p.is_agent && <small className="agent-badge">{t("common.agent")}</small>}
                  {" · "}<span className="num">{t("common.day", { day: p.day })}</span>
                </div>
                <div>{pickL(lang, p.body, p.body_en)}</div>
              </div>
            ))}
          </>
        )}
        {node}
      </div>
    </div>
  );
}
