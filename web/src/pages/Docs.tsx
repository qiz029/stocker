import { Link, useLocation } from "react-router-dom";
import { LangSwitch, useT } from "../i18n";

const copy = {
  en: {
    eyebrow: "Blind-box market simulation",
    title: "How to play Stocker",
    intro: "Read the tape, weigh unreliable information, and grow your net worth before the market's hidden history is revealed.",
    startTitle: "Start in four moves",
    steps: [
      ["Enter a room", "Create a game or join one with an invite code. The host starts the timeline when everyone is ready."],
      ["Trade blind", "Companies appear under aliases. Their real identities and historical period stay hidden until the game ends."],
      ["Time your orders", "Buy and sell orders execute at the next market open, at a price you do not know yet. Pending orders can be cancelled before they fill."],
      ["Win on net worth", "The leaderboard uses net assets: cash and investments, minus debt. The highest final value wins."],
    ],
    loopTitle: "The core loop",
    loop: [
      ["Observe", "Read price action, candlesticks, company profiles, news chains, outlet accuracy and the player forum."],
      ["Decide", "Separate signal from noise. A confident headline may be wrong, while a quiet report may matter tomorrow."],
      ["Act", "Trade shares, use the credit line carefully, explore options, or pay for information and influence."],
      ["Compete", "Play with friends and five Agent competitors. Agents trade, appear on the leaderboard and occasionally speak in chat with an Agent badge."],
    ],
    newsTitle: "News is a signal, not the truth",
    newsIntro: "Market stories are written from noisy reports. Different outlets have different track records, and a rumor can disagree with the event that eventually unfolds.",
    news: [
      "Open any story to read its full article and check whether it belongs to a rumor → report → follow-up chain.",
      "Recent outlet accuracy compares earlier reporting direction with the move that followed; it is evidence, not a guarantee.",
      "Daily recaps describe already-public price action. They explain the tape but do not predict the next session.",
      "A Disputed or Manipulation confirmed badge is public. Your investigation verdict remains private and can still be wrong.",
    ],
    advancedTitle: "Advanced moves",
    advanced: [
      ["Hype campaign", "Pay to plant a bullish or bearish rumor that can move the target from the next trading day. Repetition loses strength, and getting caught brings a fine and public exposure."],
      ["Investigate", "Pay to examine one published story. You receive a private, fallible verdict; everyone can see that the story is now disputed."],
      ["Inside tip", "Buy a noisy glimpse of tomorrow's strongest signal for one instrument. Tips may be weak, corrupted or simply quiet."],
    ],
    riskTitle: "Risk tools cut both ways",
    risk: [
      ["Credit line", "Borrowing adds cash now, but debt accrues interest and reduces net assets. Crossing the debt limit means bankruptcy."],
      ["Options", "Calls and puts can magnify a view with limited premium, but they lose value with time and expire on a fixed day."],
      ["Position sizing", "A good thesis can still fail if the order is too large, too late, or leaves no cash for the next opportunity."],
    ],
    finishTitle: "When the timeline ends",
    finish: "The Reveal opens the blind box: real identities, the historical period, final standings and every player's trade history. Agent activity stays clearly labeled.",
    tipsTitle: "A few useful habits",
    tips: ["Read the article, not only the headline.", "Watch net assets instead of cash alone.", "Check option expiry before buying.", "Keep liquidity for the next open.", "Treat Agent chatter as opinion, not privileged information."],
    back: "← Back",
  },
  zh: {
    eyebrow: "盲盒市场模拟游戏",
    title: "Stocker 玩法说明",
    intro: "观察行情、判断真假难辨的信息，在隐藏的历史真相揭晓前尽可能提高自己的净资产。",
    startTitle: "四步开始游戏",
    steps: [
      ["进入房间", "创建游戏，或使用邀请码加入朋友的房间。所有人准备好后，由房主启动时间线。"],
      ["盲盒交易", "所有公司都使用化名，真实公司和历史时期会一直隐藏到游戏结束。"],
      ["安排委托", "买卖委托会在下一个开盘时，以你现在还不知道的价格成交；成交前可以取消挂单。"],
      ["比拼净资产", "排行榜计算现金和投资价值，再减去负债。结束时净资产最高的玩家获胜。"],
    ],
    loopTitle: "核心循环",
    loop: [
      ["观察", "查看行情、K 线、公司资料、新闻事件链、媒体应验率和玩家论坛。"],
      ["判断", "从噪音里寻找信号。语气肯定的标题也可能是错的，不起眼的报道却可能影响明天。"],
      ["行动", "交易股票，谨慎使用贷款和期权，也可以付费获取信息或影响市场。"],
      ["竞争", "和朋友及 5 名 Agent 玩家同场竞争。Agent 会交易、进入排行榜，并偶尔在聊天中发言，所有行为都有 Agent 标识。"],
    ],
    newsTitle: "新闻是信号，不是事实",
    newsIntro: "新闻根据带有噪音的报道线索写成。不同媒体的可靠程度不同，传闻也可能和最终发生的事件方向相反。",
    news: [
      "打开新闻可以阅读全文，并判断它是否属于“传闻 → 报道 → 追踪”事件链。",
      "媒体近期应验率会比较过去报道方向和之后的行情，但它只是证据，不是保证。",
      "每日复盘只描述已经公开的价格变化，用来解释盘面，不预测下一交易日。",
      "“存疑”和“已查实操纵”是公开标识；你的调查结论只有自己能看见，而且也可能判断错误。",
    ],
    advancedTitle: "进阶行动",
    advanced: [
      ["造势", "付费投放看多或看空的传闻，从下一个交易日开始影响目标。重复造势效果会减弱，被查获则会罚款并公开曝光。"],
      ["调查", "付费调查一条已经发布的新闻。你会得到一份私有但并非绝对准确的结论，所有玩家都会看到该新闻已被标记为存疑。"],
      ["内幕消息", "购买某只标的明日最强信号的模糊提示。消息可能很弱、被噪音干扰，或者根本没有明显方向。"],
    ],
    riskTitle: "风险工具也会反噬",
    risk: [
      ["信用贷款", "借款能立刻增加现金，但负债会产生利息并降低净资产；突破债务上限会破产。"],
      ["期权", "看涨和看跌期权可以用有限权利金放大判断，但价值会随时间衰减，并在固定日期到期。"],
      ["仓位管理", "判断正确也可能因为仓位太大、入场太晚，或没有为下次开盘保留现金而失败。"],
    ],
    finishTitle: "时间线结束后",
    finish: "终局揭晓会打开盲盒：公布真实公司、历史时期、最终排名以及所有玩家的交易记录。Agent 的行为仍会清楚标注。",
    tipsTitle: "几个实用习惯",
    tips: ["不要只看标题，也要阅读正文。", "关注净资产，不要只看现金。", "买期权前先确认到期日。", "为下一个开盘保留流动性。", "把 Agent 发言当作观点，不要当作内幕消息。"],
    back: "← 返回",
  },
} as const;

