package pipeline

// crash1987FetchSpecs: the 1987-era (crash-1987) contribution to `pipeline
// fetch`'s download list. Every name here is deduped against the other
// universes' FetchSpecs by `pipeline fetch` (union by Name), so listing
// shared symbols (ko/mcd/dis/jnj/pg/mmm/cat/ibm/hpq/ge/xom/wmt/spx) again
// here — even though dotcom-2000/universe_dotcom.go already lists most of
// them with a wider From/To — is correct: it documents which universes
// actually consume each file, mirroring the comment convention already
// used in universe_dotcom.go and universe.go's (soon-empty) pendingFetchSpecs.
//
// mrk/ba/axp are this universe's own symbols, moved here from
// pendingFetchSpecs (plan-5 Task 3 fetched them ahead of this universe's
// existence; see universe.go's pendingFetchSpecs doc comment).
//
// Y16 (dji, 三十巨头/道琼斯工业指数) is DROPPED per controller ruling: ^DJI has
// no pre-1992 daily history on Yahoo's chart API (see pendingFetchSpecs'
// dji comment in universe.go — verified against several period1 values
// spanning 1970-1985, every one returns the same 1992-01-02 first bar),
// which is after this scenario's entire 1986-1988 window. This universe
// has 16 instruments (Y01..Y15, Y17); Y17 remains "Y17" (MarketProxy) per
// the controller's explicit instruction not to renumber.
var crash1987FetchSpecs = []FetchSpec{
	{Name: "ko", Symbol: "KO", From: "1970-06-01", To: "1989-06-30"},
	{Name: "mcd", Symbol: "MCD", From: "1970-06-01", To: "2010-06-30"},
	{Name: "dis", Symbol: "DIS", From: "1970-06-01", To: "1989-06-30"},
	{Name: "jnj", Symbol: "JNJ", From: "1970-06-01", To: "1989-06-30"},
	{Name: "pg", Symbol: "PG", From: "1970-06-01", To: "1989-06-30"},
	{Name: "mmm", Symbol: "MMM", From: "1970-06-01", To: "1989-06-30"},
	{Name: "cat", Symbol: "CAT", From: "1970-06-01", To: "1989-06-30"},
	{Name: "ibm", Symbol: "IBM", From: "1970-06-01", To: "2002-03-31"},
	{Name: "hpq", Symbol: "HPQ", From: "1985-06-01", To: "2002-03-31"},
	{Name: "ge", Symbol: "GE", From: "1985-06-01", To: "2010-06-30"},
	{Name: "xom", Symbol: "XOM", From: "1985-06-01", To: "2010-06-30"},
	{Name: "wmt", Symbol: "WMT", From: "1985-06-01", To: "2010-06-30"},
	{Name: "spx", Symbol: "^GSPC", From: "1970-06-01", To: "2010-06-30"},
	// This universe's own symbols, moved off pendingFetchSpecs.
	{Name: "mrk", Symbol: "MRK", From: "1985-06-01", To: "1989-06-30"},
	{Name: "ba", Symbol: "BA", From: "1985-06-01", To: "1989-06-30"},
	{Name: "axp", Symbol: "AXP", From: "1985-06-01", To: "1989-06-30"},
}

