package httpapi

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/toddzheng/stocker/server/internal/store"
)

// Public battle-report page. No auth: the room's share_token acts as a
// read-only capability. Blind-box rules still apply — the historical era and
// real instrument identities stay hidden until the room ends; only net-worth
// standings and progress are ever shown (same projection as the room
// leaderboard, holdings never leave the server).
var shareTmpl = template.Must(template.New("share").Funcs(template.FuncMap{
	"spark": sparklineSVG,
}).Parse(`<!doctype html>
<html lang="{{.HTMLLang}}">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<meta name="color-scheme" content="dark" />
<title>{{.Title}}</title>
<meta name="description" content="{{.OGDesc}}" />
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
<meta property="og:type" content="website" />
<meta property="og:site_name" content="Stocker" />
<meta property="og:title" content="{{.Title}}" />
<meta property="og:description" content="{{.OGDesc}}" />
<meta property="og:url" content="{{.OGURL}}" />
<meta property="og:image" content="{{.BaseURL}}/og.png" />
<meta name="twitter:card" content="summary_large_image" />
<meta name="twitter:image" content="{{.BaseURL}}/og.png" />
{{if .AutoRefresh}}<meta http-equiv="refresh" content="120" />{{end}}
<style>
:root{--bg:#0c0d10;--card:#15171c;--line:#23262d;--ink:#f6f7f9;--ink2:#8a919e;--ink3:#565c66;--up:#00c805;--down:#ff5000;--up-soft:rgba(0,200,5,.12);--down-soft:rgba(255,80,0,.12)}
*{box-sizing:border-box;margin:0}
body{background:var(--bg);color:var(--ink);font-family:-apple-system,"SF Pro Text","PingFang SC","Hiragino Sans GB","Noto Sans SC",sans-serif;line-height:1.6}
a{color:inherit}
.topbar{display:flex;align-items:center;gap:14px;padding:12px 20px;border-bottom:1px solid var(--line);background:rgba(12,13,16,.86)}
.brand{font-weight:700;text-decoration:none}
.brand em{font-style:normal;color:var(--up)}
.spacer{flex:1}
.cta{display:inline-block;border-radius:999px;padding:8px 22px;background:var(--up);color:#04140a;font-size:.86rem;font-weight:700;text-decoration:none}
.wrap{max-width:720px;margin:0 auto;padding:34px 20px 60px}
.tag-eyebrow{color:var(--up);font-size:.72rem;font-weight:750;letter-spacing:.13em;text-transform:uppercase}
h1{margin:10px 0 8px;font-size:clamp(1.7rem,4.5vw,2.6rem);line-height:1.1;letter-spacing:-.02em}
.sub{color:var(--ink2);font-size:.92rem}
.progress{height:6px;background:var(--card);border-radius:6px;margin:20px 0 6px;overflow:hidden}
.progress i{display:block;height:100%;background:var(--up);border-radius:6px}
.progress-meta{color:var(--ink3);font-size:.76rem}
.lb{margin-top:30px;border:1px solid var(--line);border-radius:16px;overflow:hidden}
.lb-head{padding:14px 18px;border-bottom:1px solid var(--line);color:var(--ink2);font-size:.74rem;font-weight:700;letter-spacing:.1em;text-transform:uppercase;background:var(--card)}
.row{display:grid;grid-template-columns:26px minmax(0,1fr) 110px 92px;align-items:center;gap:12px;padding:13px 18px;border-bottom:1px solid var(--line)}
.row:last-child{border-bottom:none}
.row.first{background:rgba(0,200,5,.05)}
.rank{color:var(--ink3);font-size:.85rem;font-variant-numeric:tabular-nums}
.who{font-weight:600;font-size:.95rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.badge{margin-left:6px;padding:1px 6px;border-radius:999px;font-size:.6rem;font-weight:700;letter-spacing:.04em;color:var(--up);background:var(--up-soft);vertical-align:2px}
.badge.bust{color:var(--down);background:var(--down-soft)}
.spark svg{display:block}
.ret{text-align:right;font-weight:650;font-size:.95rem;font-variant-numeric:tabular-nums}
.ret.up{color:var(--up)}.ret.down{color:var(--down)}
.empty{padding:34px 18px;text-align:center;color:var(--ink2);font-size:.9rem}
.final-cta{margin-top:36px;text-align:center;padding:44px 20px;border:1px solid var(--line);border-radius:18px;background:linear-gradient(145deg,#12151b,#0d0f13)}
.final-cta h2{font-size:1.3rem;letter-spacing:-.01em}
.final-cta p{color:var(--ink2);font-size:.88rem;margin:10px auto 22px;max-width:420px}
footer{margin-top:36px;text-align:center;color:var(--ink3);font-size:.74rem}
footer a{color:var(--ink2);text-decoration:none}
</style>
</head>
<body>
<header class="topbar">
  <a class="brand" href="/"><em>●</em> Stocker</a>
  <div class="spacer"></div>
  <a class="cta" href="/">{{.S.CTAPlay}}</a>
</header>
<main class="wrap">
  <div class="tag-eyebrow">{{.S.BattleReport}}</div>
  <h1>{{.RoomName}}</h1>
  <p class="sub">{{.EraLine}}</p>
  <div class="progress"><i style="width:{{.ProgressPct}}%"></i></div>
  <div class="progress-meta">{{.ProgressLine}}</div>

  <section class="lb">
    <div class="lb-head">{{.S.Leaderboard}}</div>
    {{if .Rows}}
      {{range $i, $r := .Rows}}
      <div class="row{{if and (eq $i 0) (not $r.Bankrupt)}} first{{end}}">
        <span class="rank">{{$r.Rank}}</span>
        <span class="who">{{$r.Name}}{{if $r.IsAgent}}<span class="badge">AGENT</span>{{end}}{{if $r.Bankrupt}}<span class="badge bust">{{$.S.Bankrupt}}</span>{{end}}</span>
        <span class="spark">{{spark $r.Curve $r.ReturnPct}}</span>
        <span class="ret {{if ge $r.ReturnPct 0.0}}up{{else}}down{{end}}">{{$r.ReturnText}}</span>
      </div>
      {{end}}
    {{else}}
      <div class="empty">{{.S.Empty}}</div>
    {{end}}
  </section>

  <div class="final-cta">
    <h2>{{.S.CTATitle}}</h2>
    <p>{{.S.CTABody}}</p>
    <a class="cta" href="/">{{.S.CTAPlay}}</a>
  </div>

  <footer><a href="/">Stocker</a> · {{.S.Tagline}}</footer>
</main>
</body>
</html>`))

