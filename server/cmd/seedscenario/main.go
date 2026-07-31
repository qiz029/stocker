// Command seedscenario writes the built-in synthetic scenario into
// Postgres so rooms can be created against it during development.
// Plan 4's data pipeline replaces this with real historical scenarios.
//
//	DATABASE_URL=postgres://localhost/stocker?sslmode=disable go run ./cmd/seedscenario
package main

import (
	"context"
	"log"
	"os"

	"github.com/toddzheng/stocker/server/internal/scenario"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	sc := scenario.Synthetic()
	if err := store.SaveScenario(ctx, pool, sc); err != nil {
		log.Fatalf("save scenario: %v", err)
	}
	display := map[string]store.InstrumentDisplay{
		"S1": {Alias: "Ridgeline Networks", Aliases: []string{"Ridgeline Networks", "Vantor Networks", "Copperline Communications"},
			Desc:       "网络设备巨头，泡沫叙事的旗手",
			DescEn:     "Networking-equipment giant, flag-bearer of the bubble narrative",
			Business:   "路由器与交换机占营收七成，其余来自企业网络服务合约。客户遍布电信运营商、门户网站与新兴的宽带服务商——换句话说，它的客户就是整个新经济。",
			BusinessEn: "Routers and switches make up seventy percent of revenue, the rest coming from enterprise network-service contracts. Its customers span telecom carriers, web portals, and upstart broadband providers — in other words, its customer is the entire new economy.",
			Bull:       "只要还有人往网上搬业务，它的订单就不会停。多头把它类比成淘金潮里卖铲子的人：不管哪家网站赢，铲子都得从它这买。",
			BullEn:     "As long as anyone keeps moving business online, its orders won't stop. Bulls liken it to the shovel seller in a gold rush: no matter which website wins, the shovels get bought from it.",
			Bear:       "客户集中在烧钱的互联网公司——如果融资环境收紧，下游资本开支会先于一切崩塌。此外，估值已把未来十年的增长全部计入。",
			BearEn:     "Its customers are concentrated among cash-burning internet companies — if funding conditions tighten, downstream capital spending collapses before everything else. On top of that, the valuation already prices in the next decade of growth."},
		"S2": {Alias: "Crossway Media", Aliases: []string{"Crossway Media", "Portalpoint Media", "Bannerline Interactive"},
			Desc:       "流量入口，人人都从这里上网",
			DescEn:     "A traffic gateway where everyone starts their internet session",
			Business:   "门户页面广告位销售为主，附带邮箱、搜索与社区服务。用户时长是它对广告主报价的全部底气。",
			BusinessEn: "Portal-page ad slots are the main business, with email, search, and community services on the side. User time-on-site is the entire basis of its rate card to advertisers.",
			Bull:       "互联网人口每季度都在膨胀，而入口只有几个。眼球即货币，它是铸币厂。",
			BullEn:     "The internet population swells every quarter, and there are only a few gateways. Eyeballs are currency, and it is the mint.",
			Bear:       "广告主大多也是互联网创业公司——泡沫内循环。用户忠诚度存疑：换个主页只需要三秒钟。",
			BearEn:     "Most of its advertisers are internet startups themselves — a bubble circulating within itself. User loyalty is doubtful: switching homepages takes three seconds."},
		"S3": {Alias: "Summit Commerce Group", Aliases: []string{"Summit Commerce Group", "Cartwheel Commerce", "Everyday Emporium"},
			Desc:       "烧钱换增长的电商先驱",
			DescEn:     "An e-commerce pioneer burning cash for growth",
			Business:   "线上零售平台，从图书起家扩张到全品类。自建仓储物流，每单都亏钱，但每季度单量都创新高。",
			BusinessEn: "An online retail platform that grew from books into every category. It builds its own warehousing and logistics, loses money on every order, yet sets order-volume records every quarter.",
			Bull:       "零售的未来在线上，先烧钱圈地者赢者通吃。今天的亏损是明天垄断的门票。",
			BullEn:     "The future of retail is online, and whoever burns cash to grab land first takes all. Today's losses are the ticket to tomorrow's monopoly.",
			Bear:       "现金消耗率惊人，命脉握在资本市场手里。一旦融资窗口关闭，增长故事会在一个季度内变成清算故事。",
			BearEn:     "The cash burn is staggering, and its lifeline is in the capital markets' hands. Once the funding window closes, the growth story becomes a liquidation story within a quarter."},
		"S4": {Alias: "Swiftcore Semiconductor", Aliases: []string{"Swiftcore Semiconductor", "Quickchip Semiconductor", "Nimblewafer Systems"},
			Desc:       "为新经济供货的芯片厂",
			DescEn:     "A chip maker supplying the new economy",
			Business:   "网络处理器与通信芯片设计制造。下游是服务器、路由器与个人电脑厂商。",
			BusinessEn: "Design and manufacturing of network processors and communications chips. Downstream are server, router, and PC makers.",
			Bull:       "半导体是数字时代的石油。产能供不应求，涨价函比财报先到。",
			BullEn:     "Semiconductors are the oil of the digital age. Capacity can't meet demand, and price-hike letters arrive before the earnings reports.",
			Bear:       "半导体从来是周期行业——库存周期一旦掉头，'订单排到明年'会变成'砍单砍到明年'。",
			BearEn:     "Semiconductors have always been cyclical — once the inventory cycle turns, 'booked into next year' becomes 'cancellations into next year'."},
		"S5": {Alias: "Keystone Software", Aliases: []string{"Keystone Software", "Lockstep Software", "Ironquill Systems"},
			Desc:       "企业上网潮的军火商",
			DescEn:     "Arms dealer of the corporate stampede online",
			Business:   "企业级数据库与电商中间件授权，配套实施顾问服务。签单模式：一次性授权费 + 年度维护费。",
			BusinessEn: "Enterprise database and e-commerce middleware licenses, with implementation consulting on the side. Deal model: one-time license fee plus annual maintenance.",
			Bull:       "'触网'是所有 CEO 的年度关键词，预算无上限。它的销售漏斗就是整个财富五百强名单。",
			BullEn:     "'Get online' is every CEO's keyword of the year, with unlimited budgets. Its sales funnel is the entire Fortune 500 list.",
			Bear:       "授权收入一次性确认，增长依赖不断找到新客户。当'该上网的都上完了'，增长引擎会突然熄火。",
			BearEn:     "License revenue is recognized all at once, so growth depends on constantly finding new customers. When 'everyone who should be online already is', the growth engine stalls abruptly."},
		"S6": {Alias: "Oldfield Energy", Aliases: []string{"Oldfield Energy", "Steadyflow Energy", "Blueflame Resources"},
			Desc:       "现金流稳健的传统油气",
			DescEn:     "A traditional oil-and-gas company with steady cash flow",
			Business:   "上游油气开采与管道运输，长期供销合约锁定大部分产量。资本开支保守，分红率常年行业前列。",
			BusinessEn: "Upstream oil-and-gas production and pipeline transport, with long-term supply contracts locking in most output. Capital spending is conservative and the payout ratio perennially leads the industry.",
			Bull:       "无论线上线下，人总要开车取暖。市场恐慌时，现金流就是最硬的叙事。",
			BullEn:     "Online or offline, people still drive and heat their homes. When the market panics, cash flow is the hardest narrative.",
			Bear:       "增长天花板肉眼可见，油价下行周期里分红也难保。在狂热年代，它的股价可能长期跑输大盘。",
			BearEn:     "The growth ceiling is visible to the naked eye, and even the dividend is at risk in an oil-price downcycle. In manic years its stock can lag the market for a long time."},
		"S7": {Alias: "Holloway Department Stores", Aliases: []string{"Holloway Department Stores", "Mainstreet Department Stores", "Grand Emporium Group"},
			Desc:       "全国连锁的百货集团",
			DescEn:     "A nationwide department-store chain",
			Business:   "全国数百家门店的连锁百货，自有品牌占比逐年提升，会员体系贡献一半复购。",
			BusinessEn: "A department-store chain with hundreds of stores nationwide; private labels gain share every year, and the membership program drives half of repeat purchases.",
			Bull:       "电商吵得再凶，九成五的零售额仍发生在线下。它便宜、赚钱、还在回购股票。",
			BullEn:     "However loud e-commerce gets, ninety-five percent of retail sales still happen offline. It is cheap, profitable, and buying back stock.",
			Bear:       "同店增速逐年放缓，年轻客群流失。它是电商故事里被指名道姓的'被颠覆者'。",
			BearEn:     "Same-store growth slows year by year and younger customers are drifting away. It is the named 'disruption target' in every e-commerce story."},
		"S8": {Alias: "Amalgamated Industries", Aliases: []string{"Amalgamated Industries", "United General Industries", "Farflung Enterprises"},
			Desc:       "多元化经营的工业集团",
			DescEn:     "A diversified industrial conglomerate",
			Business:   "发电设备、航空部件、医疗器械加一个不小的金融部门。业务横跨周期，东方不亮西方亮。",
			BusinessEn: "Power-generation equipment, aircraft components, medical devices, plus a not-so-small finance arm. The businesses span cycles — when one dims, another shines.",
			Bull:       "分散即防御。当单一赛道的故事破灭时，资金会回流到这种什么都做一点的巨轮上。",
			BullEn:     "Diversification is defense. When single-sector stories burst, capital rotates back into supertankers that do a bit of everything.",
			Bear:       "多元化也意味着哪个业务都不性感，管理层被批'什么都做，什么都不精'。金融部门的杠杆是报表深处的暗礁。",
			BearEn:     "Diversification also means no business is sexy; management is criticized for 'doing everything, mastering nothing'. The finance arm's leverage is a reef deep in the statements."},
	}
	if err := store.SetInstrumentDisplay(ctx, pool, sc.ID, display); err != nil {
		log.Fatalf("set instrument display: %v", err)
	}
	if err := store.SetScenarioMeta(ctx, pool, sc.ID, "合成测试剧本", "Synthetic Test Scenario", ""); err != nil {
		log.Fatalf("set scenario meta: %v", err)
	}
	log.Printf("seeded scenario %q (%d instruments, %d days)", sc.ID, len(sc.Instruments), sc.Days)
	log.Printf("applied display profiles for %d instruments", len(display))
}
