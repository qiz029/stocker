import { FormEvent, useEffect, useRef, useState } from "react";
import { api, ApiError, ChatMessage } from "../api";
import { useUser } from "../App";

export default function Chat({ roomId }: { roomId: string }) {
  const user = useUser();
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
    const t = text.trim();
    if (!t) return;
    setErr(null);
    try {
      await api.post(`/api/rooms/${roomId}/chat`, { text: t });
      setText("");
      await fetchNew();
    } catch (error) {
      setErr(error instanceof ApiError ? error.message : "发送失败");
    }
  }

  return (
    <div className="card">
      <h2>聊天室</h2>
      <div className="chat-list" ref={listRef}>
        {messages.map(m => (
          <div key={m.id} className={`chat-msg ${m.username === user.username ? "me" : ""}`}>
            <div className="cm-meta"><b>{m.username}</b> · <span className="num">第 {m.day} 日</span></div>
            <span className="cm-bubble">{m.text}</span>
          </div>
        ))}
      </div>
      {err && <p className="form-error">{err}</p>}
      <form className="chat-input" onSubmit={send}>
        <input placeholder="说点什么…" value={text} maxLength={500}
          onChange={e => setText(e.target.value)} />
        <button type="submit">发送</button>
      </form>
    </div>
  );
}