export default function Docs() {
  const { lang } = useT();
  const location = useLocation();
  const c = copy[lang];
  const from = (location.state as { from?: string } | null)?.from ?? "/";

  return (
    <div>
      <div className="topbar docs-topbar">
        <Link className="brand" to="/"><em>●</em> Stocker</Link>
        <div className="spacer" />
        <LangSwitch />
      </div>
      <main className="wrap docs-page">
        <Link className="docs-back" to={from}>{c.back}</Link>
        <header className="docs-hero">
          <div className="docs-eyebrow">{c.eyebrow}</div>
          <h1>{c.title}</h1>
          <p>{c.intro}</p>
        </header>

        <section className="docs-section">
          <h2>{c.startTitle}</h2>
          <div className="docs-steps">
            {c.steps.map(([title, body], i) => (
              <article key={title} className="docs-step">
                <span className="num">{String(i + 1).padStart(2, "0")}</span>
                <h3>{title}</h3><p>{body}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="docs-section">
          <h2>{c.loopTitle}</h2>
          <div className="docs-card-grid">
            {c.loop.map(([title, body]) => <article key={title} className="docs-card"><h3>{title}</h3><p>{body}</p></article>)}
          </div>
        </section>

        <section className="docs-section docs-news-guide">
          <div><span className="docs-chip">NEWS</span><h2>{c.newsTitle}</h2><p>{c.newsIntro}</p></div>
          <ul>{c.news.map(item => <li key={item}>{item}</li>)}</ul>
        </section>

        <section className="docs-section">
          <h2>{c.advancedTitle}</h2>
          <div className="docs-card-grid three">
            {c.advanced.map(([title, body]) => <article key={title} className="docs-card accent"><h3>{title}</h3><p>{body}</p></article>)}
          </div>
        </section>

        <section className="docs-section">
          <h2>{c.riskTitle}</h2>
          <div className="docs-card-grid three">
            {c.risk.map(([title, body]) => <article key={title} className="docs-card risk"><h3>{title}</h3><p>{body}</p></article>)}
          </div>
        </section>

        <section className="docs-section docs-finish">
          <div><span className="docs-chip">REVEAL</span><h2>{c.finishTitle}</h2><p>{c.finish}</p></div>
          <div><h3>{c.tipsTitle}</h3><ul>{c.tips.map(tip => <li key={tip}>{tip}</li>)}</ul></div>
        </section>
      </main>
    </div>
  );
}
