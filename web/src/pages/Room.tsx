import { FormEvent, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, ApiError, AvatarID, User } from "../api";
import { fmtCents, fmt$, fmtPct, fmtSignedCents } from "../format";
import { LangSwitch, pickL, useT } from "../i18n";
import { useRoomData, useSimClock } from "../roomData";
import { dayLabel } from "../simClock";
import { useToast } from "../Toast";
import { useUpdateUser, useUser } from "../App";
import HeroChart from "../components/HeroChart";
import InstrumentRow from "../components/InstrumentRow";
import LoanPanel from "../components/LoanPanel";
import OptionPositions from "../components/OptionPositions";
import RightRail from "../components/RightRail";
import DocsLink from "../components/DocsLink";
import MobileNav, { scrollToMobileSection } from "../components/MobileNav";
import "./LobbyHall.css";
import { avatarGlyph, avatarGlyphs, avatarIDs } from "../avatar";

const mmss = (s: number) =>
  `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;

export default function Room() {
  const { roomId } = useParams<{ roomId: string }>();
  const navigate = useNavigate();
  const user = useUser();
  const updateUser = useUpdateUser();
  const { lang, t } = useT();
  const { toast, node } = useToast();
  const { state, portfolio, series, curve, error, reload } = useRoomData(roomId!);
  const [showProfile, setShowProfile] = useState(false);
  const [displayName, setDisplayName] = useState(user.display_name ?? "");
  const [avatarID, setAvatarID] = useState<AvatarID>(user.avatar_id ?? "bull");
  const [joining, setJoining] = useState(false);
  const [mobileTab, setMobileTab] = useState<"trend" | "stocks" | "bank" | "activity">("trend");

  useEffect(() => {
    if (!showProfile) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setShowProfile(false);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [showProfile]);

  const room = state?.room;
  const clock = useSimClock(room);

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (!state || !room) return null;

  const curDay = room.current_day ?? 0;
  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;
  const dateLabel = clock ? dayLabel(clock.day, lang) : "";
  const spectator = room.is_member === false;

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
    if (!room!.invite_code) return;
    void navigator.clipboard?.writeText(room!.invite_code);
    toast(t("room.inviteCopied", { code: room!.invite_code }));
  }

  async function joinAsPlayer(e: FormEvent) {
    e.preventDefault(); setJoining(true);
    try {
      if (!user.profile_complete) {
        const updated = await api.put<User>("/api/me/profile", { display_name: displayName.trim(), avatar_id: avatarID });
        updateUser(updated);
      }
      await api.post(`/api/rooms/${roomId}/join`); setShowProfile(false); reload();
    } catch (err) { toast(err instanceof ApiError ? err.message : t("lobby.joinFailed")); }
    finally { setJoining(false); }
  }

  function selectMobileTab(tab: "trend" | "stocks" | "bank" | "activity") {
    setMobileTab(tab);
    scrollToMobileSection("mobile-room-top");
  }

  return (
    <div className={`has-mobile-nav room-page mobile-room-tab-${mobileTab}`}>
      <div className="topbar room-topbar" id="mobile-room-top">
        <button className="room-hall-back" aria-label={lang === "zh" ? "返回大厅" : "Back to hall"} onClick={() => navigate("/")}>
          <span aria-hidden="true">‹</span>{lang === "zh" ? "大厅" : "Hall"}
        </button>
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
        <div className="room-topbar-actions">
          {room.ended && !spectator && (
            <button className="invite room-topbar-action" aria-label={t("room.reveal")} onClick={() => navigate(`/rooms/${roomId}/reveal`)}>
              <span className="room-action-label">{t("room.reveal")}</span><span className="room-action-mark" aria-hidden="true">↗</span>
            </button>
          )}
          {!spectator && <button className="invite room-topbar-action" aria-label={t("room.invite")} onClick={copyInvite}>
            <span className="room-action-label">{t("room.invite")}</span><span className="room-action-mark" aria-hidden="true">＋</span>
          </button>}
          <DocsLink />
          <LangSwitch />
          <div className="avatar">{avatarGlyph(user.avatar_id, user.username)}</div>
        </div>
      </div>

      <div className="wrap" id="mobile-room-content">
        {spectator && <div className="spectator-banner"><div><b>{lang === "zh" ? "围观模式" : "Spectator mode"}</b><span>{lang === "zh" ? "你可以查看公开行情、新闻、聊天和榜单，但不能交易或发言。" : "You can view public market data, news, chat, and standings, but cannot trade or post."}</span></div>{room.status === "lobby" && <button onClick={()=>setShowProfile(true)}>{lang === "zh" ? "加入对局" : "Join game"}</button>}</div>}
        {room.status === "lobby" ? (
          <div className="room-card" style={{ cursor: "default" }}>
            <h3>{t("room.notStarted")}</h3>
            {!spectator && <p className="rc-meta">{t("room.shareA")} <b className="num">{room.invite_code}</b> {t("room.shareB")}{room.is_host ? t("room.shareHost") : t("room.shareEnd")}</p>}
            {spectator ? <p className="rc-meta">{lang === "zh" ? "你正在围观等待开局的公开房间。完成玩家资料后即可入座。" : "You are watching a public waiting room. Complete your player profile to take a seat."}</p> : room.is_host
              ? <button className="submit" style={{ marginTop: 14 }} onClick={startRoom}>{t("room.start")}</button>
              : <p className="rc-meta" style={{ marginTop: 14 }}>{t("room.waitingHost")}</p>}
          </div>
        ) : (
          <>
          {portfolio?.bankrupt && (
            <div className="err-banner bankrupt-banner">{t("room.bankruptBanner")}</div>
          )}
          <div className="grid">
            <div className="room-main-column">
              <section className="room-tab-panel room-trend-panel" id="mobile-room-trend" aria-label={lang === "zh" ? "趋势与持仓" : "Trend and holdings"}>
              {curve.length > 0 && (
                <HeroChart label={t("room.totalAssets")} series={curve} startDay={0} formatValue={fmtCents} intradaySeed={Number(roomId)} />
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

              {!spectator && <div className="section">
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
              </div>}

              {(portfolio?.options?.length ?? 0) > 0 && (
                <div className="section">
                  <h2>{t("option.myPositions")}</h2>
                  <OptionPositions roomId={roomId!} positions={portfolio!.options!}
                    currentDay={curDay} aliasOf={aliasOf} onChanged={reload}
                  disabled={(portfolio?.bankrupt ?? false) || (room.ended ?? false)} />
                </div>
              )}
              </section>

              <section className="section room-tab-panel room-stocks-panel" id="mobile-room-stocks" aria-label={lang === "zh" ? "股票列表" : "Stock list"}>
                <h2>{t("room.market")}</h2>
                {state.instruments.map(inst => {
                  const q = state.quotes.find(x => x.instrument_id === inst.id);
                  const s = series[inst.id] ?? [];
                  return (
                    <InstrumentRow key={inst.id}
                      name={inst.alias} sub={pickL(lang, inst.desc, inst.desc_en)}
                      price={q ? fmt$(q.close) : "—"}
                      pill={q ? fmtPct(q.close / q.prev_close - 1) : ""}
                      pillUp={q ? q.close >= q.prev_close : true}
                      sparkSeries={s.slice(Math.max(0, s.length - 30))}
                      onClick={() => navigate(`/rooms/${roomId}/i/${inst.id}`)}
                    />
                  );
                })}
              </section>
            </div>

            <div className="room-side-column">
              {!spectator && <section className="room-tab-panel room-bank-panel" id="mobile-room-bank" aria-label={lang === "zh" ? "银行" : "Bank"}>
                <LoanPanel roomId={roomId!} portfolio={portfolio} onChanged={reload} />
              </section>}
              <section className="room-tab-panel room-activity-panel" id="mobile-room-activity" aria-label={lang === "zh" ? "房间动态" : "Room activity"}>
                <RightRail roomId={roomId!} state={state} aliasOf={aliasOf} readOnly={spectator} />
              </section>
            </div>
          </div>
          </>
        )}
      </div>
      {showProfile && <div className="hall-dialog-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)setShowProfile(false)}}><form className="hall-dialog" role="dialog" aria-modal="true" aria-labelledby="room-profile-title" onSubmit={joinAsPlayer}><h2 id="room-profile-title">{lang === "zh" ? "设置玩家身份" : "Choose your player identity"}</h2><p>{lang === "zh" ? "加入前填写显示名称并选择头像。" : "Add a display name and avatar before joining."}</p><label>{lang === "zh" ? "显示名称" : "Display name"}<input autoFocus minLength={2} maxLength={24} required value={displayName} onChange={e=>setDisplayName(e.target.value)}/></label><label>{lang === "zh" ? "头像" : "Avatar"}<div className="hall-avatars">{avatarIDs.map(id=><button type="button" aria-label={id} aria-pressed={avatarID===id} className={`hall-avatar-option ${avatarID===id?"on":""}`} key={id} onClick={()=>setAvatarID(id)}>{avatarGlyphs[id]}</button>)}</div></label><div className="hall-dialog-actions"><button type="button" onClick={()=>setShowProfile(false)}>{lang === "zh" ? "取消" : "Cancel"}</button><button className="confirm" disabled={joining||displayName.trim().length<2}>{joining?"…":lang === "zh" ? "保存并加入" : "Save & join"}</button></div></form></div>}
      <MobileNav
        label={lang === "zh" ? "对局导航" : "Game navigation"}
        active={mobileTab}
        items={room.status === "lobby" ? [
          { id: "trend", icon: "◒", label: lang === "zh" ? "房间" : "Room", onSelect: () => selectMobileTab("trend") },
          spectator
            ? { id: "join", icon: "+", label: lang === "zh" ? "加入" : "Join", primary: true, onSelect: () => setShowProfile(true) }
            : { id: "invite", icon: "+", label: lang === "zh" ? "邀请" : "Invite", primary: true, onSelect: copyInvite },
          { id: "docs", icon: "?", label: lang === "zh" ? "规则" : "Rules", onSelect: () => navigate("/docs") },
        ] : [
          { id: "trend", icon: "⌁", label: lang === "zh" ? "趋势" : "Trend", onSelect: () => selectMobileTab("trend") },
          { id: "stocks", icon: "▥", label: lang === "zh" ? "股票" : "Stocks", onSelect: () => selectMobileTab("stocks") },
          ...(!spectator ? [{ id: "bank", icon: "$", label: lang === "zh" ? "银行" : "Bank", onSelect: () => selectMobileTab("bank") }] : []),
          { id: "activity", icon: "◉", label: lang === "zh" ? "动态" : "Activity", onSelect: () => selectMobileTab("activity") },
        ]}
      />
      {node}
    </div>
  );
}
