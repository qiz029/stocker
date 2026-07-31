package pipeline

// dotcomFetchSpecs: 16 surviving stocks, 2 indices, 2 macro proxies.
//
// Source: Yahoo Finance chart API (query1.finance.yahoo.com), not Stooq.
// Stooq now gates its entire site — including the plain homepage — behind
// a JavaScript proof-of-work anti-bot wall (unrelated to User-Agent or
// request rate), so a non-JS HTTP client cannot fetch real CSV data from
// it. Yahoo's chart API was verified reachable from this network and
// returns split-adjusted OHLC, so it replaces Stooq as the fetch source
// for this task; the on-disk CSV format and everything downstream of
// RawSeries is unchanged.
//
// The four dead companies (wcom/lu/nt/gblx) are anchor-reconstructed in
// reconstruct.go — free sources carry no delisted daily data.
//
// oil (crude, Stooq symbol cl.f) has no equivalent free Yahoo history for
// this era and is dropped from the fetch list per the pre-authorized
// macro-proxy contingency: the OIL factor keeps curated (non-fitted) beta
// values in Task 5 instead of a regression-fitted one.
//
// spx/ibm/hpq/aapl/ge/xom/wmt carry widened From/To windows (plan-5 Task
// 3): those seven symbols are also needed by crash-1987/nifty-1972/gfc-2008
// (Tasks 4-6), and rather than fetch a second file for the same company
// under a different name, one file per symbol covers the union of every
// era's need. Alignment slices its own scenario window at build time
// (see build.go), so the extra pre/post-window rows only cost repo bytes,
// not correctness — dotcom-2000's own fidelity/calibration gates are
// unaffected and re-verified after every widen (task-3-report.md).
//
// xom was widened a second time in Task 5: the original plan-5 Task 3 table
// only widened it to 1985-06-01 (covering crash-1987/gfc-2008), but
// nifty-1972's N13 (Ironshore Petroleum/Exxon) needs real data back to its 1972-01-03
// window start — a gap the Task 3 table missed (its "1972+1987" blue-chip
// group listed ko/mcd/dis/jnj/pg/mmm/cat but not xom). Verified live against
// Yahoo's chart API: XOM's firstTradeDate is 1962-01-02 (predecessor-ticker
// history is carried under the current symbol), so 1970-06-01 is a real,
// fetchable start — widened to match the ko/mcd/etc. blue-chip group
// instead of inventing anchors for an otherwise-real instrument.
var dotcomFetchSpecs = []FetchSpec{
	{Name: "msft", Symbol: "MSFT"}, {Name: "csco", Symbol: "CSCO"}, {Name: "intc", Symbol: "INTC"},
	{Name: "orcl", Symbol: "ORCL"},
	// ibm: union of 1970-06..1989-06 (1972+1987) and the original
	// 1998-06..2002-03 dotcom window → fetch the whole 1970-06..2002-03 span.
	{Name: "ibm", Symbol: "IBM", From: "1970-06-01", To: "2002-03-31"},
	// aapl/ge/xom/wmt: also needed from 1987 (and, for ge/xom/wmt, 2008)
	// onward → widen to 1985-06..2010-06 to cover every era at once.
	{Name: "aapl", Symbol: "AAPL", From: "1985-06-01", To: "2010-06-30"},
	{Name: "amzn", Symbol: "AMZN"}, {Name: "ebay", Symbol: "EBAY"}, {Name: "amd", Symbol: "AMD"},
	{Name: "qcom", Symbol: "QCOM"}, {Name: "txn", Symbol: "TXN"}, {Name: "adbe", Symbol: "ADBE"},
	// hpq: needed from 1987 (1985-06 margin) through the original dotcom window.
	{Name: "hpq", Symbol: "HPQ", From: "1985-06-01", To: "2002-03-31"},
	{Name: "ge", Symbol: "GE", From: "1985-06-01", To: "2010-06-30"},
	// xom: widened again in Task 5 (1970-06-01, not 1985-06-01) — see the
	// doc comment above this var for why.
	{Name: "xom", Symbol: "XOM", From: "1970-06-01", To: "2010-06-30"},
	{Name: "wmt", Symbol: "WMT", From: "1985-06-01", To: "2010-06-30"},
	{Name: "ndx", Symbol: "^NDX"},
	// spx: market proxy for dotcom-2000, also usable as a market/context
	// series for the other three eras → widen to the full 1970-06..2010-06 span.
	{Name: "spx", Symbol: "^GSPC", From: "1970-06-01", To: "2010-06-30"},
	// gold: no free Yahoo history for a bullion spot/future in this era;
	// ^XAU (PHLX Gold/Silver Sector index of gold/silver miners) is used
	// as the GOLD factor proxy instead.
	{Name: "gold", Symbol: "^XAU"},
	// us10y: ^TNX is the CBOE 10-Year Treasury Note yield index (yield in
	// index points, i.e. 10x the yield in percent), used as the US10Y
	// factor proxy.
	{Name: "us10y", Symbol: "^TNX"},
}

