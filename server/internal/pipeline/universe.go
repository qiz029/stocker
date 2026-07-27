package pipeline

// FetchSpec maps a short local name to a source symbol.
type FetchSpec struct {
	Name   string // rawdata/<Name>.csv
	Symbol string // Yahoo Finance chart-API symbol
}

// FetchList: 16 surviving stocks, 2 indices, 2 macro proxies.
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
// this era and is dropped from FetchList per the pre-authorized
// macro-proxy contingency: the OIL factor keeps curated (non-fitted) beta
// values in Task 5 instead of a regression-fitted one.
var FetchList = []FetchSpec{
	{"msft", "MSFT"}, {"csco", "CSCO"}, {"intc", "INTC"},
	{"orcl", "ORCL"}, {"ibm", "IBM"}, {"aapl", "AAPL"},
	{"amzn", "AMZN"}, {"ebay", "EBAY"}, {"amd", "AMD"},
	{"qcom", "QCOM"}, {"txn", "TXN"}, {"adbe", "ADBE"},
	{"hpq", "HPQ"}, {"ge", "GE"}, {"xom", "XOM"}, {"wmt", "WMT"},
	{"ndx", "^NDX"}, {"spx", "^GSPC"},
	// gold: no free Yahoo history for a bullion spot/future in this era;
	// ^XAU (PHLX Gold/Silver Sector index of gold/silver miners) is used
	// as the GOLD factor proxy instead.
	{"gold", "^XAU"},
	// us10y: ^TNX is the CBOE 10-Year Treasury Note yield index (yield in
	// index points, i.e. 10x the yield in percent), used as the US10Y
	// factor proxy.
	{"us10y", "^TNX"},
}