type shareStrings struct {
	BattleReport, Leaderboard, Bankrupt, Empty string
	CTATitle, CTABody, CTAPlay, Tagline        string
}

var shareStringsZH = shareStrings{
	BattleReport: "Stocker 战报", Leaderboard: "净资产排行", Bankrupt: "已破产",
	Empty: "房间还没有开局，开局后这里会显示实时战况。",
	CTATitle: "和朋友来一局？", CTABody: "穿越回一段真实的市场历史，在公司化名的盲盒市场里交易博弈，终局揭晓真相。",
	CTAPlay: "免费开局", Tagline: "多人盲盒股市游戏",
}

var shareStringsEN = shareStrings{
	BattleReport: "Stocker battle report", Leaderboard: "Net-worth standings", Bankrupt: "Bankrupt",
	Empty: "This room has not started yet — live standings will appear here once the clock starts.",
	CTATitle: "Start a game with friends?", CTABody: "Travel back to a real slice of market history, trade a blind-box market of aliased companies, and face the final reveal.",
	CTAPlay: "Play free", Tagline: "multiplayer blind-box market",
}

type shareRow struct {
	Rank       int
	Name       string
	IsAgent    bool
	Bankrupt   bool
	ReturnPct  float64
	ReturnText string
	Curve      []int64
}

type sharePage struct {
	HTMLLang, Title, OGDesc, OGURL, BaseURL string
	S                                       shareStrings
	RoomName, EraLine, ProgressLine         string
	ProgressPct                             int
	Rows                                    []shareRow
	AutoRefresh                             bool
}

