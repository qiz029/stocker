import { FormEvent, useEffect, useRef, useState } from "react";
import { api, ApiError, ChatMessage } from "../api";
import { pickL, useT } from "../i18n";
import { useUser } from "../App";
import { avatarGlyph } from "../avatar";

export default function Chat({ roomId, readOnly = false }: { roomId: string; readOnly?: boolean }) {
  const user = useUser();
  const myAlias = user.display_name?.trim() || "Player";
  const { t, lang } = useT();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [text, setText] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const lastID = useRef(0);
  const listRef = useRef<HTMLDivElement>(null);

  async function fetchNew() {
    try {
      const res = await api.get<{ items: ChatMessage[] }>(`/api/rooms/${roomId}/chat?after=${lastID.current}`);
      const fresh = res.items.filter(m => m.id > lastID.current);
      if (fresh.length) {
        lastID.current = fresh[fresh.length - 1]!.id;
        setMessages(m => {
          const seen = new Set(m.map(x => x.id));
          return [...m, ...fresh.filter(x => !seen.has(x.id))];
        });
      }
    } catch {
      /* transient poll errors are silent; next tick retries */
    }
  }

  useEffect(() => {
    lastID.current = 0;
    setMessages([]);
    void fetchNew();
    const t = setInterval(() => void fetchNew(), 30_000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomId]);

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

  async function send(e: FormEvent) {
    e.preventDefault();
    const body = text.trim();
    if (!body) return;
    setErr(null);
    try {
      await api.post(`/api/rooms/${roomId}/chat`, { text: body });
      setText("");
      await fetchNew();
    } catch (error) {
      setErr(error instanceof ApiError ? error.message : t("chat.sendFailed"));
    }
  }

  return (
    <div className="card">
      <h2>{t("chat.title")}</h2>
      <div className="chat-list" ref={listRef}>
        {messages.map(m => (
          <div key={m.id} className={`chat-msg ${!m.is_agent && (m.is_me ?? m.username === myAlias) ? "me" : ""} ${m.is_agent ? "agent" : ""}`}>
            <div className="cm-meta">
              {!m.is_agent && <span className="chat-avatar">{avatarGlyph(m.avatar_id, m.username)}</span>}
              <b>{m.is_agent ? pickL(lang, m.username, m.username_en) : m.username}</b>
              {m.is_agent && <small className="agent-badge">{t("common.agent")}</small>}
              {" · "}<span className="num">{t("common.day", { day: m.day })}</span>
            </div>
            <span className="cm-bubble">{m.is_agent ? pickL(lang, m.text, m.text_en) : m.text}</span>
          </div>
        ))}
      </div>
      {err && <p className="form-error">{err}</p>}
      {!readOnly && <form className="chat-input" onSubmit={send}>
        <input placeholder={t("chat.placeholder")} value={text} maxLength={500}
          onChange={e => setText(e.target.value)} />
        <button type="submit">{t("chat.send")}</button>
      </form>}
    </div>
  );
}
