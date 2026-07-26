package engine

import (
	"sort"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

type World struct {
	ScenarioID string
	Seed       uint64
	Prices     map[string][]scenario.OHLC
	News       []NewsEvent // 按 Day 升序; 计划 2 据此入库 room_prices / room_news
}

// GenerateWorld builds one room's complete parallel world. Deterministic in
// (scenario, seed); returns an error if the fidelity gate fails, in which
// case the caller retries with a derived seed.
func GenerateWorld(sc *scenario.Scenario, seed uint64) (*World, error) {
	shocks := GenerateShockTimeline(sc, seed)
	states, err := EvolveFactorStates(sc, shocks)
	if err != nil {
		return nil, err
	}
	prices := SynthesizePrices(sc, states, seed)
	if err := VerifyFidelity(sc, prices); err != nil {
		return nil, err
	}
	news := append(append(shocks, HistoricalNews(sc)...), NoiseNews(sc, seed)...)
	FillFallbackCopy(sc, news, seed)
	sort.SliceStable(news, func(i, j int) bool { return news[i].Day < news[j].Day })
	return &World{ScenarioID: sc.ID, Seed: seed, Prices: prices, News: news}, nil
}
