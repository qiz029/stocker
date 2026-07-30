package pipeline

// nifty1972FetchSpecs: the 1972-era (nifty-1972) contribution to `pipeline
// fetch`'s download list. Every name here is deduped against the other
// universes' FetchSpecs by `pipeline fetch` (union by Name), so listing
// shared symbols (ko/mcd/dis/jnj/pg/mmm/cat/ibm/xom/spx) again here — even
// though universe_dotcom.go/universe_1987.go already list most of them with
// their own From/To — is correct: it documents which universes actually
// consume each file, following the shared-symbol convention. Shared symbols
// keep IDENTICAL From/To values across all universes, so `cmd/pipeline`'s
// FetchSpec dedup (first-seen-by-Name, across Universes() in sorted id order)
// never depends on which universe happens to be processed first.
//
// xrx is this universe's own symbol, moved here during plan-5 Task 3
// when it was fetched ahead of this universe's existence. xom's From was
// widened a second time by this universe (1970-06-01, not the 1985-06-01
// the plan-5 Task 3 table originally used) — see the doc comment on xom's
// entry in universe_dotcom.go for why.
//
// N14 (dji, Founders Bluechip Index/道琼斯工业指数) is DROPPED per controller ruling: ^DJI has no
// pre-1992 daily history on Yahoo's chart API (see the dji entry doc in
// universe_2008.go), which is after this scenario's entire 1972-01..1975-06
// window. This universe has 14 instruments (N01..N13, N15); N15 remains
// "N15" (MarketProxy) per the controller's explicit instruction not to
// renumber.
//
// avp/ek have no FetchSpec entries: both are unfetchable per the
// pre-authorized availability contingency (verified live in plan-5 Task 3 —
// AVP 404s, EK resolves to an unrelated Yahoo-internal placeholder with no
// relevant history) and fall back to Anchors-based reconstruction below,
// the controller-ruled resolution for both. xrx WAS fetchable (1970-06-01
// start verified, 1537 bars) and is used as real data with no anchors.
var nifty1972FetchSpecs = []FetchSpec{
	{Name: "ko", Symbol: "KO", From: "1970-06-01", To: "1989-06-30"},
	{Name: "mcd", Symbol: "MCD", From: "1970-06-01", To: "2010-06-30"},
	{Name: "dis", Symbol: "DIS", From: "1970-06-01", To: "1989-06-30"},
	{Name: "jnj", Symbol: "JNJ", From: "1970-06-01", To: "1989-06-30"},
	{Name: "pg", Symbol: "PG", From: "1970-06-01", To: "1989-06-30"},
	{Name: "mmm", Symbol: "MMM", From: "1970-06-01", To: "1989-06-30"},
	{Name: "cat", Symbol: "CAT", From: "1970-06-01", To: "1989-06-30"},
	{Name: "ibm", Symbol: "IBM", From: "1970-06-01", To: "2002-03-31"},
	{Name: "xom", Symbol: "XOM", From: "1970-06-01", To: "2010-06-30"},
	{Name: "spx", Symbol: "^GSPC", From: "1970-06-01", To: "2010-06-30"},
	// This universe's own symbol, moved off pendingFetchSpecs.
	{Name: "xrx", Symbol: "XRX", From: "1970-06-01", To: "1976-06-30"},
}

