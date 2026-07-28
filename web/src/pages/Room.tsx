import { useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api";
import { fmtCents, fmt$, fmtPct } from "../format";
import { useRoomData, useSimClock } from "../roomData";
import { useToast } from "../Toast";
import { useUser } from "../App";
import HeroChart from "../components/HeroChart";
import InstrumentRow from "../components/InstrumentRow";
import RightRail from "../components/RightRail";

const mmss = (s: number) =>
  `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;

export default function Room() {
  const { roomId } = useParams<{ roomId: string }>();
  const navigate = useNavigate();
  const user = useUser();
  const { toast, node } = useToast();
  const { state, portfolio, series, curve, error, reload } = useRoomData(roomId!);

  const room = state?.room;
  const clock = useSimClock(room);

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (!state || !room) return null;

  const curDay = room.current_day ?? 0;
  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;

  async function startRoom() {
    try {
      await api.post(`/api/rooms/${roomId}/start`);
      toast("时间轴已启动");
      reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "启动失败");
    }
  }

  function copyInvite() {
    void navigator.clipboard?.writeText(room!.invite_code);
    toast(`邀请码 ${room!.invite_code} 已复制`);
  }

  return (
    <div>
      <div className="topbar">
        <div className="brand" onClick={() => navigate("/")}><em>●</em> Stocker</div>
        <div className="day-pill">
          {room.status === "lobby"
            ? "等待开局"
            : room.ended
              ? "已结束 · 等待揭晓"
              : <>神秘年代 · 第 <b className="num">{curDay}</b> / {room.days} 个交易日</>}
        </div>
        {clock && clock.phase !== "ended" && (
          <div className="countdown">
            {clock.phase === "open"
              ? <>{clock.dateLabel} <b className="num">{clock.time}</b></>
              : <>
                  {clock.phase === "weekend" ? "周末休市" : `${clock.dateLabel} · 已收盘`}
                  {clock.nextOpenSecs !== null && <>
                    {" · 距开盘 "}<b className="num">{mmss(clock.nextOpenSecs)}</b>
                  </>}
                </>}
          </div>
        )}
        <div className="spacer" />
        {room.ended && (
          <button className="invite" onClick={() => navigate(`/rooms/${roomId}/reveal`)}>查看揭晓</button>
        )}
        <button className="invite" onClick={copyInvite}>邀请好友</button>
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>

      <div className="wrap">
        {room.status === "lobby" ? (
          <div className="room-card" style={{ cursor: "default" }}>
            <h3>房间尚未开局</h3>
            <p className="rc-meta">把邀请码 <b className="num">{room.invite_code}</b> 发给朋友{room.is_host && "；人齐后由房主启动时间轴"}。</p>
            {room.is_host
              ? <button className="submit" style={{ marginTop: 14 }} onClick={startRoom}>启动时间轴（房主）</button>
              : <p className="rc-meta" style={{ marginTop: 14 }}>等待房主启动…</p>}
          </div>
        ) : (
          <div className="grid">
            <div>
              {curve.length > 1 && (
                <HeroChart label="总资产" series={curve} startDay={0} formatValue={fmtCents} />
              )}

              <div className="section">
                <h2>持仓</h2>
                {(portfolio?.positions ?? []).map(p => (
                  <InstrumentRow key={p.instrument_id}
                    name={aliasOf(p.instrument_id)}
                    sub={`${p.shares.toFixed(1)} 股 · 市值 ${fmtCents(p.value_cents)}`}
                    price={fmt$(p.close)}
                    pill={fmtPct(p.close / (series[p.instrument_id]?.[0] ?? p.close) - 1)}
                    pillUp={p.close >= (series[p.instrument_id]?.[0] ?? p.close)}
                    onClick={() => navigate(`/rooms/${roomId}/i/${p.instrument_id}`)}
                  />
                ))}
                <InstrumentRow name="现金" sub="可随时下单"
                  price={portfolio ? fmtCents(portfolio.cash_cents) : "—"} pill="" pillUp />
                {(portfolio?.pending?.length ?? 0) > 0 && (
                  <p className="rc-meta">另有 {portfolio!.pending.length} 笔挂单冻结中，开盘成交。</p>
                )}
              </div>

              <div className="section">
                <h2>行情 · 盲盒标的</h2>
                {state.instruments.map(inst => {
                  const q = state.quotes.find(x => x.instrument_id === inst.id);
                  const s = series[inst.id] ?? [];
                  return (
                    <InstrumentRow key={inst.id}
                      name={inst.alias} sub={inst.desc}
                      price={q ? fmt$(q.close) : "—"}
                      pill={q ? fmtPct(q.close / q.prev_close - 1) : ""}
                      pillUp={q ? q.close >= q.prev_close : true}
                      sparkSeries={s.slice(Math.max(0, s.length - 30))}
                      onClick={() => navigate(`/rooms/${roomId}/i/${inst.id}`)}
                    />
                  );
                })}
              </div>
            </div>

            <RightRail roomId={roomId!} state={state} aliasOf={aliasOf} />
          </div>
        )}
      </div>
      {node}
    </div>
  );
}