var dotcomUniverse = ScenarioUniverse{
	ScenarioID:    "dotcom-2000",
	Name:          "2000 互联网泡沫",
	NameEn:        "2000 Dot-Com Bubble",
	RealPeriod:    "1999-01 ~ 2001-12",
	EraHint:       "类似世纪之交科技股狂热",
	WindowStart:   "1999-01-04",
	WindowEnd:     "2001-12-31",
	MarketProxy:   "X22",
	FidelitySeeds: 12,
	FetchSpecs:    dotcomFetchSpecs,
	Sectors: []SectorSpec{
		{ID: "NET", Name: "网络设备", NameEn: "Network Equipment"},
		{ID: "TELCO", Name: "电信运营", NameEn: "Telecom Carriers"},
		{ID: "ECOM", Name: "电商门户", NameEn: "E-Commerce & Portals"},
		{ID: "CHIP", Name: "半导体", NameEn: "Semiconductors"},
		{ID: "SOFT", Name: "软件服务", NameEn: "Software & Services"},
		{ID: "HW", Name: "硬件整机", NameEn: "Hardware"},
		{ID: "OLD", Name: "传统经济", NameEn: "Old Economy"},
	},
	Macros: []SectorSpec{
		{ID: "GOLD", Name: "黄金", NameEn: "Gold"},
		{ID: "OIL", Name: "原油", NameEn: "Crude Oil"},
		{ID: "RATE", Name: "利率", NameEn: "Interest Rates"},
	},
	KeyWindows: []DateWindow{
		{"2000-03-10", "2000-05-26", -1}, // 见顶崩盘期: 火上浇油可以，泼冷水不行
		{"2000-11-08", "2001-01-05", -1}, // 二次下跌
		{"2001-09-17", "2001-09-28", -1}, // 重开市恐慌
	},
	Instruments: []InstrumentSpec{
		{ID: "X01", Raw: "msft", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "Northgate Systems", Aliases: []string{"Northgate Systems", "Lattice Software", "Windmere Systems"}, Desc: "桌面软件的垄断者", RealName: "Microsoft",
				DescEn:     "The monopolist of desktop software",
				Business:   "操作系统与办公套件的授权费像税收一样稳定，正把触角伸向服务器与互联网。",
				BusinessEn: "License fees for its operating system and office suite roll in as steadily as taxes, and it is now reaching into servers and the internet.",
				Bull:       "每台新电脑都要向它交税，现金堆成山，垄断者的定价权在任何时代都值钱。",
				BullEn:     "Every new PC pays it a toll; cash piles up like mountains, and a monopolist's pricing power is valuable in any era.",
				Bear:       "反垄断阴云笼罩，拆分传闻不断；互联网时代它更像追赶者而非引领者。",
				BearEn:     "Antitrust clouds hang over it and breakup rumors never stop; in the internet era it looks more like a follower than a leader."}},
		{ID: "X02", Raw: "csco", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.4},
			Dossier: Dossier{Alias: "Vantidge Networks", Aliases: []string{"Vantidge Networks", "Ironbridge Networks", "Tessera Networks"}, Desc: "网络设备巨头，泡沫叙事的旗手", RealName: "Cisco Systems",
				DescEn:     "Networking-equipment giant, flag-bearer of the bubble narrative",
				Business:   "路由器与交换机的绝对霸主，客户是整个新经济：运营商、门户、企业机房。",
				BusinessEn: "The undisputed king of routers and switches; its customers are the entire new economy: carriers, portals, and corporate data centers.",
				Bull:       "淘金潮里卖铲子的人——不管哪家网站赢，铲子都得从它这买。",
				BullEn:     "The one selling shovels in a gold rush — no matter which website wins, the shovels get bought from it.",
				Bear:       "客户都在烧风投的钱；融资一断，下游资本开支先于一切崩塌，而估值已透支十年。",
				BearEn:     "Its customers are all burning venture money; the moment funding stops, downstream capital spending collapses before everything else — and the valuation has already priced in a decade."}},
		{ID: "X03", Raw: "intc", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "Meridian Semiconductor", Aliases: []string{"Meridian Semiconductor", "Pinnacle Semiconductor", "Ardent Microsystems"}, Desc: "处理器行业的王者", RealName: "Intel",
				DescEn:     "King of the processor industry",
				Business:   "个人电脑与服务器处理器的双料霸主，制程领先一代就是护城河。",
				BusinessEn: "Dual champion of PC and server processors; a one-generation process lead is its moat.",
				Bull:       "上网热潮就是换机热潮，每一台新电脑的心脏都印着它的标。",
				BullEn:     "The rush to get online is a rush to replace PCs, and the heart of every new computer bears its logo.",
				Bear:       "半导体是周期行业，库存一旦掉头，'供不应求'三个月内变'砍单'。",
				BearEn:     "Semiconductors are a cyclical business; once inventories turn, 'supply shortage' becomes 'order cancellations' within three months."}},
		{ID: "X04", Raw: "orcl", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "Crownfield Data Systems", Aliases: []string{"Crownfield Data Systems", "Sable Data Systems", "Winterbrook Software"}, Desc: "企业数据库军火商", RealName: "Oracle",
				DescEn:     "Arms dealer of enterprise databases",
				Business:   "关系数据库的行业标准，电商潮里每个网站背后都要一套它的授权。",
				BusinessEn: "The industry standard in relational databases; behind every website in the e-commerce wave sits one of its licenses.",
				Bull:       "'触网'是所有 CEO 的年度关键词，而所有网站的地基都是数据库。",
				BullEn:     "'Get online' is every CEO's keyword of the year, and the foundation of every website is a database.",
				Bear:       "授权收入一次性确认，增长靠不断找新客户；该上网的都上完了怎么办。",
				BearEn:     "License revenue is recognized all at once, so growth depends on constantly finding new customers; what happens once everyone who should be online already is?"}},
		{ID: "X05", Raw: "ibm", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.2, "RATE": -0.2},
			Dossier: Dossier{Alias: "Halcyon Computing", Aliases: []string{"Halcyon Computing", "Sentinel Computing", "Granville Systems"}, Desc: "百年计算公司", RealName: "IBM",
				DescEn:     "A century-old computing company",
				Business:   "大型机、服务与咨询三驾马车，新经济里做旧经济的生意。",
				BusinessEn: "Mainframes, services, and consulting as its three engines — doing old-economy business inside the new economy.",
				Bull:       "企业级信任无可替代，泡沫破了大家还得找它做系统集成。",
				BullEn:     "Enterprise-grade trust is irreplaceable; when the bubble bursts, everyone will still come to it for systems integration.",
				Bear:       "增长平庸，故事乏味，在狂热年代里资金看不上稳健。",
				BearEn:     "Mediocre growth and a dull story; in manic times, capital looks down on steadiness."}},
		{ID: "X06", Raw: "aapl", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.2},
			Dossier: Dossier{Alias: "Solstice Computers", Aliases: []string{"Solstice Computers", "Zephyr Computers", "Auric Systems"}, Desc: "特立独行的电脑厂", RealName: "Apple",
				DescEn:     "The maverick computer maker",
				Business:   "设计驱动的个人电脑，创始人回归后靠一体机打了场翻身仗。",
				BusinessEn: "Design-driven personal computers; after its founder's return, an all-in-one machine staged the comeback.",
				Bull:       "品牌信徒式的忠诚度，硬件卖出了奢侈品毛利。",
				BullEn:     "Cult-like brand loyalty lets it sell hardware at luxury-goods margins.",
				Bear:       "市场份额个位数，一款产品失手就是一个财年的灾难。",
				BearEn:     "Single-digit market share; one product miss is a fiscal-year disaster."}},
		{ID: "X07", Raw: "amzn", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 1.0, "RATE": -0.5},
			Dossier: Dossier{Alias: "Longmoor Retail", Aliases: []string{"Longmoor Retail", "Everline Commerce", "Crossharbor Retail"}, Desc: "烧钱换增长的万货商店", RealName: "Amazon",
				DescEn:     "An everything store burning cash for growth",
				Business:   "从图书扩到全品类的线上零售，自建仓储物流，每单都亏，每季单量都新高。",
				BusinessEn: "Online retail expanded from books to every category, with self-built warehousing and logistics; it loses money on every order and sets order-volume records every quarter.",
				Bull:       "零售的未来在线上，先烧钱圈地者赢者通吃；今天的亏损是明天垄断的门票。",
				BullEn:     "The future of retail is online, and whoever burns cash to grab land first takes all; today's losses are the ticket to tomorrow's monopoly.",
				Bear:       "现金消耗率惊人，命脉握在资本市场手里；融资窗口一关，增长故事一个季度变清算故事。",
				BearEn:     "Its cash burn is staggering and its lifeline is in the capital markets' hands; the moment the funding window closes, the growth story becomes a liquidation story within a quarter."}},
		{ID: "X08", Raw: "ebay", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "Fenwick Exchange", Aliases: []string{"Fenwick Exchange", "Gavelmark Exchange", "Brightwell Auctions"}, Desc: "全民线上拍卖行", RealName: "eBay",
				DescEn:     "Everyone's online auction house",
				Business:   "买卖双方自己定价的拍卖平台，轻资产抽佣，罕见地在互联网公司里赚钱。",
				BusinessEn: "An auction platform where buyers and sellers set prices themselves; asset-light commissions make it a rare profitable internet company.",
				Bull:       "网络效应教科书：买家越多卖家越多，飞轮一旦转起来没人追得上。",
				BullEn:     "A textbook network effect: more buyers bring more sellers, and once the flywheel spins, nobody can catch up.",
				Bear:       "假货与欺诈是平台的原罪，信任一旦崩塌飞轮就倒转。",
				BearEn:     "Counterfeits and fraud are the platform's original sin; once trust collapses, the flywheel reverses."}},
		{ID: "X09", Raw: "amd", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "Crestline Semiconductor", Aliases: []string{"Crestline Semiconductor", "Stratos Semiconductor", "Wolfcreek Microsystems"}, Desc: "万年老二的芯片挑战者", RealName: "AMD",
				DescEn:     "The perennial runner-up challenging the chip throne",
				Business:   "处理器市场的挑战者，靠性价比和新架构从霸主嘴里抢份额。",
				BusinessEn: "The challenger in the processor market, grabbing share from the overlord on price-performance and new architectures.",
				Bull:       "老二逆袭的故事最性感，新品每赢一次评测股价就跳一级。",
				BullEn:     "The underdog comeback is the sexiest story; every benchmark win sends the stock up a level.",
				Bear:       "价格战里毛利被霸主按在地上摩擦，一代产品失手就要卖厂求生。",
				BearEn:     "In price wars the overlord grinds its margins into the dirt; one failed product generation means selling fabs to survive."}},
		{ID: "X10", Raw: "qcom", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "Wavecrest Communications", Aliases: []string{"Wavecrest Communications", "Signalcrest Technologies", "Marbleton Wireless"}, Desc: "无线时代的专利收租人", RealName: "Qualcomm",
				DescEn:     "The patent rent-collector of the wireless age",
				Business:   "手机通信标准的核心专利持有者，卖芯片更收授权费，躺着分成。",
				BusinessEn: "Holder of core patents in mobile communication standards; it sells chips and, better, collects royalties — earning a cut without lifting a finger.",
				Bull:       "无线互联网是下一波浪潮，每卖一部手机它都抽成。",
				BullEn:     "The wireless internet is the next wave, and it takes a cut of every phone sold.",
				Bear:       "标准之争悬而未决，押错路线专利池就成废纸；估值已按赢者通吃计价。",
				BearEn:     "The standards battle is unresolved; back the wrong standard and the patent pool becomes scrap paper — yet the valuation already prices in winner-take-all."}},
		{ID: "X11", Raw: "txn", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "Dunmore Instruments", Aliases: []string{"Dunmore Instruments", "Quietline Instruments", "Bedford Electronics"}, Desc: "模拟芯片的隐形冠军", RealName: "Texas Instruments",
				DescEn:     "The hidden champion of analog chips",
				Business:   "从计算器到手机基带的模拟与数字信号芯片，客户遍布所有电子产品。",
				BusinessEn: "Analog and signal-processing chips from calculators to phone basebands, with customers across every electronic product.",
				Bear:       "下游消费电子景气一凉，订单立刻跟着凉。",
				BearEn:     "The moment downstream consumer electronics cool, its orders cool with them.",
				Bull:       "不押注单一终端，什么电子设备火它都有份。",
				BullEn:     "Not betting on any single device category — whatever electronics sell, it gets a piece."}},
		{ID: "X12", Raw: "adbe", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "Marlow Creative Systems", Aliases: []string{"Marlow Creative Systems", "Palette Creative Systems", "Aster Design Systems"}, Desc: "设计师的标配工具箱", RealName: "Adobe",
				DescEn:     "The standard toolbox of designers",
				Business:   "图像处理与排版软件的事实标准，网页时代设计需求爆发的直接受益者。",
				BusinessEn: "The de facto standard in image editing and publishing software, a direct beneficiary of the web era's explosion in design demand.",
				Bull:       "每个新网站都需要设计师，每个设计师都得买它的全家桶。",
				BullEn:     "Every new website needs a designer, and every designer has to buy its full suite.",
				Bear:       "工具软件盗版猖獗，增长天花板取决于正版化速度。",
				BearEn:     "Piracy of tools software is rampant; the growth ceiling depends on how fast legalization catches up."}},
		{ID: "X13", Raw: "hpq", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.2},
			Dossier: Dossier{Alias: "Branford Instruments", Aliases: []string{"Branford Instruments", "Kingsport Instruments", "Alton Precision Systems"}, Desc: "硅谷车库神话的老牌厂", RealName: "Hewlett-Packard",
				DescEn:     "A veteran of the Silicon Valley garage legend",
				Business:   "打印机、个人电脑与服务器的全线硬件厂，打印耗材是隐藏的现金牛。",
				BusinessEn: "A full-line hardware maker of printers, PCs, and servers; printing supplies are the hidden cash cow.",
				Bull:       "墨盒是比咖啡更暴利的消耗品，装机量就是年金。",
				BullEn:     "Ink cartridges are a consumable more lucrative than coffee; the installed base is an annuity.",
				Bear:       "电脑业务毛利薄如纸，大公司病拖慢每一次转身。",
				BearEn:     "PC margins are razor-thin, and big-company disease slows every turn."}},
		{ID: "X14", Raw: "ge", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.1, "RATE": -0.4, "OIL": 0.1},
			Dossier: Dossier{Alias: "Grantham Industries", Aliases: []string{"Grantham Industries", "Beacon Industrial Group", "Magnus Consolidated"}, Desc: "什么都造的工业帝国", RealName: "General Electric",
				DescEn:     "An industrial empire that makes everything",
				Business:   "发电机、飞机引擎、医疗设备加一个庞大的金融部门，业务横跨所有周期。",
				BusinessEn: "Generators, aircraft engines, medical equipment, plus a huge finance arm — spanning every cycle.",
				Bull:       "传奇 CEO 治下二十年利润从不失手，机构的压舱石首选。",
				BullEn:     "Under its legendary CEO, profits haven't missed in two decades — institutions' ballast of choice.",
				Bear:       "金融部门的杠杆是报表深处的暗礁，'从不失手'本身就值得怀疑。",
				BearEn:     "The finance arm's leverage is a reef deep in the statements, and 'never misses' is itself suspicious."}},
		{ID: "X15", Raw: "xom", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": -0.1, "OIL": 0.8, "GOLD": 0.1},
			Dossier: Dossier{Alias: "Grandhaven Petroleum", Aliases: []string{"Grandhaven Petroleum", "Seaward Energy", "Monarch Petroleum"}, Desc: "全球油气巨轮", RealName: "ExxonMobil",
				DescEn:     "A global oil-and-gas supertanker",
				Business:   "从油井到加油站的全产业链油气巨头，世纪合并后规模冠绝全球。",
				BusinessEn: "A fully integrated oil-and-gas giant from wellhead to gas station, its scale unmatched worldwide after a merger of the century.",
				Bull:       "无论线上线下，人总要开车取暖；恐慌时现金流是最硬的叙事。",
				BullEn:     "Online or offline, people still drive and heat their homes; in a panic, cash flow is the hardest narrative.",
				Bear:       "油价下行周期里巨轮也得随波逐流，狂热年代长期跑输大盘。",
				BearEn:     "In an oil-price downcycle even supertankers drift with the tide; in manic years it chronically lags the market."}},
		{ID: "X16", Raw: "wmt", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.0, "RATE": -0.2},
			Dossier: Dossier{Alias: "Fairhaven Stores", Aliases: []string{"Fairhaven Stores", "Sundial Stores", "Harvestmart Retail"}, Desc: "乡镇起家的零售之王", RealName: "Walmart",
				DescEn:     "The retail king from small-town roots",
				Business:   "天天低价的连锁超市帝国，供应链效率碾压一切同行。",
				BusinessEn: "An everyday-low-price supermarket empire whose supply-chain efficiency crushes every rival.",
				Bull:       "九成五的零售仍在线下，它便宜、赚钱、还在扩张。",
				BullEn:     "Ninety-five percent of retail still happens offline, and it is cheap, profitable, and still expanding.",
				Bear:       "它是电商故事里被指名道姓的'被颠覆者'。",
				BearEn:     "It is the named 'disruption target' in every e-commerce story."}},
		{ID: "X17", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.4, "RATE": -0.5},
			Anchors: []Anchor{
				{"1999-01-04", 60, 0}, {"1999-06-21", 64, 0}, {"2000-01-03", 44, 0},
				{"2000-07-03", 40, 0}, {"2000-11-01", 18, 0}, {"2001-03-01", 16, 0},
				{"2001-07-02", 15, 0}, {"2001-10-01", 14, 0}, {"2001-12-31", 11.5, 0},
			},
			Dossier: Dossier{Alias: "Aldergate Communications", Aliases: []string{"Aldergate Communications", "Apex Communications", "Longhaul Telecom Group"}, Desc: "并购成瘾的长途电话帝国", RealName: "WorldCom（重建）",
				DescEn:     "A long-distance phone empire addicted to M&A",
				Business:   "靠连环并购堆出来的长途与数据通信巨头，报表增速全行业最快。",
				BusinessEn: "A long-distance and data-communications giant stacked together by serial acquisitions, with the fastest reported growth in the industry.",
				Bull:       "互联网流量每百天翻一倍——它自己说的，卖的就是流量的管道。",
				BullEn:     "Internet traffic doubles every hundred days — its own claim — and it sells the pipes that traffic flows through.",
				Bear:       "并购停下来的那天，增长从哪来？报表越漂亮的公司越要多问一句。",
				BearEn:     "When the acquisitions stop, where does growth come from? The prettier the books, the more questions one should ask."}},
		{ID: "X18", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 55, 0}, {"1999-11-01", 65, 0}, {"1999-12-09", 80, 0},
				{"2000-01-14", 72, 0}, {"2000-07-03", 58, 0}, {"2000-10-02", 30, 0},
				{"2000-12-01", 17, 0}, {"2001-06-01", 7, 0}, {"2001-12-31", 6.3, 0},
			},
			Dossier: Dossier{Alias: "Kestrel Telecom Equipment", Aliases: []string{"Kestrel Telecom Equipment", "Foxglove Telecom Systems", "Albion Network Equipment"}, Desc: "百年实验室拆出的设备商", RealName: "Lucent（重建）",
				DescEn:     "An equipment maker spun out of a century-old lab",
				Business:   "从传奇实验室分拆的电信设备商，光网络与交换机订单排到明年。",
				BusinessEn: "A telecom-equipment maker spun off from a legendary laboratory, with optical-networking and switch orders booked into next year.",
				Bull:       "运营商军备竞赛的最大军火商，手握全行业最深的技术储备。",
				BullEn:     "The biggest arms dealer of the carrier arms race, holding the deepest technology bench in the industry.",
				Bear:       "为了冲营收向客户放贷卖货——客户还不上钱时，营收和坏账一起爆。",
				BearEn:     "It lends customers money to buy its gear to juice revenue — when customers can't pay, revenue and bad debts blow up together."}},
		{ID: "X19", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 12, 0}, {"1999-12-01", 25, 0}, {"2000-07-17", 87, 0},
				{"2000-10-02", 60, 0}, {"2001-02-01", 32, 0}, {"2001-06-01", 11, 0},
				{"2001-09-17", 5.5, 0}, {"2001-12-31", 7.5, 0},
			},
			Dossier: Dossier{Alias: "Skyline Optical Networks", Aliases: []string{"Skyline Optical Networks", "Polaris Photonics", "Glacier Optical Systems"}, Desc: "北方来的光网络之王", RealName: "Nortel（重建）",
				DescEn:     "The optical-networking king from the north",
				Business:   "光传输设备的领跑者，骨干网扩容潮里订单接到手软。",
				BusinessEn: "The leader in optical transmission gear, drowning in orders amid the backbone build-out boom.",
				Bull:       "带宽需求没有尽头，光纤铺到哪它卖到哪。",
				BullEn:     "Bandwidth demand has no end; wherever fiber is laid, it sells.",
				Bear:       "客户全是举债铺网的运营商；产能按泡沫顶点规划，退潮时最先搁浅。",
				BearEn:     "Its customers are all carriers laying fiber on borrowed money; capacity was planned for the bubble's peak, so it will be first aground when the tide goes out."}},
		{ID: "X20", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.6},
			Anchors: []Anchor{
				{"1999-01-04", 22, 0}, {"1999-05-03", 60, 0}, {"2000-01-03", 50, 0},
				{"2000-09-01", 30, 0}, {"2001-01-02", 14, 0}, {"2001-06-01", 8, 0},
				{"2001-10-01", 1.9, 0}, {"2001-12-31", 0.85, 0},
			},
			Dossier: Dossier{Alias: "Deepcurrent Networks", Aliases: []string{"Deepcurrent Networks", "Seabridge Networks", "Pelagic Cable Systems"}, Desc: "海底光缆狂想家", RealName: "Global Crossing（重建）",
				DescEn:     "The undersea-cable dreamer",
				Business:   "举债在大洋底铺光缆的全球网络运营商，资产是几万公里的海底玻璃。",
				BusinessEn: "A global network operator laying fiber-optic cable on the ocean floor with borrowed money; its asset is tens of thousands of kilometers of submarine glass.",
				Bull:       "把五大洲连起来的人收全世界的过路费。",
				BullEn:     "Whoever connects the five continents collects tolls from the whole world.",
				Bear:       "光缆铺得越多带宽越不值钱；债务是刚性的，带宽价格不是。",
				BearEn:     "The more cable laid, the cheaper bandwidth gets; debt is rigid, bandwidth prices are not."}},
		{ID: "X21", Raw: "ndx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.6},
			Dossier: Dossier{Alias: "Momentum 100 Index Fund", Aliases: []string{"Momentum 100 Index Fund", "Century Tech Index Fund", "Velocity 100 Index"}, Desc: "科技百强指数基金", RealName: "NASDAQ-100 指数",
				DescEn:     "A top-100 tech index fund",
				Business:   "一键买入整个新经济：一篮子科技龙头的指数化组合。",
				BusinessEn: "Buy the entire new economy in one click: an indexed basket of tech leaders.",
				Bull:       "选不出赢家就全买，时代的贝塔好过个股的阿尔法。",
				BullEn:     "If you can't pick the winners, buy them all — the era's beta beats any single stock's alpha.",
				Bear:       "篮子里装的全是同一个故事，泡沫破时没有分散可言。",
				BearEn:     "The basket holds one and the same story; when the bubble bursts there is no diversification to speak of."}},
		{ID: "X22", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.3},
			Dossier: Dossier{Alias: "Capital 500 Index", Aliases: []string{"Capital 500 Index", "Broadmarket 500 Index", "Liberty 500 Index"}, Desc: "全市场指数基金", RealName: "S&P 500 指数",
				DescEn:     "A whole-market index fund",
				Business:   "五百家大公司的市值加权组合，美国经济本身。",
				BusinessEn: "A market-cap-weighted basket of five hundred large companies — the economy itself.",
				Bull:       "不赌行业不赌个股，赌国运。",
				BullEn:     "Bet on no sector and no stock; bet on the country's fortune.",
				Bear:       "科技权重已被泡沫吹到历史高位，'分散'没有看上去那么分散。",
				BearEn:     "Tech's weight has been inflated to historic highs by the bubble; the 'diversification' is less diversified than it looks."}},
	},
}

func init() {
	Register(&dotcomUniverse)
}
