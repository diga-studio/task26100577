package dedup

import "math"

// fieldSimilarity 由编辑距离与较长字段长度计算相似度（百分制）。
// 两字段均为空时约定相似度为 100。
func fieldSimilarity(dist, maxLen int) float64 {
	if maxLen == 0 {
		return 100
	}
	return float64(maxLen-dist) * 100 / float64(maxLen)
}

// combinedScore 是两字段相似度的算术平均值。
func combinedScore(nameSim, addrSim float64) float64 {
	return (nameSim + addrSim) / 2
}

// maxFieldDist 返回单字段相似度达到 threshold 所允许的最大编辑距离。
// 即 floor(maxLen*(100-threshold)/100)，用整数放大避免浮点下取整误差。
func maxFieldDist(maxLen int, threshold float64) int {
	if threshold == math.Trunc(threshold) {
		return maxLen * (100 - int(threshold)) / 100
	}
	return int(math.Floor(float64(maxLen) * (100 - threshold) / 100))
}

// maxPairFieldDist 返回综合分数达标时单字段允许的最大编辑距离上界。
// 综合达标要求 dn/Ln + da/La <= 2*(100-threshold)/100，
// 因此任一字段的距离都不超过 floor(maxLen*2*(100-threshold)/100)。
func maxPairFieldDist(maxLen int, threshold float64) int {
	if threshold == math.Trunc(threshold) {
		return maxLen * 2 * (100 - int(threshold)) / 100
	}
	return int(math.Floor(float64(maxLen) * 2 * (100 - threshold) / 100))
}

// maxOtherDist 在已知一字段长度 Lself 与距离 dSelf 时，
// 返回另一字段（长度 Lother）仍可能使综合分数达标的最大编辑距离。
// 返回值小于 0 表示无论另一字段如何都不可能达标。
func maxOtherDist(Lself, dSelf, Lother int, threshold float64) int {
	// 达标条件：50*((Ls-ds)*Lo + (Lo-do)*Ls) >= T*Ls*Lo
	// 解出 do <= Lo - (T*Ls*Lo - 50*(Ls-ds)*Lo) / (50*Ls)
	if threshold == math.Trunc(threshold) {
		num := int64(int(threshold))*int64(Lself)*int64(Lother) -
			50*int64(Lself-dSelf)*int64(Lother)
		den := int64(50 * Lself)
		if num <= 0 {
			return Lother
		}
		ceil := (num + den - 1) / den
		return Lother - int(ceil)
	}
	rhs := (threshold*float64(Lself)*float64(Lother) -
		50*float64(Lself-dSelf)*float64(Lother)) / (50 * float64(Lself))
	return Lother - int(math.Ceil(rhs))
}

// meetsThreshold 用未舍入的精确值判定综合分数是否达到 threshold。
// 整数阈值下比较 50*((Ln-dn)*La+(La-da)*Ln) 与 T*Ln*La，全部为整数运算，
// 因此恰好等于阈值（如 85.0）必然命中。
func meetsThreshold(Ln, dn, La, da int, threshold float64) bool {
	if threshold == math.Trunc(threshold) {
		lhs := 50 * ((Ln-dn)*La + (La-da)*Ln)
		rhs := int(threshold) * Ln * La
		return lhs >= rhs
	}
	score := 50 * (float64(Ln-dn)/float64(Ln) + float64(La-da)/float64(La))
	return score >= threshold
}

// distMeetsThreshold 判断字段距离 d 是否满足单字段相似度达到 threshold，
// 即 d <= floor(L*(100-threshold)/100)，与相似度判定完全等价。
func distMeetsThreshold(L, d int, threshold float64) bool {
	return d <= maxFieldDist(L, threshold)
}
