package engine

import (
	"fmt"
	"math"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

const (
	minReturnCorr = 0.85
	extremumSlack = 10
	// 近似横盘的标的没有"大势"可保真：净变动小于 minNetLogForDirection 时
	// 跳过方向检查（相关性与极值检查仍然生效）。spec §4.6 的方向条款保护
	// 的是历史大势（如纳指 −78% 这种量级的走势），不是噪声符号。阈值收紧至
	// 0.13（final review amendment, human-authorized）：豁免带仅覆盖真正
	// 无趋势的净变动——dotcom 全域中，唯一落在带内的是 AMD（净变动
	// +0.125），纳指（−0.162）与 GE（+0.179）仍在保护范围内，方向检查
	// 对二者继续严格生效。
	minNetLogForDirection = 0.13
	// 双子极值等价容差（round-4/5 amendments, task-5-report.md）：spec §4.6
	// 的极值条款要求历史峰/谷日在 ±extremumSlack 内"仍为局部极值"，并未要求
	// display 的全局 argmax 日等于基线的。真实数据常有相隔数百天、仅差
	// 千分之几的双子极值（如 IBM 双底 0.23%/473 天），任何非零扰动都会以
	// 接近抛硬币的概率翻转全局 argmax——这不破坏历史叙事。因此 argmax 位移
	// 超出 slack 时，若 display 的极值落在基线另一处几乎等高（容差内）的
	// 真实极值上，且基线极值日邻域内 display 仍是近极值，则通过。
	//
	// extremumValueTol bounds how far a displaced extremum's baseline value
	// may sit below the true extreme (multiplicative, applied symmetrically
	// in both sub-conditions of twinExtremumOK). 0.18 ≈ 2× the spec's §4.2
	// p95 deviation (中位~3%, p95~9%, 最大~26%): displacements inside the
	// sanctioned perturbation envelope are "深一点浅一点" (spec §4.6), not
	// infidelity. Extremes MORE prominent than the envelope (the actual
	// 大势 — e.g. the NDX bubble top, where every non-peak-region day sits
	// 40%+ below the peak) remain strictly pinned by this same condition.
	extremumValueTol = 0.18
)

func logReturns(p []scenario.OHLC) []float64 {
	r := make([]float64, len(p)-1)
	for i := 1; i < len(p); i++ {
		r[i-1] = math.Log(p[i].Close / p[i-1].Close)
	}
	return r
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		sa += a[i]
		sb += b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
		sab += a[i] * b[i]
	}
	cov := sab - sa*sb/n
	va := saa - sa*sa/n
	vb := sbb - sb*sb/n
	return cov / math.Sqrt(va*vb)
}

func argExtremum(p []scenario.OHLC, max bool) int {
	best := 0
	for i := range p {
		if (max && p[i].Close > p[best].Close) || (!max && p[i].Close < p[best].Close) {
			best = i
		}
	}
	return best
}

// VerifyFidelity enforces spec §4.6: perturbed prices must preserve the
// broad historical narrative. Also used by the data pipeline (plan 4) as a
// scenario acceptance gate.
func VerifyFidelity(sc *scenario.Scenario, prices map[string][]scenario.OHLC) error {
	for _, inst := range sc.Instruments {
		base := sc.Baseline[inst.ID]
		disp, ok := prices[inst.ID]
		if !ok {
			return fmt.Errorf("%s: missing prices", inst.ID)
		}
		if len(disp) != len(base) {
			return fmt.Errorf("%s: length mismatch (base=%d, disp=%d)", inst.ID, len(base), len(disp))
		}
		if len(base) < 2 {
			return fmt.Errorf("%s: too few days (%d) to verify fidelity", inst.ID, len(base))
		}
		corr := pearson(logReturns(base), logReturns(disp))
		if math.IsNaN(corr) {
			return fmt.Errorf("%s: return correlation is NaN (degenerate series)", inst.ID)
		}
		if corr < minReturnCorr {
			return fmt.Errorf("%s: return correlation %.3f < %.2f", inst.ID, corr, minReturnCorr)
		}
		baseNet := math.Log(base[len(base)-1].Close / base[0].Close)
		if math.Abs(baseNet) >= minNetLogForDirection {
			bDir := base[len(base)-1].Close >= base[0].Close
			dDir := disp[len(disp)-1].Close >= disp[0].Close
			if bDir != dDir {
				return fmt.Errorf("%s: cumulative direction flipped", inst.ID)
			}
		}
		for _, max := range []bool{true, false} {
			bd, dd := argExtremum(base, max), argExtremum(disp, max)
			if abs(bd-dd) <= extremumSlack {
				continue
			}
			if twinExtremumOK(base, disp, bd, dd, max) {
				continue
			}
			return fmt.Errorf("%s: extremum moved %d days (max=%v)", inst.ID, abs(bd-dd), max)
		}
	}
	return nil
}

// twinExtremumOK implements the twin-extreme equivalence (round-4 amendment,
// see extremumValueTol): the display's global extremum day dd sits outside
// bd ± extremumSlack, but the historical narrative is preserved if BOTH
//
//	(a) the baseline close on dd is within extremumValueTol of the baseline's
//	    true extreme — the display's extreme sits on a genuine near-equal
//	    historical extreme, not on an arbitrary day; and
//	(b) the display still shows a near-extreme (within extremumValueTol of
//	    its own global extreme) somewhere in bd ± extremumSlack — the
//	    historical extremum day remains a local extreme of the display.
func twinExtremumOK(base, disp []scenario.OHLC, bd, dd int, max bool) bool {
	lo, hi := bd-extremumSlack, bd+extremumSlack
	if lo < 0 {
		lo = 0
	}
	if hi > len(disp)-1 {
		hi = len(disp) - 1
	}
	if max {
		if base[dd].Close < (1-extremumValueTol)*base[bd].Close {
			return false
		}
		best := disp[lo].Close
		for i := lo + 1; i <= hi; i++ {
			if disp[i].Close > best {
				best = disp[i].Close
			}
		}
		return best >= (1-extremumValueTol)*disp[dd].Close
	}
	if base[dd].Close > (1+extremumValueTol)*base[bd].Close {
		return false
	}
	best := disp[lo].Close
	for i := lo + 1; i <= hi; i++ {
		if disp[i].Close < best {
			best = disp[i].Close
		}
	}
	return best <= (1+extremumValueTol)*disp[dd].Close
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
