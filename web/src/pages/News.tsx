import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError, DebunkVerdict, NewsItem, RoomState, fetchNewsItem, api, postDebunk } from "../api";
import { fmtCents, prettifyHeadline } from "../format";
import { LangSwitch, mediaName, pickL, useT } from "../i18n";

const DEBUNK_FEE_CENTS = 200_000;

export default function News() {
  const { roomId, newsId } = useParams<{ roomId: string; newsId: string }>();
  const { t, lang } = useT();
  const [story, setStory] = useState<NewsItem | null>(null);
  const [room, setRoom] = useState<RoomState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [verdict, setVerdict] = useState<DebunkVerdict | null>(null);
  const loadFailed = t("news.loadFailed");

  useEffect(() => {
    let active = true;
    setStory(null);
    setRoom(null);
    setError(null);
    setVerdict(null);
    Promise.all([
      fetchNewsItem(roomId!, newsId!),
      api.get<RoomState>(`/api/rooms/${roomId}`),
    ]).then(([nextStory, nextRoom]) => {
      if (!active) return;
      setStory(nextStory);
      setRoom(nextRoom);
    }).catch((err: unknown) => {
      if (active) setError(err instanceof ApiError ? err.message : loadFailed);
    });
    return () => { active = false; };
  }, [loadFailed, newsId, roomId]);

  async function investigate() {
    if (!story) return;
    setError(null);
    try {
      const result = await postDebunk(roomId!, story.id);
      setVerdict(result.verdict);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("news.investigateFailed"));
    }
  }

  const aliasOf = (id: string) => room?.instruments.find(inst => inst.id === id)?.alias ?? id;

  return (
    <div className="wrap news-detail">
      <header className="news-detail-nav">
        <Link className="back-btn" to={`/rooms/${roomId}`}>{t("common.backToRoom")}</Link>
        <LangSwitch />
      </header>
      {error && <div className="err-banner">{error}</div>}
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
            {!story.disputed && !verdict && (
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
