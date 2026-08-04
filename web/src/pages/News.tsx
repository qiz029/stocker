import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError, DebunkVerdict, NewsItem, RoomState, fetchNewsItem, api, postDebunk } from "../api";
import { fmtCents, prettifyHeadline } from "../format";
import { LangSwitch, mediaName, pickL, useT } from "../i18n";
import DocsLink from "../components/DocsLink";
import { useUser } from "../App";
import { loadDebunkVerdicts, saveDebunkVerdict } from "../debunkVerdicts";

const DEBUNK_FEE_CENTS = 200_000;
type NewsError = { message: string } | { key: "news.loadFailed" | "news.investigateFailed" };

export default function News() {
  const { roomId, newsId } = useParams<{ roomId: string; newsId: string }>();
  const user = useUser();
  const { t, lang } = useT();
  const [story, setStory] = useState<NewsItem | null>(null);
  const [room, setRoom] = useState<RoomState | null>(null);
  const [error, setError] = useState<NewsError | null>(null);
  const [verdict, setVerdict] = useState<DebunkVerdict | null>(null);

  useEffect(() => {
    let active = true;
    setStory(null);
    setRoom(null);
    setError(null);
    setVerdict(loadDebunkVerdicts(user.id, roomId!)[Number(newsId)] ?? null);
    Promise.all([
      fetchNewsItem(roomId!, newsId!),
      api.get<RoomState>(`/api/rooms/${roomId}`),
    ]).then(([nextStory, nextRoom]) => {
      if (!active) return;
      setStory(nextStory);
      setRoom(nextRoom);
    }).catch((err: unknown) => {
      if (active) setError(err instanceof ApiError ? { message: err.message } : { key: "news.loadFailed" });
    });
    return () => { active = false; };
  }, [newsId, roomId, user.id]);

  async function investigate() {
    if (!story) return;
    setError(null);
    try {
      const result = await postDebunk(roomId!, story.id);
      setVerdict(result.verdict);
      saveDebunkVerdict(user.id, roomId!, story.id, result.verdict);
    } catch (err) {
      setError(err instanceof ApiError ? { message: err.message } : { key: "news.investigateFailed" });
    }
  }

  const aliasOf = (id: string) => room?.instruments.find(inst => inst.id === id)?.alias ?? id;

  return (
    <div className="wrap news-detail">
      <header className="news-detail-nav">
        <Link className="back-btn" to={`/rooms/${roomId}`}>{t("common.backToRoom")}</Link>
        <div className="page-tools"><DocsLink /><LangSwitch /></div>
      </header>
      {error && <div className="err-banner">{"message" in error ? error.message : t(error.key)}</div>}
      {story && room && (
        <article className="card news-article">
          <div className="news-kicker">
            {mediaName(story.media_id, t)}
            {" · "}<span className="num">{t("common.day", { day: story.day })}</span>
            {(story.disputed || verdict) && <span className="fi-badge disputed">{t("news.disputed")}</span>}
            {story.exposed && <span className="fi-badge exposed">{t("news.exposed")}</span>}
          </div>
          <h1>{prettifyHeadline(pickL(lang, story.headline, story.headline_en), aliasOf)}</h1>
          <div className="news-body">
            {pickL(lang, story.body, story.body_en) || <span className="muted">{t("news.noBody")}</span>}
          </div>
          <div className="fi-actions">
            {room.room.is_member !== false && !story.disputed && !verdict && (
              <button className="fi-act" onClick={investigate}>
                {t("news.investigate", { fee: fmtCents(DEBUNK_FEE_CENTS) })}
              </button>
            )}
            {verdict && (
              <span className="fi-verdict">
                {t(`news.verdict.${verdict}`)} <small>{t("news.verdictPrivate")}</small>
              </span>
            )}
          </div>
        </article>
      )}
    </div>
  );
}
