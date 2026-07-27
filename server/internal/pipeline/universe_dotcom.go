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
	{Name: "xom", Symbol: "XOM", From: "1985-06-01", To: "2010-06-30"},
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
	RealPeriod:    "1999-01 ~ 2001-12",
	EraHint:       "类似世纪之交科技股狂热",
	WindowStart:   "1999-01-04",
	WindowEnd:     "2001-12-31",
	MarketProxy:   "X22",
	FidelitySeeds: 12,
	FetchSpecs:    dotcomFetchSpecs,
	Sectors: []SectorSpec{
		{"NET", "网络设备"}, {"TELCO", "电信运营"}, {"ECOM", "电商门户"},
		{"CHIP", "半导体"}, {"SOFT", "软件服务"}, {"HW", "硬件整机"}, {"OLD", "传统经济"},
	},
	Macros: []SectorSpec{{"GOLD", "黄金"}, {"OIL", "原油"}, {"RATE", "利率"}},
	KeyWindows: []DateWindow{
		{"2000-03-10", "2000-05-26", -1}, // 见顶崩盘期: 火上浇油可以，泼冷水不行
		{"2000-11-08", "2001-01-05", -1}, // 二次下跌
		{"2001-09-17", "2001-09-28", -1}, // 重开市恐慌
	},
	Instruments: []InstrumentSpec{
		{ID: "X01", Raw: "msft", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "巨硬软件", Desc: "桌面软件的垄断者", RealName: "Microsoft",
				Business: "操作系统与办公套件的授权费像税收一样稳定，正把触角伸向服务器与互联网。",
				Bull:     "每台新电脑都要向它交税，现金堆成山，垄断者的定价权在任何时代都值钱。",
				Bear:     "反垄断阴云笼罩，拆分传闻不断；互联网时代它更像追赶者而非引领者。"}},
		{ID: "X02", Raw: "csco", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.4},
			Dossier: Dossier{Alias: "郊狼网络", Desc: "网络设备巨头，泡沫叙事的旗手", RealName: "Cisco Systems",
				Business: "路由器与交换机的绝对霸主，客户是整个新经济：运营商、门户、企业机房。",
				Bull:     "淘金潮里卖铲子的人——不管哪家网站赢，铲子都得从它这买。",
				Bear:     "客户都在烧风投的钱；融资一断，下游资本开支先于一切崩塌，而估值已透支十年。"}},
		{ID: "X03", Raw: "intc", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "芯际半导", Desc: "处理器行业的王者", RealName: "Intel",
				Business: "个人电脑与服务器处理器的双料霸主，制程领先一代就是护城河。",
				Bull:     "上网热潮就是换机热潮，每一台新电脑的心脏都印着它的标。",
				Bear:     "半导体是周期行业，库存一旦掉头，'供不应求'三个月内变'砍单'。"}},
		{ID: "X04", Raw: "orcl", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "神谕数据", Desc: "企业数据库军火商", RealName: "Oracle",
				Business: "关系数据库的行业标准，电商潮里每个网站背后都要一套它的授权。",
				Bull:     "'触网'是所有 CEO 的年度关键词，而所有网站的地基都是数据库。",
				Bear:     "授权收入一次性确认，增长靠不断找新客户；该上网的都上完了怎么办。"}},
		{ID: "X05", Raw: "ibm", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.2, "RATE": -0.2},
			Dossier: Dossier{Alias: "蓝色巨人", Desc: "百年计算公司", RealName: "IBM",
				Business: "大型机、服务与咨询三驾马车，新经济里做旧经济的生意。",
				Bull:     "企业级信任无可替代，泡沫破了大家还得找它做系统集成。",
				Bear:     "增长平庸，故事乏味，在狂热年代里资金看不上稳健。"}},
		{ID: "X06", Raw: "aapl", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.2},
			Dossier: Dossier{Alias: "果核电脑", Desc: "特立独行的电脑厂", RealName: "Apple",
				Business: "设计驱动的个人电脑，创始人回归后靠一体机打了场翻身仗。",
				Bull:     "品牌信徒式的忠诚度，硬件卖出了奢侈品毛利。",
				Bear:     "市场份额个位数，一款产品失手就是一个财年的灾难。"}},
		{ID: "X07", Raw: "amzn", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 1.0, "RATE": -0.5},
			Dossier: Dossier{Alias: "雨林书店", Desc: "烧钱换增长的万货商店", RealName: "Amazon",
				Business: "从图书扩到全品类的线上零售，自建仓储物流，每单都亏，每季单量都新高。",
				Bull:     "零售的未来在线上，先烧钱圈地者赢者通吃；今天的亏损是明天垄断的门票。",
				Bear:     "现金消耗率惊人，命脉握在资本市场手里；融资窗口一关，增长故事一个季度变清算故事。"}},
		{ID: "X08", Raw: "ebay", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "万人集市", Desc: "全民线上拍卖行", RealName: "eBay",
				Business: "买卖双方自己定价的拍卖平台，轻资产抽佣，罕见地在互联网公司里赚钱。",
				Bull:     "网络效应教科书：买家越多卖家越多，飞轮一旦转起来没人追得上。",
				Bear:     "假货与欺诈是平台的原罪，信任一旦崩塌飞轮就倒转。"}},
		{ID: "X09", Raw: "amd", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "二号芯厂", Desc: "万年老二的芯片挑战者", RealName: "AMD",
				Business: "处理器市场的挑战者，靠性价比和新架构从霸主嘴里抢份额。",
				Bull:     "老二逆袭的故事最性感，新品每赢一次评测股价就跳一级。",
				Bear:     "价格战里毛利被霸主按在地上摩擦，一代产品失手就要卖厂求生。"}},
		{ID: "X10", Raw: "qcom", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "通联芯片", Desc: "无线时代的专利收租人", RealName: "Qualcomm",
				Business: "手机通信标准的核心专利持有者，卖芯片更收授权费，躺着分成。",
				Bull:     "无线互联网是下一波浪潮，每卖一部手机它都抽成。",
				Bear:     "标准之争悬而未决，押错路线专利池就成废纸；估值已按赢者通吃计价。"}},
		{ID: "X11", Raw: "txn", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "仪芯半导", Desc: "模拟芯片的隐形冠军", RealName: "Texas Instruments",
				Business: "从计算器到手机基带的模拟与数字信号芯片，客户遍布所有电子产品。",
				Bear:     "下游消费电子景气一凉，订单立刻跟着凉。",
				Bull:     "不押注单一终端，什么电子设备火它都有份。"}},
		{ID: "X12", Raw: "adbe", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "创意软件", Desc: "设计师的标配工具箱", RealName: "Adobe",
				Business: "图像处理与排版软件的事实标准，网页时代设计需求爆发的直接受益者。",
				Bull:     "每个新网站都需要设计师，每个设计师都得买它的全家桶。",
				Bear:     "工具软件盗版猖獗，增长天花板取决于正版化速度。"}},
		{ID: "X13", Raw: "hpq", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.2},
			Dossier: Dossier{Alias: "车库仪器", Desc: "硅谷车库神话的老牌厂", RealName: "Hewlett-Packard",
				Business: "打印机、个人电脑与服务器的全线硬件厂，打印耗材是隐藏的现金牛。",
				Bull:     "墨盒是比咖啡更暴利的消耗品，装机量就是年金。",
				Bear:     "电脑业务毛利薄如纸，大公司病拖慢每一次转身。"}},
		{ID: "X14", Raw: "ge", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.1, "RATE": -0.4, "OIL": 0.1},
			Dossier: Dossier{Alias: "万象电气", Desc: "什么都造的工业帝国", RealName: "General Electric",
				Business: "发电机、飞机引擎、医疗设备加一个庞大的金融部门，业务横跨所有周期。",
				Bull:     "传奇 CEO 治下二十年利润从不失手，机构的压舱石首选。",
				Bear:     "金融部门的杠杆是报表深处的暗礁，'从不失手'本身就值得怀疑。"}},
		{ID: "X15", Raw: "xom", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": -0.1, "OIL": 0.8, "GOLD": 0.1},
			Dossier: Dossier{Alias: "磐石石油", Desc: "全球油气巨轮", RealName: "ExxonMobil",
				Business: "从油井到加油站的全产业链油气巨头，世纪合并后规模冠绝全球。",
				Bull:     "无论线上线下，人总要开车取暖；恐慌时现金流是最硬的叙事。",
				Bear:     "油价下行周期里巨轮也得随波逐流，狂热年代长期跑输大盘。"}},
		{ID: "X16", Raw: "wmt", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.0, "RATE": -0.2},
			Dossier: Dossier{Alias: "平价百货", Desc: "乡镇起家的零售之王", RealName: "Walmart",
				Business: "天天低价的连锁超市帝国，供应链效率碾压一切同行。",
				Bull:     "九成五的零售仍在线下，它便宜、赚钱、还在扩张。",
				Bear:     "它是电商故事里被指名道姓的'被颠覆者'。"}},
		{ID: "X17", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.4, "RATE": -0.5},
			Anchors: []Anchor{
				{"1999-01-04", 60, 0}, {"1999-06-21", 64, 0}, {"2000-01-03", 44, 0},
				{"2000-07-03", 40, 0}, {"2000-11-01", 18, 0}, {"2001-03-01", 16, 0},
				{"2001-07-02", 15, 0}, {"2001-10-01", 14, 0}, {"2001-12-31", 11.5, 0},
			},
			Dossier: Dossier{Alias: "环声通讯", Desc: "并购成瘾的长途电话帝国", RealName: "WorldCom（重建）",
				Business: "靠连环并购堆出来的长途与数据通信巨头，报表增速全行业最快。",
				Bull:     "互联网流量每百天翻一倍——它自己说的，卖的就是流量的管道。",
				Bear:     "并购停下来的那天，增长从哪来？报表越漂亮的公司越要多问一句。"}},
		{ID: "X18", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 55, 0}, {"1999-11-01", 65, 0}, {"1999-12-09", 80, 0},
				{"2000-01-14", 72, 0}, {"2000-07-03", 58, 0}, {"2000-10-02", 30, 0},
				{"2000-12-01", 17, 0}, {"2001-06-01", 7, 0}, {"2001-12-31", 6.3, 0},
			},
			Dossier: Dossier{Alias: "贝铃设备", Desc: "百年实验室拆出的设备商", RealName: "Lucent（重建）",
				Business: "从传奇实验室分拆的电信设备商，光网络与交换机订单排到明年。",
				Bull:     "运营商军备竞赛的最大军火商，手握全行业最深的技术储备。",
				Bear:     "为了冲营收向客户放贷卖货——客户还不上钱时，营收和坏账一起爆。"}},
		{ID: "X19", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 12, 0}, {"1999-12-01", 25, 0}, {"2000-07-17", 87, 0},
				{"2000-10-02", 60, 0}, {"2001-02-01", 32, 0}, {"2001-06-01", 11, 0},
				{"2001-09-17", 5.5, 0}, {"2001-12-31", 7.5, 0},
			},
			Dossier: Dossier{Alias: "北极星网络", Desc: "北方来的光网络之王", RealName: "Nortel（重建）",
				Business: "光传输设备的领跑者，骨干网扩容潮里订单接到手软。",
				Bull:     "带宽需求没有尽头，光纤铺到哪它卖到哪。",
				Bear:     "客户全是举债铺网的运营商；产能按泡沫顶点规划，退潮时最先搁浅。"}},
		{ID: "X20", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.6},
			Anchors: []Anchor{
				{"1999-01-04", 22, 0}, {"1999-05-03", 60, 0}, {"2000-01-03", 50, 0},
				{"2000-09-01", 30, 0}, {"2001-01-02", 14, 0}, {"2001-06-01", 8, 0},
				{"2001-10-01", 1.9, 0}, {"2001-12-31", 0.85, 0},
			},
			Dossier: Dossier{Alias: "环洋光缆", Desc: "海底光缆狂想家", RealName: "Global Crossing（重建）",
				Business: "举债在大洋底铺光缆的全球网络运营商，资产是几万公里的海底玻璃。",
				Bull:     "把五大洲连起来的人收全世界的过路费。",
				Bear:     "光缆铺得越多带宽越不值钱；债务是刚性的，带宽价格不是。"}},
		{ID: "X21", Raw: "ndx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.6},
			Dossier: Dossier{Alias: "新经济一篮子", Desc: "科技百强指数基金", RealName: "NASDAQ-100 指数",
				Business: "一键买入整个新经济：一篮子科技龙头的指数化组合。",
				Bull:     "选不出赢家就全买，时代的贝塔好过个股的阿尔法。",
				Bear:     "篮子里装的全是同一个故事，泡沫破时没有分散可言。"}},
		{ID: "X22", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.3},
			Dossier: Dossier{Alias: "大盘五百", Desc: "全市场指数基金", RealName: "S&P 500 指数",
				Business: "五百家大公司的市值加权组合，美国经济本身。",
				Bull:     "不赌行业不赌个股，赌国运。",
				Bear:     "科技权重已被泡沫吹到历史高位，'分散'没有看上去那么分散。"}},
	},
}

func init() {
	Register(&dotcomUniverse)
}