// sparklineSVG renders a player's daily net-worth curve as a tiny inline SVG.
// Input is server-generated numeric data only, so the markup is safe.
func sparklineSVG(curve []int64, returnPct float64) template.HTML {
	const w, h, pad = 110.0, 30.0, 2.0
	color := "#00c805"
	if returnPct < 0 {
		color = "#ff5000"
	}
	if len(curve) == 0 {
		return template.HTML(fmt.Sprintf(`<svg width="110" height="30" viewBox="0 0 %.0f %.0f"><line x1="0" y1="%.1f" x2="%.0f" y2="%.1f" stroke="%s" stroke-width="1.5" opacity=".6"/></svg>`, w, h, h/2, w, h/2, color))
	}
	lo, hi := curve[0], curve[0]
	for _, v := range curve {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := float64(hi - lo)
	if span == 0 {
		span = 1
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg width="110" height="30" viewBox="0 0 %.0f %.0f"><polyline fill="none" stroke="%s" stroke-width="1.5" points="`, w, h, color))
	for i, v := range curve {
		x := pad + float64(i)*(w-2*pad)/float64(max(len(curve)-1, 1))
		y := pad + (1-float64(v-lo)/span)*(h-2*pad)
		fmt.Fprintf(&b, "%.1f,%.1f ", x, y)
	}
	b.WriteString(`"/></svg>`)
	return template.HTML(b.String())
}

func shareBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	room, err := store.GetRoomByShareToken(r.Context(), s.DB, chi.URLParam(r, "token"))
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body style="background:#0c0d10;color:#8a919e;font-family:-apple-system,'PingFang SC',sans-serif;display:grid;place-items:center;min-height:100vh;margin:0"><div style="text-align:center"><div style="color:#00c805;font-weight:700;margin-bottom:8px">● Stocker</div><p>分享链接无效或房间已不存在。<br>This share link is invalid or the room no longer exists.</p><p style="margin-top:18px"><a href="/" style="color:#00c805;text-decoration:none">→ Stocker</a></p></div></body>`)
		return
	}
	curDay, ended, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	started := room.StartedAt != nil

	ss := shareStringsEN
	htmlLang := "en"
	if strings.Contains(r.Header.Get("Accept-Language"), "zh") {
		ss = shareStringsZH
		htmlLang = "zh-CN"
	}

	var rows []shareRow
	if started {
		lb, err := store.Leaderboard(r.Context(), s.DB, room, curDay)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		rows = make([]shareRow, 0, len(lb))
		for i, lr := range lb {
			name := lr.UsernameEn
			if htmlLang == "zh-CN" {
				name = lr.Username
			}
			pct := float64(lr.TotalCents-store.InitialCashCents) / float64(store.InitialCashCents)
			rows = append(rows, shareRow{
				Rank: i + 1, Name: name, IsAgent: lr.IsAgent, Bankrupt: lr.Bankrupt,
				ReturnPct: pct, ReturnText: fmt.Sprintf("%+.1f%%", pct*100), Curve: lr.Curve,
			})
		}
	}

	day := 0
	if started {
		day = min(curDay+1, room.Days)
	}
	progressPct := 0
	if started && room.Days > 0 {
		progressPct = day * 100 / room.Days
	}

	// Blind box: the era stays hidden until the timeline ends.
	eraLine := "An unknown slice of history"
	progressLine := "Waiting for the host to start the clock"
	ogDesc := "A Stocker room battle report."
	if htmlLang == "zh-CN" {
		eraLine = "一段尚未揭晓的历史"
		progressLine = "等待房主启动时间线"
		ogDesc = "一间 Stocker 房间的实时战报。"
	}
	if started {
		if htmlLang == "zh-CN" {
			progressLine = fmt.Sprintf("第 %d / %d 个交易日", day, room.Days)
		} else {
			progressLine = fmt.Sprintf("Trading day %d / %d", day, room.Days)
		}
		if len(rows) > 0 && !rows[0].Bankrupt {
			if htmlLang == "zh-CN" {
				ogDesc = fmt.Sprintf("第 %d/%d 天 · 当前领先：%s %s", day, room.Days, rows[0].Name, rows[0].ReturnText)
			} else {
				ogDesc = fmt.Sprintf("Day %d/%d · leader: %s %s", day, room.Days, rows[0].Name, rows[0].ReturnText)
			}
		}
	}
	if ended {
		var name, nameEn string
		if qerr := s.DB.QueryRow(r.Context(),
			`SELECT name, name_en FROM scenarios WHERE id = $1`, room.ScenarioID).Scan(&name, &nameEn); qerr == nil {
			eraLine = nameEn
			if htmlLang == "zh-CN" {
				eraLine = name
			}
		}
		if len(rows) > 0 {
			if htmlLang == "zh-CN" {
				ogDesc = fmt.Sprintf("终局揭晓 · %s · 冠军：%s %s", eraLine, rows[0].Name, rows[0].ReturnText)
			} else {
				ogDesc = fmt.Sprintf("Final reveal · %s · winner: %s %s", eraLine, rows[0].Name, rows[0].ReturnText)
			}
		}
		progressLine = eraLine
	}

	title := room.Name + " · Stocker 战报"
	if htmlLang != "zh-CN" {
		title = room.Name + " · Stocker battle report"
	}
	base := shareBaseURL(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_ = shareTmpl.Execute(w, sharePage{
		HTMLLang: htmlLang, Title: title, OGDesc: ogDesc,
		OGURL: base + r.URL.Path, BaseURL: base,
		S: ss, RoomName: room.Name, EraLine: eraLine,
		ProgressLine: progressLine, ProgressPct: progressPct,
		Rows: rows, AutoRefresh: started && !ended,
	})
}
