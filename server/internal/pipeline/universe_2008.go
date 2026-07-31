package pipeline

// gfc2008FetchSpecs: the 2008-era (gfc-2008) contribution to `pipeline
// fetch`'s download list. Shared symbols (aapl/xom/wmt/mcd/spx) are listed
// again here — even though universe_dotcom.go/universe_1987.go already list
// them with the same (or a wider) From/To — for the same documentation
// reason given in universe_1987.go/universe_1972.go: it records which
// universes actually consume each file. Shared symbols keep IDENTICAL
// From/To values across every universe.go that lists them, so
// `cmd/pipeline`'s FetchSpec dedup (first-seen-by-Name, across Universes()
// in sorted id order) never depends on which universe happens to be
// processed first.
//
// gs/ms/jpm/bac/c/wfc/aig/len/gld/cvx/dji are this universe's own symbols,
// fetched during plan-5 Task 3 ahead of this universe's existence.
//
// dji's From is 1992-01-01 (not the brief's aspirational 1970 start): Yahoo's
// chart API has no daily ^DJI history before 1992-01-02 no matter how early
// period1 is set (verified live in plan-5 Task 3 against several period1
// values spanning 1970-1985 — every one returns the same first bar). That
// floor is well before this scenario's 2006-10..2009-12 window, so G18 (道琼斯
// 工业指数) uses real data with no anchors. Unlike this universe, crash-1987
// and nifty-1972 had to drop their DJI instruments entirely because ^DJI's
// 1992 start date lies outside their historical windows (1986-1988 and
// 1972-1975 respectively).
var gfc2008FetchSpecs = []FetchSpec{
	{Name: "aapl", Symbol: "AAPL", From: "1985-06-01", To: "2010-06-30"},
	{Name: "xom", Symbol: "XOM", From: "1970-06-01", To: "2010-06-30"},
	{Name: "wmt", Symbol: "WMT", From: "1985-06-01", To: "2010-06-30"},
	{Name: "mcd", Symbol: "MCD", From: "1970-06-01", To: "2010-06-30"},
	{Name: "spx", Symbol: "^GSPC", From: "1970-06-01", To: "2010-06-30"},
	// This universe's own symbols, moved off pendingFetchSpecs.
	{Name: "dji", Symbol: "^DJI", From: "1992-01-01", To: "2010-06-30"},
	{Name: "gs", Symbol: "GS", From: "2006-04-01", To: "2010-06-30"},
	{Name: "ms", Symbol: "MS", From: "2006-04-01", To: "2010-06-30"},
	{Name: "jpm", Symbol: "JPM", From: "2006-04-01", To: "2010-06-30"},
	{Name: "bac", Symbol: "BAC", From: "2006-04-01", To: "2010-06-30"},
	{Name: "c", Symbol: "C", From: "2006-04-01", To: "2010-06-30"},
	{Name: "wfc", Symbol: "WFC", From: "2006-04-01", To: "2010-06-30"},
	{Name: "aig", Symbol: "AIG", From: "2006-04-01", To: "2010-06-30"},
	{Name: "len", Symbol: "LEN", From: "2006-04-01", To: "2010-06-30"},
	{Name: "gld", Symbol: "GLD", From: "2006-04-01", To: "2010-06-30"},
	{Name: "cvx", Symbol: "CVX", From: "2006-04-01", To: "2010-06-30"},
}

