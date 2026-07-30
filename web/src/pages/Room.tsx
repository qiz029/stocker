import { useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api";
import { fmtCents, fmt$, fmtPct, fmtSignedCents } from "../format";
import { LangSwitch, useT } from "../i18n";
import { useRoomData, useSimClock } from "../roomData";
import { dayLabel } from "../simClock";
import { useToast } from "../Toast";
import { useUser } from "../App";
import HeroChart from "../components/HeroChart";
import InstrumentRow from "../components/InstrumentRow";
import LoanPanel from "../components/LoanPanel";
import OptionPositions from "../components/OptionPositions";
import RightRail from "../components/RightRail";

const mmss = (s: number) =>
  `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;

export default function Room() {
  const { roomId } = useParams<{ roomId: string }>();
  const navigate = useNavigate();
  const user = useUser();
  const { lang, t } = useT();
  const { toast, node } = useToast();
  const { state, portfolio, series, curve, error, reload } = useRoomData(roomId!);

  const room = state?.room;
  const clock = useSimClock(room);

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (!state || !room) return null;

  const curDay = room.current_day ?? 0;
  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;
  const dateLabel = clock ? dayLabel(clock.day, lang) : "";

  // Asset breakdown: cash / invested / debt against a net total.
  const investedCents = (portfolio?.positions ?? []).reduce((s, p) => s + p.value_cents, 0);
  const cashCents = portfolio?.cash_cents ?? 0;
  const debtCents = portfolio?.debt_cents ?? 0;
  const assetBase = Math.max(1, cashCents + investedCents + debtCents);
  const ratePct = `${((portfolio?.interest_rate_annual_bp ?? 0) / 100).toFixed(2)}%`;

  async function startRoom() {
    try {
      await api.post(`/api/rooms/${roomId}/start`);
      toast(t("room.started"));
      reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : t("room.startFailed"));
    }
  }

  function copyInvite() {
    void navigator.clipboard?.writeText(room!.invite_code);
    toast(t("room.inviteCopied", { code: room!.invite_code }));
  }

  return (
    <div>
      <div className="topbar">
        <div className="brand" onClick={() => navigate("/")}><em>●</em> Stocker</div>
        <div className="day-pill">
          {room.status === "lobby"
            ? t("status.waiting")
            : room.ended
              ? t("room.endedPill")
              : <>{t("era.name")} · {t("lobby.dayA")} <b className="num">{curDay}</b> {t("lobby.dayB", { days: room.days })}</>}
        </div>
        {clock && clock.phase !== "ended" && (
          <div className="countdown">
            {clock.phase === "open"
              ? <>{dateLabel} <b className="num">{clock.time}</b></>
              : <>
                  {clock.phase === "weekend" ? t("room.weekend") : `${dateLabel} · ${t("room.closed")}`}
                  {clock.nextOpenSecs !== null && <>
                    {" · "}{t("room.nextOpen")}{" "}<b className="num">{mmss(clock.nextOpenSecs)}</b>
                  </>}
                </>}
          </div>
        )}
        <div className="spacer" />
        {room.ended && (
          <button className="invite" onClick={() => navigate(`/rooms/${roomId}/reveal`)}>{t("room.reveal")}</button>
        )}
        <button className="invite" onClick={copyInvite}>{t("room.invite")}</button>
        <LangSwitch />
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>

      <div className="wrap">
        {room.status === "lobby" ? (
          <div className="room-card" style={{ cursor: "default" }}>
            <h3>{t("room.notStarted")}</h3>
            <p className="rc-meta">{t("room.shareA")} <b className="num">{room.invite_code}</b> {t("room.shareB")}{room.is_host ? t("room.shareHost") : t("room.shareEnd")}</p>
            {room.is_host
              ? <button className="submit" style={{ marginTop: 14 }} onClick={startRoom}>{t("room.start")}</button>
              : <p className="rc-meta" style={{ marginTop: 14 }}>{t("room.waitingHost")}</p>}
          </div>
        ) : (
          <>
          {portfolio?.bankrupt && (
            <div className="err-banner bankrupt-banner">{t("room.bankruptBanner")}</div>
          )}
          <div className="grid">
            <div>
              {curve.length > 1 && (
                <HeroChart label={t("room.totalAssets")} series={curve} startDay={0} formatValue={fmtCents} />
              )}

              {portfolio && (
                <div className="card assets">
                  <h2>{t("room.assets")}</h2>
                  <div className="asset-bar">
                    <i className="seg cash" style={{ width: `${(cashCents / assetBase) * 100}%` }} />
                    <i className="seg inv" style={{ width: `${(investedCents / assetBase) * 100}%` }} />
                    {debtCents > 0 && <i className="seg debt" style={{ width: `${(debtCents / assetBase) * 100}%` }} />}
                  </div>
                  <div className="est"><span>{t("room.cash")}</span><b className="num">{fmtCents(cashCents)}</b></div>
                  <div className="est"><span>{t("room.assetsInvested")}</span><b className="num">{fmtCents(investedCents)}</b></div>
                  <div className="est"><span>{t("room.assetsDebt")}</span><b className="num">{fmtCents(debtCents)}</b></div>
                  <div className="est"><span>{t("room.assetsNet")}</span><b className="num">{fmtCents(portfolio.total_cents)}</b></div>
                  {debtCents > 0 && (
                    <p className="note">
                      {t("room.assetsRate", { pct: ratePct })} · {t("room.assetsToLine", { amount: fmtCents(Math.max(0, (portfolio.max_debt_cents ?? 0) - debtCents)) })}
                    </p>
                  )}
                </div>
              )}

              <div className="section">
                <h2>{t("room.positions")}</h2>
                {(portfolio?.positions ?? []).map(p => (
                  <InstrumentRow key={p.instrument_id}
                    name={aliasOf(p.instrument_id)}
                    sub={t("room.positionSub", { shares: p.shares.toFixed(1), value: fmtCents(p.value_cents) })}
                    price={fmt$(p.close)}
                    pill={fmtPct(p.close / (series[p.instrument_id]?.[0] ?? p.close) - 1)}
                    pillUp={p.close >= (series[p.instrument_id]?.[0] ?? p.close)}
                    pnl={{
                      text: t("room.positionPnl", {
                        avg: fmt$(p.avg_cost), amount: fmtSignedCents(p.pnl_cents), pct: fmtPct(p.pnl_pct),
                      }),
                      up: p.pnl_cents >= 0,
                    }}
                    onClick={() => navigate(`/rooms/${roomId}/i/${p.instrument_id}`)}
                  />
                ))}
                <InstrumentRow name={t("room.cash")} sub={t("room.cashSub")}
                  price={portfolio ? fmtCents(portfolio.cash_cents) : "—"} pill="" pillUp />
                {(portfolio?.pending?.length ?? 0) > 0 && (
                  <p className="rc-meta">{t("room.pendingNote", { n: portfolio!.pending.length })}</p>
                )}
              </div>

              {(portfolio?.options?.length ?? 0) > 0 && (
                <div className="section">
                  <h2>{t("option.myPositions")}</h2>
                  <OptionPositions roomId={roomId!} positions={portfolio!.options!}
                    currentDay={curDay} aliasOf={aliasOf} onChanged={reload}
                    disabled={(portfolio?.bankrupt ?? false) || (room.ended ?? false)} />
                </div>
              )}

              <div className="section">
                <h2>{t("room.market")}</h2>
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

            <div>
              <LoanPanel roomId={roomId!} portfolio={portfolio} onChanged={reload} />
              <RightRail roomId={roomId!} state={state} aliasOf={aliasOf} />
            </div>
          </div>
          </>
        )}
      </div>
      {node}
    </div>
  );
}