var nifty1972Universe = ScenarioUniverse{
	ScenarioID:  "nifty-1972",
	Name:        "1972 漂亮50与石油危机",
	RealPeriod:  "1972-01 ~ 1975-06",
	EraHint:     "类似蓝筹成长股信仰与石油危机交织的年代",
	WindowStart: "1972-01-03",
	WindowEnd:   "1975-06-30",
	MarketProxy: "N15",
	FetchSpecs:  nifty1972FetchSpecs,
	Sectors: []SectorSpec{
		{"NIFT", "一流成长"}, {"OFFC", "办公科技"}, {"IND", "工业"}, {"ENGY", "能源"},
	},
	Macros: []SectorSpec{{"GOLD", "黄金"}, {"OIL", "原油"}, {"RATE", "利率"}},
	KeyWindows: []DateWindow{
		{"1973-11-01", "1974-01-04", -1}, // 禁运恐慌
		{"1974-08-01", "1974-10-04", -1}, // 投降底
	},
	Instruments: []InstrumentSpec{
		{ID: "N01", Raw: "ko", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Suncrest Beverages", Aliases: []string{"Suncrest Beverages", "Fizzwell Beverages", "Colonial Bottling Co."}, Desc: "全球装瓶的饮料霸主", RealName: "Coca-Cola",
				Business: "一瓶糖水靠装瓶网络卖到全球每个角落的品牌帝国，配方是压箱底的秘密。",
				Bull:     "只要还有人口渴，提价权与全球化就是永动机；机构组合里公认'买了就不用管'的信仰股。",
				Bear:     "信仰的代价是天价市盈率，只要增长慢下来一个季度，故事就会被重新计算。"}},
		{ID: "N02", Raw: "mcd", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.55, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Crossroads Restaurants", Aliases: []string{"Crossroads Restaurants", "Speedway Burger Co.", "Quickserve Restaurants"}, Desc: "高速展店的快餐新贵", RealName: "McDonald's",
				Business: "标准化流程复制到每个路口的连锁快餐，新店开张的速度本身就是故事。",
				Bull:     "加盟费和地产租金滚雪球，每开一家新店就多锁定一份未来现金流。",
				Bear:     "扩张神话总有天花板，路口开完了，同店销售的真实增长才见分晓。"}},
		{ID: "N03", Raw: "dis", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.65, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Carousel Entertainment", Aliases: []string{"Carousel Entertainment", "Reelwood Entertainment", "Playland Amusements"}, Desc: "刚开完新乐园的娱乐帝国", RealName: "Disney",
				Business: "老牌动画厂靠新落成的主题乐园、门票与周边授权撑起下一段增长曲线。",
				Bull:     "家庭出游是永恒的需求，乐园门票年年提价，客流照样爆满。",
				Bear:     "大项目建成之后靠什么讲新故事？内容生意大小年，观众的口味说变就变。"}},
		{ID: "N04", Raw: "jnj", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Cloverleaf Health", Aliases: []string{"Cloverleaf Health", "Cradlecare Health", "Trusty Medical Group"}, Desc: "永远增长的健康帝国", RealName: "Johnson & Johnson",
				Business: "婴儿护理用品与医疗器械、处方药三条线并行，家家户户的浴室柜里都有它。",
				Bull:     "人只会越活越久，医疗支出只涨不跌，永续增长的教科书案例。",
				Bear:     "分散到没有一条业务能真正加速，'永远增长'的另一面是永远平庸。"}},
		{ID: "N05", Raw: "pg", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.55, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Kirkwood Household Brands", Aliases: []string{"Kirkwood Household Brands", "Everyday Household Brands", "Marrowgate Consumer Goods"}, Desc: "百年日化帝国", RealName: "Procter & Gamble",
				Business: "牙膏、洗衣粉、纸尿裤，货架上一堆认不全集团名字的日用品全是它旗下的。",
				Bull:     "消费者换品牌的成本极高，现金流稳定到可以当债券买。",
				Bear:     "增长全靠一点点抢占货架份额，讲不出让人兴奋的新故事，估值却按成长股计价。"}},
		{ID: "N06", Raw: "ibm", Sector: "OFFC",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.3},
			Dossier: Dossier{Alias: "Sterling Computing", Aliases: []string{"Sterling Computing", "Ledger Computing Corp.", "Duplex Data Systems"}, Desc: "大型计算机的代名词", RealName: "IBM",
				Business: "企业后台离不开的大型计算设备，整机、软件与服务捆绑租赁，客户粘性极高。",
				Bull:     "每一家大公司迟早都要租一台它的机器，装机量就是滚滚而来的年金。",
				Bear:     "机器体积庞大、价格高昂，一旦有更便宜的替代方案冒头，客户未必肯继续买单。"}},
		{ID: "N07", Raw: "xrx", Sector: "OFFC",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.3},
			Dossier: Dossier{Alias: "Paperflow Systems", Aliases: []string{"Paperflow Systems", "Graphite Office Systems", "Tonerline Systems"}, Desc: "办公室复印机的代名词", RealName: "Xerox",
				Business: "每台复印机都是印钞机——设备租赁加按张计费，办公室离不开它。",
				Bull:     "无纸化还是科幻小说，纸张洪流只增不减，装机量就是年金。",
				Bear:     "当核心专利保护伞收起、模仿者涌入时，按张计费的暴利模式首当其冲。"}},
		{ID: "N08", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 90, 0}, {"1973-01-02", 145, 0}, {"1973-12-03", 110, 0},
				{"1974-10-01", 60, 0}, {"1975-06-30", 95, 0},
			},
			Dossier: Dossier{Alias: "Silvergrain Photographics", Aliases: []string{"Silvergrain Photographics", "Momenta Photographics", "Shutterwell Imaging"}, Desc: "感光胶卷与相机霸主", RealName: "Eastman Kodak（重建）",
				Business: "从胶卷到相纸再到冲印，家家户户记录回忆都绕不开它的黄色包装。",
				Bull:     "全民摄影才刚刚开始普及，每按一次快门就要再买一卷胶卷，复购是天生的商业模式。",
				Bear:     "整条生意链都押注在同一种感光技术路径上，若有新的成像方式跑出来，护城河一夜作废。"}},
		{ID: "N09", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 90, 0}, {"1972-06-01", 110, 0}, {"1973-01-02", 140, 0},
				{"1973-06-01", 120, 0}, {"1973-12-03", 70, 0}, {"1974-06-03", 40, 0},
				{"1974-10-01", 15, 0}, {"1975-06-30", 30, 0},
			},
			Dossier: Dossier{Alias: "Lumina Optics", Aliases: []string{"Lumina Optics", "Prisma Optics", "Vista Camera Works"}, Desc: "即时成像相机的发明者", RealName: "Polaroid（重建）",
				Business: "按下快门一分钟后照片就在手里——即时成像的独家专利帝国，毛利像奢侈品。",
				Bull:     "一次性决策股：这种公司买了就永远不用卖，付五十倍市盈率是为未来三十年付的。",
				Bear:     "为'永远'付的价格，只要增长慢一个季度就会塌方；专利到期与新技术是永远悬着的剑。"}},
		{ID: "N10", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.65, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 100, 0}, {"1973-03-01", 135, 0}, {"1974-01-02", 60, 0},
				{"1974-10-01", 19, 0}, {"1975-06-30", 35, 0},
			},
			Dossier: Dossier{Alias: "Homeway Cosmetics", Aliases: []string{"Homeway Cosmetics", "Neighborhood Beauty Co.", "Satchel Cosmetics"}, Desc: "上门直销的化妆品帝国", RealName: "Avon（重建）",
				Business: "靠上门推销员一家一户敲门卖化妆品，销售大军规模就是护城河。",
				Bull:     "直销大军渗透到每条街区，业绩曲线比商场专柜更陡峭。",
				Bear:     "生意的地基是招募到足够多愿意敲门的推销员，一旦人手招不够，飞轮就会反转。"}},
		{ID: "N11", Raw: "mmm", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.45},
			Dossier: Dossier{Alias: "Versatek Materials", Aliases: []string{"Versatek Materials", "Bondwell Materials", "Multimix Industries"}, Desc: "什么都能粘的材料公司", RealName: "3M",
				Business: "从胶带到研磨材料，几千种工业与办公产品共享同一套材料科学底子。",
				Bull:     "产品线极度分散，只要工厂在开工总有一样东西被买走。",
				Bear:     "工业周期一旦转冷，资本开支收缩会传导到它的每一条产品线。"}},
		{ID: "N12", Raw: "cat", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.5},
			Dossier: Dossier{Alias: "Quarryline Heavy Industries", Aliases: []string{"Quarryline Heavy Industries", "Dozerline Heavy Industries", "Boulder Equipment Group"}, Desc: "推土机与矿山机械之王", RealName: "Caterpillar",
				Business: "工地与矿场少不了的重型机械，经销商网络铺满全球每一片工地。",
				Bull:     "基建与资源开发是长期趋势，重型设备折旧完了还得再买新的。",
				Bear:     "重资产、高杠杆的周期股，利率一抬头，工地的订单第一个被砍。"}},
		{ID: "N13", Raw: "xom", Sector: "ENGY",
			ExtraBeta: map[string]float64{"SENT": 0.15, "RATE": -0.25, "OIL": 0.7, "GOLD": 0.15},
			Dossier: Dossier{Alias: "Ironshore Petroleum", Aliases: []string{"Ironshore Petroleum", "Derrick Petroleum", "Greatplain Energy"}, Desc: "全球油气巨轮", RealName: "Exxon",
				Business: "从油井到加油站的全产业链油气巨头，规模冠绝全球同行。",
				Bull:     "禁运一来油价飞涨，谁攥着油井谁就攥着定价权，恐慌时现金流是最硬的叙事。",
				Bear:     "配给限购和物价管制的传闻不断，政策的手随时可能把超额利润摁下去。"}},
		{ID: "N15", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.35},
			Dossier: Dossier{Alias: "Bluechip 500 Index", Aliases: []string{"Bluechip 500 Index", "Topflight 500 Index", "Crest 500 Index"}, Desc: "五百家大公司指数", RealName: "S&P 500 指数",
				Business: "五百家大公司的市值加权组合，一笔交易买下整个经济的横截面。",
				Bull:     "不赌个股不赌行业，赌整体经济的长期向上。",
				Bear:     "信仰股的权重太重，当'买了就不用管'的股票集体重新定价，指数也无处可藏。"}},
	},
}

func init() {
	Register(&nifty1972Universe)
}
