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
		"S1": {Alias: "郊狼网络", Desc: "网络设备巨头，泡沫叙事的旗手",
			Business: "路由器与交换机占营收七成，其余来自企业网络服务合约。客户遍布电信运营商、门户网站与新兴的宽带服务商——换句话说，它的客户就是整个新经济。",
			Bull:     "只要还有人往网上搬业务，它的订单就不会停。多头把它类比成淘金潮里卖铲子的人：不管哪家网站赢，铲子都得从它这买。",
			Bear:     "客户集中在烧钱的互联网公司——如果融资环境收紧，下游资本开支会先于一切崩塌。此外，估值已把未来十年的增长全部计入。"},
		"S2": {Alias: "门户之星", Desc: "流量入口，人人都从这里上网",
			Business: "门户页面广告位销售为主，附带邮箱、搜索与社区服务。用户时长是它对广告主报价的全部底气。",
			Bull:     "互联网人口每季度都在膨胀，而入口只有几个。眼球即货币，它是铸币厂。",
			Bear:     "广告主大多也是互联网创业公司——泡沫内循环。用户忠诚度存疑：换个主页只需要三秒钟。"},
		"S3": {Alias: "网购乐", Desc: "烧钱换增长的电商先驱",
			Business: "线上零售平台，从图书起家扩张到全品类。自建仓储物流，每单都亏钱，但每季度单量都创新高。",
			Bull:     "零售的未来在线上，先烧钱圈地者赢者通吃。今天的亏损是明天垄断的门票。",
			Bear:     "现金消耗率惊人，命脉握在资本市场手里。一旦融资窗口关闭，增长故事会在一个季度内变成清算故事。"},
		"S4": {Alias: "芯速半导", Desc: "为新经济供货的芯片厂",
			Business: "网络处理器与通信芯片设计制造。下游是服务器、路由器与个人电脑厂商。",
			Bull:     "半导体是数字时代的石油。产能供不应求，涨价函比财报先到。",
			Bear:     "半导体从来是周期行业——库存周期一旦掉头，'订单排到明年'会变成'砍单砍到明年'。"},
		"S5": {Alias: "码力软件", Desc: "企业上网潮的军火商",
			Business: "企业级数据库与电商中间件授权，配套实施顾问服务。签单模式：一次性授权费 + 年度维护费。",
			Bull:     "'触网'是所有 CEO 的年度关键词，预算无上限。它的销售漏斗就是整个财富五百强名单。",
			Bear:     "授权收入一次性确认，增长依赖不断找到新客户。当'该上网的都上完了'，增长引擎会突然熄火。"},
		"S6": {Alias: "老树能源", Desc: "现金流稳健的传统油气",
			Business: "上游油气开采与管道运输，长期供销合约锁定大部分产量。资本开支保守，分红率常年行业前列。",
			Bull:     "无论线上线下，人总要开车取暖。市场恐慌时，现金流就是最硬的叙事。",
			Bear:     "增长天花板肉眼可见，油价下行周期里分红也难保。在狂热年代，它的股价可能长期跑输大盘。"},
		"S7": {Alias: "稳健零售", Desc: "全国连锁的百货集团",
			Business: "全国数百家门店的连锁百货，自有品牌占比逐年提升，会员体系贡献一半复购。",
			Bull:     "电商吵得再凶，九成五的零售额仍发生在线下。它便宜、赚钱、还在回购股票。",
			Bear:     "同店增速逐年放缓，年轻客群流失。它是电商故事里被指名道姓的'被颠覆者'。"},
		"S8": {Alias: "环宇工业", Desc: "多元化经营的工业集团",
			Business: "发电设备、航空部件、医疗器械加一个不小的金融部门。业务横跨周期，东方不亮西方亮。",
			Bull:     "分散即防御。当单一赛道的故事破灭时，资金会回流到这种什么都做一点的巨轮上。",
			Bear:     "多元化也意味着哪个业务都不性感，管理层被批'什么都做，什么都不精'。金融部门的杠杆是报表深处的暗礁。"},
	}
	if err := store.SetInstrumentDisplay(ctx, pool, sc.ID, display); err != nil {
		log.Fatalf("set instrument display: %v", err)
	}
	log.Printf("seeded scenario %q (%d instruments, %d days)", sc.ID, len(sc.Instruments), sc.Days)
	log.Printf("applied display profiles for %d instruments", len(display))
}