var crash1987Universe = ScenarioUniverse{
	ScenarioID:  "crash-1987",
	Name:        "1987 黑色星期一",
	RealPeriod:  "1986-01 ~ 1988-12",
	EraHint:     "类似程序化交易与并购热潮推起大牛市的年代",
	WindowStart: "1986-01-02",
	WindowEnd:   "1988-12-30",
	MarketProxy: "Y17",
	FetchSpecs:  crash1987FetchSpecs,
	Sectors: []SectorSpec{
		{"IND", "工业制造"}, {"CONS", "消费品牌"}, {"PHRM", "医药"},
		{"TECH", "计算机"}, {"FIN", "金融"}, {"ENGY", "能源"}, {"RETL", "零售"},
	},
	Macros: []SectorSpec{{"GOLD", "黄金"}, {"OIL", "原油"}, {"RATE", "利率"}},
	KeyWindows: []DateWindow{
		{"1987-01-02", "1987-08-25", 1},  // 疯牛期，压制逆势冷水
		{"1987-10-14", "1987-10-30", -1}, // 崩盘周
	},
	Instruments: []InstrumentSpec{
		{ID: "Y01", Raw: "ibm", Sector: "TECH",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.3},
			Dossier: Dossier{Alias: "蓝色巨人", Desc: "百年计算公司", RealName: "IBM",
				Business: "大型机、系统与服务三驾马车，企业后台离不开它的机器。",
				Bull:     "并购与回购热潮里现金牛最抗跌，蓝筹信仰历久弥新。",
				Bear:     "大型机的增长故事讲了太多年，新一代小型机器正在啃它的市场。"}},
		{ID: "Y02", Raw: "hpq", Sector: "TECH",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.3},
			Dossier: Dossier{Alias: "车库仪器", Desc: "老牌硅谷仪器厂", RealName: "Hewlett-Packard",
				Business: "从实验室仪器起家，如今靠打印机与工作站两条腿走路。",
				Bull:     "工程师文化做出来的产品口碑扎实，办公室自动化浪潮才刚开始。",
				Bear:     "仪器业务增长见顶，新业务能否接棒还是问号。"}},
		{ID: "Y03", Raw: "ko", Sector: "CONS",
			ExtraBeta: map[string]float64{"SENT": 0.45, "RATE": -0.25},
			Dossier: Dossier{Alias: "快乐水业", Desc: "全球装瓶的饮料霸主", RealName: "Coca-Cola",
				Business: "一瓶糖水卖遍全球的品牌机器，装瓶网络深入每个国家的毛细血管。",
				Bull:     "品牌就是复利：只要人还口渴，提价权与全球化就是双引擎，牛市里机构最安心的核心持仓。",
				Bear:     "所有人都安心的持仓，估值早已不便宜；当组合保险开始机械抛售时，最拥挤的地方跌得最快。"}},
		{ID: "Y04", Raw: "pg", Sector: "CONS",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.25},
			Dossier: Dossier{Alias: "日用大王", Desc: "百年日化帝国", RealName: "Procter & Gamble",
				Business: "牙膏、洗衣粉、纸尿裤——货架上一堆你叫不出集团名字的日用品都是它的。",
				Bull:     "消费品里最无聊也最稳的现金流，牛市里当压舱石照样跟涨。",
				Bear:     "增长全靠货架份额一点点抢，讲不出让人兴奋的新故事。"}},
		{ID: "Y05", Raw: "mcd", Sector: "CONS",
			ExtraBeta: map[string]float64{"SENT": 0.45, "RATE": -0.25},
			Dossier: Dossier{Alias: "金拱门快餐", Desc: "全球连锁快餐之王", RealName: "McDonald's",
				Business: "标准化流程复制到全球的连锁快餐，加盟费与地产租金才是真正的利润引擎。",
				Bull:     "开一家新店就多一份稳定现金流，扩张速度就是股价的发动机。",
				Bear:     "同店销售增速终会撞天花板，加盟商的抱怨迟早传到股东耳朵里。"}},
		{ID: "Y06", Raw: "dis", Sector: "CONS",
			ExtraBeta: map[string]float64{"SENT": 0.45, "RATE": -0.25},
			Dossier: Dossier{Alias: "梦幻影业", Desc: "动画起家的娱乐帝国", RealName: "Disney",
				Business: "老牌动画厂靠乐园门票、周边授权和新片接力撑起收入。",
				Bull:     "新管理层重新点燃创意引擎，乐园涨价照样人潮涌动。",
				Bear:     "内容生意大小年明显，一部大片扑街就要等下一部救场。"}},
		{ID: "Y07", Raw: "mrk", Sector: "PHRM",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.25},
			Dossier: Dossier{Alias: "济世制药", Desc: "研发驱动的大药厂", RealName: "Merck",
				Business: "重金投入研发管线，靠专利期内的爆款处方药收割高毛利。",
				Bull:     "老龄化与慢性病是永续需求，专利护城河让定价权稳如泰山。",
				Bear:     "专利总有到期日，管线一旦断档增长立刻熄火。"}},
		{ID: "Y08", Raw: "jnj", Sector: "PHRM",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.25},
			Dossier: Dossier{Alias: "康婴健护", Desc: "从婴儿爽身粉到手术缝线", RealName: "Johnson & Johnson",
				Business: "婴儿护理、医疗器械、处方药三条业务线同时下注，风险天然分散。",
				Bull:     "东边不亮西边亮，多元业务组合让业绩曲线格外平滑。",
				Bear:     "分散也意味着没有一条业务能真正引领增长，故事讲不出爆点。"}},
		{ID: "Y09", Raw: "mmm", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.45},
			Dossier: Dossier{Alias: "百宝胶业", Desc: "什么都能粘的材料公司", RealName: "3M",
				Business: "从胶带到研磨材料，几千种工业与办公产品共享同一套材料科学底子。",
				Bull:     "产品线极度分散，只要工厂在开工它总有一样东西被买走。",
				Bear:     "工业周期一旦转冷，资本开支收缩会传导到它的每一条产品线。"}},
		{ID: "Y10", Raw: "ba", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.45},
			Dossier: Dossier{Alias: "云际客机", Desc: "民航客机双雄之一", RealName: "Boeing",
				Business: "长周期的商用客机制造，订单排到数年后，交付节奏决定利润节奏。",
				Bull:     "航空旅行需求长期向上，积压订单就是未来好几年的收入保证。",
				Bear:     "订单周期长意味着景气反转要等很久才能显现，且取消订单代价高昂。"}},
		{ID: "Y11", Raw: "cat", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.25, "RATE": -0.5},
			Dossier: Dossier{Alias: "黄铁重工", Desc: "推土机与矿山机械之王", RealName: "Caterpillar",
				Business: "工地与矿场少不了的重型机械，经销商网络铺满全球每一片工地。",
				Bull:     "基建与资源开发是长期趋势，重型设备折旧完了还得再买新的。",
				Bear:     "重资产、高杠杆的周期股，利率一抬头，工地的挖机订单第一个被砍。"}},
		{ID: "Y12", Raw: "ge", Sector: "IND",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.45},
			Dossier: Dossier{Alias: "万象电气", Desc: "什么都造的工业帝国", RealName: "General Electric",
				Business: "发电机、飞机引擎、医疗设备加一个庞大的金融部门，业务横跨所有周期。",
				Bull:     "传奇管理层治下利润从不失手，机构的压舱石首选。",
				Bear:     "金融部门的杠杆是报表深处的暗礁，'从不失手'本身就值得怀疑。"}},
		{ID: "Y13", Raw: "axp", Sector: "FIN",
			ExtraBeta: map[string]float64{"SENT": 0.35, "RATE": -0.5},
			Dossier: Dossier{Alias: "绿卡金融", Desc: "高端签账卡与旅行支票", RealName: "American Express",
				Business: "高端客群的签账卡与旅行支票生意，赚的是商户回佣与浮存金。",
				Bull:     "消费升级的年代，绿色卡片是身份的通行证，会员费提了又提照样排队。",
				Bear:     "利率一抬头，浮存金优势缩水；金融股在恐慌里从没当过避风港。"}},
		{ID: "Y14", Raw: "xom", Sector: "ENGY",
			ExtraBeta: map[string]float64{"SENT": 0.2, "RATE": -0.25, "OIL": 0.6, "GOLD": 0.1},
			Dossier: Dossier{Alias: "磐石石油", Desc: "全球油气巨轮", RealName: "Exxon",
				Business: "从油井到加油站的全产业链油气巨头，规模冠绝全球同行。",
				Bull:     "无论景气与否，人总要开车取暖；恐慌时现金流是最硬的叙事。",
				Bear:     "油价下行周期里巨轮也得随波逐流，牛市狂热年代反而常常跑输大盘。"}},
		{ID: "Y15", Raw: "wmt", Sector: "RETL",
			ExtraBeta: map[string]float64{"SENT": 0.4, "RATE": -0.3},
			Dossier: Dossier{Alias: "平价百货", Desc: "乡镇起家的零售之王", RealName: "Walmart",
				Business: "天天低价的连锁超市帝国，供应链效率碾压一切同行。",
				Bull:     "从乡镇一路开进大城市，门店扩张曲线还远没走完。",
				Bear:     "低毛利模式对成本敏感，扩张太快也容易踩到管理短板。"}},
		{ID: "Y17", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.35},
			Dossier: Dossier{Alias: "大盘五百", Desc: "五百家大公司的市值加权指数", RealName: "S&P 500 指数",
				Business: "五百家大公司的市值加权组合，一键买入整个经济体。",
				Bull:     "不赌个股不赌行业，赌整体经济的长期向上。",
				Bear:     "指数里挤满了同一批热门蓝筹，程序化抛售一来谁都躲不掉。"}},
	},
}

func init() {
	Register(&crash1987Universe)
}
