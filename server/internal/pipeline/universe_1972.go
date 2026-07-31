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
	NameEn:      "1972 Nifty Fifty & Oil Crisis",
	RealPeriod:  "1972-01 ~ 1975-06",
	EraHint:     "类似蓝筹成长股信仰与石油危机交织的年代",
	WindowStart: "1972-01-03",
	WindowEnd:   "1975-06-30",
	MarketProxy: "N15",
	FetchSpecs:  nifty1972FetchSpecs,
	Sectors: []SectorSpec{
		{ID: "NIFT", Name: "一流成长", NameEn: "Blue-Chip Growth"},
		{ID: "OFFC", Name: "办公科技", NameEn: "Office Technology"},
		{ID: "IND", Name: "工业", NameEn: "Industrials"},
		{ID: "ENGY", Name: "能源", NameEn: "Energy"},
	},
	Macros: []SectorSpec{
		{ID: "GOLD", Name: "黄金", NameEn: "Gold"},
		{ID: "OIL", Name: "原油", NameEn: "Crude Oil"},
		{ID: "RATE", Name: "利率", NameEn: "Interest Rates"},
	},
	KeyWindows: []DateWindow{
		{"1973-11-01", "1974-01-04", -1}, // 禁运恐慌
		{"1974-08-01", "1974-10-04", -1}, // 投降底
	},
	Instruments: []InstrumentSpec{
		{ID: "N01", Raw: "ko", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Suncrest Beverages", Aliases: []string{"Suncrest Beverages", "Fizzwell Beverages", "Colonial Bottling Co."}, Desc: "全球装瓶的饮料霸主", RealName: "Coca-Cola",
				DescEn:     "The global bottling beverage overlord",
				Business:   "一瓶糖水靠装瓶网络卖到全球每个角落的品牌帝国，配方是压箱底的秘密。",
				BusinessEn: "A brand empire selling a bottle of sugared water to every corner of the globe through its bottling network; the formula is the family's most guarded secret.",
				Bull:       "只要还有人口渴，提价权与全球化就是永动机；机构组合里公认'买了就不用管'的信仰股。",
				BullEn:     "As long as people get thirsty, pricing power and globalization are perpetual-motion machines; institutions call it the 'buy it and forget it' faith stock.",
				Bear:       "信仰的代价是天价市盈率，只要增长慢下来一个季度，故事就会被重新计算。",
				BearEn:     "Faith costs a sky-high P/E; one quarter of slower growth and the story gets recalculated."}},
		{ID: "N02", Raw: "mcd", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.55, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Crossroads Restaurants", Aliases: []string{"Crossroads Restaurants", "Speedway Burger Co.", "Quickserve Restaurants"}, Desc: "高速展店的快餐新贵", RealName: "McDonald's",
				DescEn:     "A fast-expanding fast-food upstart",
				Business:   "标准化流程复制到每个路口的连锁快餐，新店开张的速度本身就是故事。",
				BusinessEn: "A standardized fast-food formula replicated at every intersection; the speed of new openings is itself the story.",
				Bull:       "加盟费和地产租金滚雪球，每开一家新店就多锁定一份未来现金流。",
				BullEn:     "Franchise fees and real-estate rents snowball; every new store locks in another stream of future cash flow.",
				Bear:       "扩张神话总有天花板，路口开完了，同店销售的真实增长才见分晓。",
				BearEn:     "Every expansion myth has a ceiling; once the intersections are all taken, true same-store growth will show itself."}},
		{ID: "N03", Raw: "dis", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.65, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Carousel Entertainment", Aliases: []string{"Carousel Entertainment", "Reelwood Entertainment", "Playland Amusements"}, Desc: "刚开完新乐园的娱乐帝国", RealName: "Disney",
				DescEn:     "An entertainment empire fresh from opening a new park",
				Business:   "老牌动画厂靠新落成的主题乐园、门票与周边授权撑起下一段增长曲线。",
				BusinessEn: "A veteran animation studio whose next growth leg rests on a newly opened theme park, ticket sales, and merchandise licensing.",
				Bull:       "家庭出游是永恒的需求，乐园门票年年提价，客流照样爆满。",
				BullEn:     "Family outings are eternal demand; park tickets rise every year and the crowds keep coming.",
				Bear:       "大项目建成之后靠什么讲新故事？内容生意大小年，观众的口味说变就变。",
				BearEn:     "Once the big project is built, what tells the next story? Content is hit-driven and audiences' tastes change overnight."}},
		{ID: "N04", Raw: "jnj", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Cloverleaf Health", Aliases: []string{"Cloverleaf Health", "Cradlecare Health", "Trusty Medical Group"}, Desc: "永远增长的健康帝国", RealName: "Johnson & Johnson",
				DescEn:     "The health empire that grows forever",
				Business:   "婴儿护理用品与医疗器械、处方药三条线并行，家家户户的浴室柜里都有它。",
				BusinessEn: "Three lines in parallel — baby care, medical devices, prescription drugs; it's in every household's bathroom cabinet.",
				Bull:       "人只会越活越久，医疗支出只涨不跌，永续增长的教科书案例。",
				BullEn:     "People only live longer and medical spending only rises — a textbook case of perpetual growth.",
				Bear:       "分散到没有一条业务能真正加速，'永远增长'的另一面是永远平庸。",
				BearEn:     "So diversified that no single business can truly accelerate; the flip side of 'grows forever' is 'mediocre forever'."}},
		{ID: "N05", Raw: "pg", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.55, "RATE": -0.4, "OIL": -0.2},
			Dossier: Dossier{Alias: "Kirkwood Household Brands", Aliases: []string{"Kirkwood Household Brands", "Everyday Household Brands", "Marrowgate Consumer Goods"}, Desc: "百年日化帝国", RealName: "Procter & Gamble",
				DescEn:     "A century-old household-goods empire",
				Business:   "牙膏、洗衣粉、纸尿裤，货架上一堆认不全集团名字的日用品全是它旗下的。",
				BusinessEn: "Toothpaste, laundry detergent, diapers — the everyday goods on the shelf whose parent name you can't recite are all its brands.",
				Bull:       "消费者换品牌的成本极高，现金流稳定到可以当债券买。",
				BullEn:     "Consumers' cost of switching brands is enormous; the cash flow is so steady you could hold it like a bond.",
				Bear:       "增长全靠一点点抢占货架份额，讲不出让人兴奋的新故事，估值却按成长股计价。",
				BearEn:     "Growth comes from clawing shelf share inch by inch; there is no exciting new story, yet the valuation is priced like a growth stock."}},
		{ID: "N06", Raw: "ibm", Sector: "OFFC",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.3},
			Dossier: Dossier{Alias: "Sterling Computing", Aliases: []string{"Sterling Computing", "Ledger Computing Corp.", "Duplex Data Systems"}, Desc: "大型计算机的代名词", RealName: "IBM",
				DescEn:     "Synonymous with the mainframe",
				Business:   "企业后台离不开的大型计算设备，整机、软件与服务捆绑租赁，客户粘性极高。",
				BusinessEn: "Large computing machines no corporate back office can do without; hardware, software, and services bundled into leases keep customers extremely sticky.",
				Bull:       "每一家大公司迟早都要租一台它的机器，装机量就是滚滚而来的年金。",
				BullEn:     "Every big company eventually has to lease one of its machines; the installed base is a rolling annuity.",
				Bear:       "机器体积庞大、价格高昂，一旦有更便宜的替代方案冒头，客户未必肯继续买单。",
				BearEn:     "The machines are huge and expensive; once a cheaper alternative emerges, customers may not keep paying."}},
		{ID: "N07", Raw: "xrx", Sector: "OFFC",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.3},
			Dossier: Dossier{Alias: "Paperflow Systems", Aliases: []string{"Paperflow Systems", "Graphite Office Systems", "Tonerline Systems"}, Desc: "办公室复印机的代名词", RealName: "Xerox",
				DescEn:     "Synonymous with the office copier",
				Business:   "每台复印机都是印钞机——设备租赁加按张计费，办公室离不开它。",
				BusinessEn: "Every copier is a money press — equipment leasing plus per-page metering; offices can't function without it.",
				Bull:       "无纸化还是科幻小说，纸张洪流只增不减，装机量就是年金。",
				BullEn:     "The paperless office is still science fiction; the flood of paper only grows, and the installed base is an annuity.",
				Bear:       "当核心专利保护伞收起、模仿者涌入时，按张计费的暴利模式首当其冲。",
				BearEn:     "When the core patent umbrella folds and imitators flood in, the lucrative per-page model is first in the line of fire."}},
		{ID: "N08", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 90, 0}, {"1973-01-02", 145, 0}, {"1973-12-03", 110, 0},
				{"1974-10-01", 60, 0}, {"1975-06-30", 95, 0},
			},
			Dossier: Dossier{Alias: "Silvergrain Photographics", Aliases: []string{"Silvergrain Photographics", "Momenta Photographics", "Shutterwell Imaging"}, Desc: "感光胶卷与相机霸主", RealName: "Eastman Kodak（重建）",
				DescEn:     "The overlord of film and cameras",
				Business:   "从胶卷到相纸再到冲印，家家户户记录回忆都绕不开它的黄色包装。",
				BusinessEn: "From film to photo paper to developing, every household's memories pass through its yellow boxes.",
				Bull:       "全民摄影才刚刚开始普及，每按一次快门就要再买一卷胶卷，复购是天生的商业模式。",
				BullEn:     "Popular photography has only begun to spread; every shutter click means another roll of film sold — repurchase is the business model.",
				Bear:       "整条生意链都押注在同一种感光技术路径上，若有新的成像方式跑出来，护城河一夜作废。",
				BearEn:     "The whole business chain bets on one imaging technology; if a new way of capturing images emerges, the moat is void overnight."}},
		{ID: "N09", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 90, 0}, {"1972-06-01", 110, 0}, {"1973-01-02", 140, 0},
				{"1973-06-01", 120, 0}, {"1973-12-03", 70, 0}, {"1974-06-03", 40, 0},
				{"1974-10-01", 15, 0}, {"1975-06-30", 30, 0},
			},
			Dossier: Dossier{Alias: "Lumina Optics", Aliases: []string{"Lumina Optics", "Prisma Optics", "Vista Camera Works"}, Desc: "即时成像相机的发明者", RealName: "Polaroid（重建）",
				DescEn:     "Inventor of the instant camera",
				Business:   "按下快门一分钟后照片就在手里——即时成像的独家专利帝国，毛利像奢侈品。",
				BusinessEn: "Press the shutter and the photo is in your hand a minute later — an exclusive-patent instant-imaging empire with luxury-goods margins.",
				Bull:       "一次性决策股：这种公司买了就永远不用卖，付五十倍市盈率是为未来三十年付的。",
				BullEn:     "A one-decision stock: buy it and never sell; paying fifty times earnings is paying for the next thirty years.",
				Bear:       "为'永远'付的价格，只要增长慢一个季度就会塌方；专利到期与新技术是永远悬着的剑。",
				BearEn:     "A price paid for 'forever' collapses the moment growth slows for a single quarter; patent expiry and new technology are swords forever hanging overhead."}},
		{ID: "N10", Raw: "", Sector: "NIFT",
			ExtraBeta: map[string]float64{"SENT": 0.65, "RATE": -0.4, "OIL": -0.2},
			Anchors: []Anchor{
				{"1972-01-03", 100, 0}, {"1973-03-01", 135, 0}, {"1974-01-02", 60, 0},
				{"1974-10-01", 19, 0}, {"1975-06-30", 35, 0},
			},
			Dossier: Dossier{Alias: "Homeway Cosmetics", Aliases: []string{"Homeway Cosmetics", "Neighborhood Beauty Co.", "Satchel Cosmetics"}, Desc: "上门直销的化妆品帝国", RealName: "Avon（重建）",
				DescEn:     "A door-to-door cosmetics empire",
				Business:   "靠上门推销员一家一户敲门卖化妆品，销售大军规模就是护城河。",
				BusinessEn: "Cosmetics sold door to door by an army of salespeople; the size of the sales force is the moat.",
				Bull:       "直销大军渗透到每条街区，业绩曲线比商场专柜更陡峭。",
				BullEn:     "The direct-sales army reaches every block, and the earnings curve is steeper than any department-store counter's.",
				Bear:       "生意的地基是招募到足够多愿意敲门的推销员，一旦人手招不够，飞轮就会反转。",
				BearEn:     "The foundation of the business is recruiting enough people willing to knock on doors; once recruitment falls short, the flywheel reverses."}},
		{ID: "N11", Raw: "mmm", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.45},
			Dossier: Dossier{Alias: "Versatek Materials", Aliases: []string{"Versatek Materials", "Bondwell Materials", "Multimix Industries"}, Desc: "什么都能粘的材料公司", RealName: "3M",
				DescEn:     "A materials company that can stick anything",
				Business:   "从胶带到研磨材料，几千种工业与办公产品共享同一套材料科学底子。",
				BusinessEn: "From tape to abrasives, thousands of industrial and office products share one materials-science foundation.",
				Bull:       "产品线极度分散，只要工厂在开工总有一样东西被买走。",
				BullEn:     "The product line is so dispersed that as long as factories are running, something of its is always being bought.",
				Bear:       "工业周期一旦转冷，资本开支收缩会传导到它的每一条产品线。",
				BearEn:     "Once the industrial cycle cools, shrinking capital spending transmits to every one of its product lines."}},
		{ID: "N12", Raw: "cat", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.5},
			Dossier: Dossier{Alias: "Quarryline Heavy Industries", Aliases: []string{"Quarryline Heavy Industries", "Dozerline Heavy Industries", "Boulder Equipment Group"}, Desc: "推土机与矿山机械之王", RealName: "Caterpillar",
				DescEn:     "The king of bulldozers and mining machinery",
				Business:   "工地与矿场少不了的重型机械，经销商网络铺满全球每一片工地。",
				BusinessEn: "Heavy machinery no construction site or mine can do without, its dealer network covering every worksite on earth.",
				Bull:       "基建与资源开发是长期趋势，重型设备折旧完了还得再买新的。",
				BullEn:     "Infrastructure and resource development are long-term trends, and worn-out heavy equipment must be replaced.",
				Bear:       "重资产、高杠杆的周期股，利率一抬头，工地的订单第一个被砍。",
				BearEn:     "An asset-heavy, high-leverage cyclical; the moment rates tick up, worksite orders are the first to be cut."}},
		{ID: "N13", Raw: "xom", Sector: "ENGY",
			ExtraBeta: map[string]float64{"SENT": 0.15, "RATE": -0.25, "OIL": 0.7, "GOLD": 0.15},
			Dossier: Dossier{Alias: "Ironshore Petroleum", Aliases: []string{"Ironshore Petroleum", "Derrick Petroleum", "Greatplain Energy"}, Desc: "全球油气巨轮", RealName: "Exxon",
				DescEn:     "A global oil-and-gas supertanker",
				Business:   "从油井到加油站的全产业链油气巨头，规模冠绝全球同行。",
				BusinessEn: "A fully integrated oil-and-gas giant from wellhead to gas station, scale unmatched among global peers.",
				Bull:       "禁运一来油价飞涨，谁攥着油井谁就攥着定价权，恐慌时现金流是最硬的叙事。",
				BullEn:     "When the embargo comes, oil prices soar; whoever holds the wells holds the pricing power — in a panic, cash flow is the hardest narrative.",
				Bear:       "配给限购和物价管制的传闻不断，政策的手随时可能把超额利润摁下去。",
				BearEn:     "Rumors of rationing and price controls never stop; the hand of policy can press the excess profits down at any moment."}},
		{ID: "N15", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.35},
			Dossier: Dossier{Alias: "Bluechip 500 Index", Aliases: []string{"Bluechip 500 Index", "Topflight 500 Index", "Crest 500 Index"}, Desc: "五百家大公司指数", RealName: "S&P 500 指数",
				DescEn:     "An index of five hundred large companies",
				Business:   "五百家大公司的市值加权组合，一笔交易买下整个经济的横截面。",
				BusinessEn: "A market-cap-weighted basket of five hundred large companies — a cross-section of the whole economy in one trade.",
				Bull:       "不赌个股不赌行业，赌整体经济的长期向上。",
				BullEn:     "Bet on no stock and no sector; bet on the economy's long-term rise.",
				Bear:       "信仰股的权重太重，当'买了就不用管'的股票集体重新定价，指数也无处可藏。",
				BearEn:     "The faith stocks' weight is too heavy; when the 'buy-and-forget' names are repriced en masse, the index has nowhere to hide."}},
	},
}

func init() {
	Register(&nifty1972Universe)
}