// gfc2008Universe: 2006-10 ~ 2009-12, the financial-crisis era. G03 (LEH) and
// G04 (BSC) are reconstructed from the brief's anchor tables (hard data,
// transcribed verbatim) — both are the fidelity stress case this universe
// exists for: a bankrupt-to-zero penny-stock tail (G03) and a
// takeover-consideration tail modeled as post-acquisition value rather than
// a traded price (G04, RealName carries （对价模拟）accordingly, see its
// Anchors comment below). Every other instrument is real Yahoo data; C's raw
// series is 2011-reverse-split-adjusted (~10x nominal, per Task 3's note) —
// this is purely cosmetic since build.go's levelScale band-clamps every
// baseline's day-0 close into [2, 500] anyway, so it is left as-is rather
// than "corrected".
//
// Every anchor date below (and every key-window boundary) was verified
// against rawdata/spx.csv (this universe's MarketProxy/calendar source) to
// already be a trading day — no shifts were needed.
var gfc2008Universe = ScenarioUniverse{
	ScenarioID:  "gfc-2008",
	Name:        "2008 金融危机",
	NameEn:      "2008 Financial Crisis",
	RealPeriod:  "2006-10 ~ 2009-12",
	EraHint:     "类似信贷与地产狂欢滑向系统性金融危机的年代",
	WindowStart: "2006-10-02",
	WindowEnd:   "2009-12-31",
	MarketProxy: "G17",
	FetchSpecs:  gfc2008FetchSpecs,
	Sectors: []SectorSpec{
		{ID: "IBNK", Name: "投资银行", NameEn: "Investment Banking"},
		{ID: "BANK", Name: "商业银行", NameEn: "Commercial Banking"},
		{ID: "INSR", Name: "保险", NameEn: "Insurance"},
		{ID: "HOME", Name: "地产建商", NameEn: "Homebuilders"},
		{ID: "DEFC", Name: "防御消费", NameEn: "Defensive Consumer"},
		{ID: "TECH", Name: "科技", NameEn: "Technology"},
		{ID: "ENGY", Name: "能源", NameEn: "Energy"},
	},
	Macros: []SectorSpec{
		{ID: "GOLD", Name: "黄金", NameEn: "Gold"},
		{ID: "OIL", Name: "原油", NameEn: "Crude Oil"},
		{ID: "RATE", Name: "利率", NameEn: "Interest Rates"},
	},
	KeyWindows: []DateWindow{
		{"2008-03-10", "2008-03-20", -1}, // 贝尔斯登周
		// 雷曼连锁崩塌: start moved from the brief's 2008-09-08 to
		// 2008-09-15 (round-2 fidelity fix, task-6-report.md "Round 2: G08
		// extremum") — 2008-09-15 is itself the real Lehman bankruptcy
		// filing date (verified against rawdata/spx.csv's trading calendar,
		// day index 491), so the window still opens exactly on the event
		// the label names; it now excludes only the preceding week's
		// pre-bankruptcy rumor jitters (2008-09-08..09-12), not any of the
		// collapse itself. That preceding week's negative-direction bias
		// (engine/shocks.go's pFlipInWin) was compounding into the shared
		// MKT/BANK factor state by the time of the real, singular
		// mid-window rally G08 (WFC) had on 2008-09-19 (the SEC
		// short-selling-ban short-covering spike, day 495 — a genuine,
		// documented historical event, not a data error) — suppressing
		// that display day just enough to fail fidelity.go's twin-extremum
		// value tolerance on some seeds (measured: calibration seed 102,
		// ratio 0.802 against the required 0.82). End (2008-10-10) is
		// unchanged, preserving the full TARP/October-crash aftermath.
		{"2008-09-15", "2008-10-10", -1},
		{"2009-03-02", "2009-03-31", 1}, // 绝望底反转
	},
	Instruments: []InstrumentSpec{
		{ID: "G01", Raw: "gs", Sector: "IBNK",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Sovereign Capital Partners", Aliases: []string{"Sovereign Capital Partners", "Kingsley Capital Partners", "Ironclad Securities"}, Desc: "投行之王，自营交易机器", RealName: "Goldman Sachs",
				DescEn:     "The king of investment banks, a proprietary-trading machine",
				Business:   "顶级投行里最会自营做单的一家，客户业务和自营交易的边界一向暧昧。",
				BusinessEn: "The most skilled proprietary trader among the top investment banks; the line between client business and prop trading has always been blurry.",
				Bull:       "别人恐惧时它总能先一步调转船头，交易台的嗅觉是全行业的传说。",
				BullEn:     "When others are fearful, it always turns the ship first; its trading desk's nose is the industry's legend.",
				Bear:       "自营交易赚得多，亏得也能一样快；当整个行业信任崩塌，谁的账本都经不起放大镜。",
				BearEn:     "Prop trading earns fast and loses just as fast; when trust across the industry collapses, no one's books survive a magnifying glass."}},
		{ID: "G02", Raw: "ms", Sector: "IBNK",
			ExtraBeta: map[string]float64{"SENT": 0.75, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Whitfield Securities", Aliases: []string{"Whitfield Securities", "Whitehall Securities", "Dunbar Capital"}, Desc: "老牌白鞋投行", RealName: "Morgan Stanley",
				DescEn:     "A venerable white-shoe investment bank",
				Business:   "百年积累下来的白鞋投行招牌，并购顾问费与自营交易并重的双引擎。",
				BusinessEn: "A white-shoe name built over a century, with twin engines of M&A advisory fees and proprietary trading.",
				Bull:       "招牌够老，关系网够深，大客户的并购单子永远绕不开它。",
				BullEn:     "The name is old enough and the network deep enough that big clients' M&A mandates never bypass it.",
				Bear:       "若高杠杆的同行中倒下一家，市场对'谁是下一个'的猜测本身就足以压垮下一家投行。",
				BearEn:     "If one highly leveraged peer falls, the market's guessing game of 'who's next' is itself enough to crush the next bank."}},
		{ID: "G03", Raw: "", Sector: "IBNK",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.5, "GOLD": 0.05},
			// The 2006-10..2008-01 segments (Sigma 0.012, below the 0.025
			// default) are a round-1 fidelity-gate fix, not brief data: the
			// brief only marks Sigma on the post-bankruptcy tail (see
			// below). At default noise, the 74->84->76->62 pre-crisis hump's
			// underlying drift (~0.15%/day) is far smaller than the 2.5%/day
			// default noise, leaving the segment's local high (~2007-02)
			// statistically indistinguishable from a dozen nearby days —
			// exactly the "twin extremum" ambiguity fidelity.go's
			// twinExtremumOK exists for, except here the ambiguity is severe
			// enough (values 15-25% below the nominal peak within days of
			// it) that engine-seed noise on top pushes the displayed argmax
			// outside both the slack AND the value-tolerance band (measured:
			// seed 5 fails "extremum moved 61 days (max=true)" at default
			// Sigma). Tightening these three calm pre-crisis segments to
			// 0.012 sharpens the hump into an unambiguous local peak without
			// touching a single anchor price, and has negligible effect on
			// G03's overall return stddev (dominated by the tail's
			// multi-hundred-percent single-day jumps) — see task-6-report.md
			// "Round 1: G03 extremum".
			Anchors: []Anchor{
				{"2006-10-02", 74, 0}, {"2007-02-01", 84, 0.012}, {"2007-06-01", 76, 0.012},
				{"2008-01-02", 62, 0.012}, {"2008-03-17", 32, 0}, {"2008-06-02", 33, 0},
				{"2008-09-02", 16, 0}, {"2008-09-09", 7.8, 0}, {"2008-09-12", 3.65, 0},
				{"2008-09-15", 0.21, 0.25}, {"2008-10-01", 0.10, 0.18},
				{"2009-03-02", 0.05, 0.15}, {"2009-12-31", 0.032, 0.15},
			},
			Dossier: Dossier{Alias: "Hartwell Investment Bank", Aliases: []string{"Hartwell Investment Bank", "Colossus Investment Bank", "Vesper Capital Group"}, Desc: "债券起家的百年投行", RealName: "Lehman Brothers（重建）",
				DescEn:     "A century-old investment bank born in bonds",
				Business:   "债券承销起家的百年投行，这些年最赚钱的部门是把千千万万笔房贷打包成证券再卖出去。",
				BusinessEn: "A century-old investment bank that grew up underwriting bonds; these days its most profitable desk packages millions of mortgages into securities and sells them on.",
				Bull:       "熬过一个半世纪所有危机的公司还会怕这一次？杠杆是利润的放大器，管理层说流动性充足。",
				BullEn:     "Would a firm that survived a century and a half of crises fear this one? Leverage is a profit amplifier, and management says liquidity is ample.",
				Bear:       "三十倍杠杆意味着资产跌百分之三就资不抵债；当抵押品是别人不想再要的东西，'充足'两个字撑不过一个周末。",
				BearEn:     "Thirty times leverage means a three percent asset decline wipes out equity; when the collateral is something nobody wants anymore, the word 'ample' won't survive a weekend."}},
		{ID: "G04", Raw: "", Sector: "IBNK",
			ExtraBeta: map[string]float64{"SENT": 0.85, "RATE": -0.5, "GOLD": 0.05},
			// 2008-06 之后（2008-11-20 起的三个锚点）按换股对价价值模拟——真实交易在
			// 2008-05-30 之后的收购要约价附近就已停止波动，其"价格"此后实际是并购
			// 换股比例隐含的对价价值，而非公开市场成交价；RealName 已标注
			// （对价模拟），揭晓页会如实呈现。
			Anchors: []Anchor{
				{"2006-10-02", 138, 0}, {"2007-01-16", 168, 0}, {"2007-08-01", 115, 0},
				{"2008-01-02", 85, 0}, {"2008-03-13", 57, 0}, {"2008-03-17", 4.8, 0},
				{"2008-03-24", 10.9, 0}, {"2008-05-30", 9.9, 0},
				{"2008-11-20", 5.2, 0.05}, {"2009-03-09", 3.9, 0.05},
				{"2009-06-01", 7.4, 0.04}, {"2009-12-31", 9.0, 0.04},
			},
			Dossier: Dossier{Alias: "Stonebridge Securities", Aliases: []string{"Stonebridge Securities", "Narrowgate Securities", "Crossmargin Securities"}, Desc: "按揭证券化的先锋", RealName: "Bear Stearns（重建·后段为对价模拟）",
				DescEn:     "The pioneer of mortgage securitization",
				Business:   "最早也最深地把按揭贷款打包成证券卖给全世界的投行，旗下对冲基金全押在这类资产上。",
				BusinessEn: "The earliest and deepest investment bank in packaging mortgages into securities sold worldwide; its own hedge funds are all-in on such assets.",
				Bull:       "华尔街最精明的债券交易台之一，这点风浪见得多了。",
				BullEn:     "One of the shrewdest bond desks on the street; it has seen far worse storms.",
				Bear:       "两只自家旗舰对冲基金已经清零，杠杆和信任一样，一旦出现裂缝就会一夜之间全部抽走。",
				BearEn:     "Two of its own flagship hedge funds have already been wiped out; leverage, like trust, disappears overnight once a crack appears."}},
		{ID: "G05", Raw: "jpm", Sector: "BANK",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Metropolitan Bank Corp", Aliases: []string{"Metropolitan Bank Corp", "Anchor National Bank", "Vaultline Bank Corp"}, Desc: "大而稳的全能银行", RealName: "JPMorgan Chase",
				DescEn:     "A big, steady universal bank",
				Business:   "存贷、投行、信用卡业务一应俱全的全能银行，资产负债表是同业里最厚实的一家。",
				BusinessEn: "A universal bank with deposits and lending, investment banking, and credit cards all under one roof; its balance sheet is the thickest among peers.",
				Bull:       "别人资金链紧张时，账上现金最多的那家反而能低价捡便宜货。",
				BullEn:     "When others face funding squeezes, the one with the most cash on hand gets to pick up bargains on the cheap.",
				Bear:       "全能银行意味着哪条业务线出问题都会牵连全身，规模大不等于隔离得干净。",
				BearEn:     "Universal means a problem in any business line drags down the whole; being big is not the same as being firewalled."}},
		{ID: "G06", Raw: "bac", Sector: "BANK",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Heartland National Bank", Aliases: []string{"Heartland National Bank", "Mainstreet National Bank", "Branchline Bank Corp"}, Desc: "零售网点最多的巨无霸", RealName: "Bank of America",
				DescEn:     "The behemoth with the most branches",
				Business:   "遍布全国的零售网点是它的护城河，靠吸储和信用卡业务赚稳定利差。",
				BusinessEn: "A nationwide branch network is its moat, earning steady spreads from deposits and credit cards.",
				Bull:       "网点多、储户多，负债端比同行便宜，逆势抄底同行资产的筹码最足。",
				BullEn:     "More branches, more depositors, cheaper funding than peers — the best-positioned balance sheet to buy rivals' assets at the bottom.",
				Bear:       "为了扩张吞并了太多同行的烂账，消化不良的并购标的比想象中难消化。",
				BearEn:     "It has swallowed too many peers' bad books in the name of expansion; indigestible acquisitions are harder to digest than imagined."}},
		{ID: "G07", Raw: "c", Sector: "BANK",
			ExtraBeta: map[string]float64{"SENT": 0.75, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Panorama Financial Group", Aliases: []string{"Panorama Financial Group", "Worldport Financial Group", "Transglobal Bank Corp"}, Desc: "全球扩张最激进的银行", RealName: "Citigroup",
				DescEn:     "The most aggressive bank in global expansion",
				Business:   "业务铺满全球每个时区的金融百货商店，表外的结构化投资工具规模惊人。",
				BusinessEn: "A financial department store spanning every time zone, with staggering off-balance-sheet structured investment vehicles.",
				Bull:       "全球化程度最深的银行，哪个市场景气都能分一杯羹。",
				BullEn:     "The most globalized bank of all; whichever market booms, it takes a share.",
				Bear:       "表外的东西迟早要搬回表内，越是扩张激进的巨无霸，账本里藏的雷往往越大。",
				BearEn:     "What's off the balance sheet comes back onto it sooner or later; the more aggressive the behemoth, the bigger the mines buried in its books."}},
		{ID: "G08", Raw: "wfc", Sector: "BANK",
			ExtraBeta: map[string]float64{"SENT": 0.65, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Homestead Bank & Trust", Aliases: []string{"Homestead Bank & Trust", "Prairie Bank & Trust", "Crossplains Mortgage Corp"}, Desc: "西部起家的房贷大行", RealName: "Wells Fargo",
				DescEn:     "A mortgage giant born out west",
				Business:   "西部起家的零售银行，房贷发放规模常年位居行业前列。",
				BusinessEn: "A retail bank born out west, perennially among the industry's largest mortgage originators.",
				Bull:       "只做看得懂的存贷业务，没沾染太多复杂衍生品，是同行里少有的'干净'账本。",
				BullEn:     "It sticks to plain-vanilla lending, untouched by complex derivatives — one of the few 'clean' books among peers.",
				Bear:       "房贷发放规模全行业最大，意味着房价一旦掉头，它的敞口谁都比不了。",
				BearEn:     "Being the industry's largest mortgage originator means that once home prices turn, nobody's exposure compares."}},
		{ID: "G09", Raw: "aig", Sector: "INSR",
			ExtraBeta: map[string]float64{"SENT": 0.85, "RATE": -0.5, "GOLD": 0.05},
			Dossier: Dossier{Alias: "Worldspan Insurance Group", Aliases: []string{"Worldspan Insurance Group", "Titan Insurance Group", "Century Assurance Group"}, Desc: "保险帝国与它神秘的衍生品部门", RealName: "AIG",
				DescEn:     "An insurance empire and its mysterious derivatives arm",
				Business:   "全球最大的保险帝国，但利润引擎藏在一个几百人的衍生品部门——他们为天量的债券违约风险卖了保险。",
				BusinessEn: "The world's largest insurance empire, but its profit engine hides in a derivatives unit of a few hundred people — they sold insurance on mountains of bond defaults.",
				Bull:       "百年一遇的违约潮才能伤到它，而它的精算师说那是不可能事件。",
				BullEn:     "Only a once-in-a-century wave of defaults could hurt it, and its actuaries say that's impossible.",
				Bear:       "卖的是'不可能事件'的保险，收的是确定的小钱，赌上的是整个集团；不可能事件只需要发生一次。",
				BearEn:     "It sells insurance on 'impossible events', collects small certain premiums, and bets the entire group; the impossible only needs to happen once."}},
		{ID: "G10", Raw: "len", Sector: "HOME",
			ExtraBeta: map[string]float64{"SENT": 0.75, "RATE": -0.5},
			Dossier: Dossier{Alias: "Sunhaven Homes", Aliases: []string{"Sunhaven Homes", "Lumberlane Homes", "Frameline Builders"}, Desc: "全国性住宅建商", RealName: "Lennar",
				DescEn:     "A nationwide homebuilder",
				Business:   "全国铺开的住宅建筑商，靠土地储备和在建工程滚动开发。",
				BusinessEn: "A nationwide homebuilder rolling development forward on land banks and work in progress.",
				Bull:       "刚需永远存在，房子盖得越快，账面利润确认得越快。",
				BullEn:     "Housing demand always exists; the faster homes are built, the faster book profits are recognized.",
				Bear:       "土地和在建工程都是重资产，一旦成屋卖不动，存货就从资产变成沉重的负担。",
				BearEn:     "Land and work in progress are heavy assets; once finished homes stop selling, inventory turns from asset to burden."}},
		{ID: "G11", Raw: "wmt", Sector: "DEFC",
			ExtraBeta: map[string]float64{"SENT": 0.1},
			Dossier: Dossier{Alias: "Ledgemont Retail Group", Aliases: []string{"Ledgemont Retail Group", "Saveright Stores", "Bulkline Retail"}, Desc: "乡镇起家的零售之王", RealName: "Walmart",
				DescEn:     "The retail king from small-town roots",
				Business:   "天天低价的连锁超市帝国，供应链效率碾压一切同行。",
				BusinessEn: "An everyday-low-price supermarket empire whose supply-chain efficiency crushes every rival.",
				Bull:       "钱包越紧，消费者越往它这儿挤；日用品的需求不会因为恐慌而消失。",
				BullEn:     "The tighter wallets get, the more consumers crowd into its aisles; demand for daily necessities doesn't vanish in a panic.",
				Bear:       "低毛利模式对成本敏感，规模已经大到很难再靠开店提速增长。",
				BearEn:     "The low-margin model is cost-sensitive, and the scale is already so large that opening more stores can hardly speed up growth."}},
		{ID: "G12", Raw: "mcd", Sector: "DEFC",
			ExtraBeta: map[string]float64{"SENT": 0.15},
			Dossier: Dossier{Alias: "Quickbite Restaurants", Aliases: []string{"Quickbite Restaurants", "Counterserve Restaurants", "Fastlane Food Group"}, Desc: "全球连锁快餐之王", RealName: "McDonald's",
				DescEn:     "The king of global fast-food chains",
				Business:   "标准化流程复制到全球的连锁快餐，加盟费与地产租金是真正的利润引擎。",
				BusinessEn: "A standardized fast-food formula replicated worldwide; franchise fees and real-estate rents are the true profit engine.",
				Bull:       "手头紧的时候，人们从高档餐厅转向它的柜台，逆周期属性写在生意模式里。",
				BullEn:     "When money gets tight, people trade down from fancy restaurants to its counters — the countercyclical trait is written into the business model.",
				Bear:       "同店销售增速终会撞天花板，全球扩张也要看当地消费者兜里还有没有钱。",
				BearEn:     "Same-store sales growth will eventually hit a ceiling, and global expansion still depends on whether local consumers have money left."}},
		{ID: "G13", Raw: "aapl", Sector: "TECH",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.2},
			Dossier: Dossier{Alias: "Copperleaf Technologies", Aliases: []string{"Copperleaf Technologies", "Sleekline Technologies", "Novaridge Computers"}, Desc: "刚发布革命性手机的电脑厂", RealName: "Apple",
				DescEn:     "A computer maker that just launched a revolutionary phone",
				Business:   "从个人电脑起家，靠一部刚问世的触屏手机打开了全新的产品线。",
				BusinessEn: "Born in personal computers, it opened an entirely new product line with a just-launched touchscreen phone.",
				Bull:       "新产品品类颠覆整个手机行业，忠实用户愿意为设计和体验多付钱。",
				BullEn:     "The new product category upends the entire phone industry, and loyal users happily pay extra for design and experience.",
				Bear:       "新品类刚起步就撞上消费者兜里的钱大幅缩水，非必需的高价电子产品最先被砍预算。",
				BearEn:     "The new category has barely started and already collides with shrinking consumer wallets; pricey discretionary electronics are the first budget cut."}},
		{ID: "G14", Raw: "xom", Sector: "ENGY",
			ExtraBeta: map[string]float64{"SENT": 0.1, "OIL": 0.7, "GOLD": 0.1},
			Dossier: Dossier{Alias: "Vanguard Petroleum", Aliases: []string{"Vanguard Petroleum", "Blackgold Petroleum", "Redrock Petroleum"}, Desc: "全球油气巨轮", RealName: "ExxonMobil",
				DescEn:     "A global oil-and-gas supertanker",
				Business:   "从油井到加油站的全产业链油气巨头，规模冠绝全球同行。",
				BusinessEn: "A fully integrated oil-and-gas giant from wellhead to gas station, scale unmatched among global peers.",
				Bull:       "油价一度冲上历史高位，现金流丰厚到能在别人缺钱时逆势加仓。",
				BullEn:     "Oil once spiked to historic highs, leaving cash flow rich enough to add positions while everyone else is starved for money.",
				Bear:       "全球经济一旦真的熄火，最先被砍的就是能源需求，油价从高位摔下来会摔得很重。",
				BearEn:     "If the global economy truly stalls, energy demand is the first thing cut, and oil prices falling from highs fall hard."}},
		{ID: "G15", Raw: "cvx", Sector: "ENGY",
			ExtraBeta: map[string]float64{"SENT": 0.1, "OIL": 0.7, "GOLD": 0.1},
			Dossier: Dossier{Alias: "Blueridge Energy", Aliases: []string{"Blueridge Energy", "Pumpjack Energy", "Cordillera Energy"}, Desc: "全产业链油气巨头", RealName: "Chevron",
				DescEn:     "A fully integrated oil-and-gas major",
				Business:   "上游勘探开采到下游炼化销售通吃的综合性油气公司。",
				BusinessEn: "An integrated oil-and-gas company spanning upstream exploration to downstream refining and marketing.",
				Bull:       "资源在手，定价权在手，恐慌时期硬资产比任何承诺都可信。",
				BullEn:     "Resources in hand, pricing power in hand; in panicky times hard assets are more credible than any promise.",
				Bear:       "需求端一旦全面萎缩，再大的资源储备也换不来买家愿意付的价钱。",
				BearEn:     "Once demand shrinks across the board, no reserve, however vast, fetches the price buyers are willing to pay."}},
		{ID: "G16", Raw: "gld", Sector: "",
			ExtraBeta: map[string]float64{"GOLD": 0.9, "SENT": -0.15},
			Dossier: Dossier{Alias: "HardAsset Trust", Aliases: []string{"HardAsset Trust", "Bullionback Trust", "Aurum Reserve"}, Desc: "黄金现货支持的基金", RealName: "SPDR Gold",
				DescEn:     "A fund backed by physical gold",
				Business:   "每一份基金份额背后都有对应的实物黄金现货托管，买卖它就是买卖黄金本身。",
				BusinessEn: "Every fund share is backed by physical gold bullion in custody; trading it is trading gold itself.",
				Bull:       "当整个金融体系的承诺都开始让人怀疑，摸得着的黄金重新变得性感。",
				BullEn:     "When every promise in the financial system starts to look doubtful, tangible gold becomes sexy again.",
				Bear:       "黄金不生息也不分红，一旦恐慌退潮，资金会毫不犹豫地转回生息资产。",
				BearEn:     "Gold pays no interest and no dividend; once the panic ebbs, money rotates back into yield-bearing assets without hesitation."}},
		{ID: "G17", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.5},
			Dossier: Dossier{Alias: "National 500 Index", Aliases: []string{"National 500 Index", "Federation 500 Index", "Monument 500 Index"}, Desc: "五百家大公司指数", RealName: "S&P 500 指数",
				DescEn:     "An index of five hundred large companies",
				Business:   "五百家大公司的市值加权组合，一笔交易买下整个经济的横截面。",
				BusinessEn: "A market-cap-weighted basket of five hundred large companies — a cross-section of the whole economy in one trade.",
				Bull:       "不赌个股不赌行业，赌整体经济终将复苏。",
				BullEn:     "Bet on no stock and no sector; bet that the economy will recover in the end.",
				Bear:       "金融股权重不低，当金融体系本身摇晃，指数里没有真正安全的角落。",
				BearEn:     "Financials carry no small weight; when the financial system itself wobbles, there is no truly safe corner in the index."}},
		{ID: "G18", Raw: "dji", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.5},
			Dossier: Dossier{Alias: "Heritage Bluechip Index", Aliases: []string{"Heritage Bluechip Index", "Smokestack 30 Index", "Oldline Industrials Index"}, Desc: "三十家巨头指数", RealName: "道琼斯工业指数",
				DescEn:     "An index of thirty giants",
				Business:   "三十家蓝筹巨头按价格加权组成的古老指数，成分股换过多次但招牌不变。",
				BusinessEn: "An ancient price-weighted index of thirty blue-chip giants; the constituents have changed many times, but the name endures.",
				Bull:       "剩下的都是各行各业活下来的巨头，经济复苏第一波涨的通常是它们。",
				BullEn:     "What remains are the survivors of every industry; they're usually the first to rise in a recovery.",
				Bear:       "价格加权的老规则让个别高价股的涨跌左右全局，未必真实反映整体经济。",
				BearEn:     "The old price-weighting rule lets a few high-priced stocks sway the whole, not necessarily reflecting the real economy."}},
	},
}

func init() {
	Register(&gfc2008Universe)
}
