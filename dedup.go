// Package dedup 提供企业数据清洗用的模糊查重能力。
//
// 输入是一批带唯一 ID、名称和地址的记录（上游已完成大小写与空白标准化），
// 输出全部综合分数达标的记录对与由达标关系连通而成的重复簇。
// 批次数据与计算结果均保存在进程内存中，本组件不修改原始记录。
package dedup

import "sort"

// DefaultThreshold 是默认的综合分数达标阈值（百分制）。
const DefaultThreshold = 85.0

// Record 是一条待查重的记录，ID 唯一，Name 与 Address 已由上游标准化。
type Record struct {
	ID      string
	Name    string
	Address string
}

// Pair 是一对达标记录及其各字段相似度与综合分数（均为未舍入的数学值）。
type Pair struct {
	ID1               string
	ID2               string
	NameSimilarity    float64
	AddressSimilarity float64
	Score             float64
}

// Result 是整批查重的结果：全部达标记录对与重复簇。
// Pairs 按 (ID1, ID2) 升序，簇内 ID 升序、簇间按首个 ID 升序，
// 因此结果与输入顺序无关。
type Result struct {
	Threshold float64
	Pairs     []Pair
	Clusters  [][]string
}

// EditDistance 计算两个字符串按 Unicode 码点计的 Levenshtein 编辑距离，
// 插入、删除、替换的代价均为 1。
func EditDistance(a, b string) int {
	return editDistance([]rune(a), []rune(b))
}

// Similarity 计算单字段相似度：1 减去编辑距离与较长字段长度的比值，再乘 100。
// 两个字段均为空时约定为 100。
func Similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	return fieldSimilarity(editDistance(ra, rb), max(len(ra), len(rb)))
}

// ScorePair 对两条记录评分，返回各字段相似度与综合分数（算术平均值）。
func ScorePair(a, b Record) Pair {
	nameSim := Similarity(a.Name, b.Name)
	addrSim := Similarity(a.Address, b.Address)
	return Pair{
		ID1:               a.ID,
		ID2:               b.ID,
		NameSimilarity:    nameSim,
		AddressSimilarity: addrSim,
		Score:             combinedScore(nameSim, addrSim),
	}
}

// FindDuplicates 使用默认阈值 85 对整批记录查重。
func FindDuplicates(records []Record) *Result {
	return FindDuplicatesWithThreshold(records, DefaultThreshold)
}

// FindDuplicatesWithThreshold 使用指定阈值（0,100] 内的百分制分数）查重。
//
// 实现分两趟进行：先在名称字段、再在地址字段用前缀过滤生成
// 单字段可能达标的候选对（由鸽笼原理保证完备），再对每对精确评分。
// 名称趟只输出名称相似度达标的对，地址趟只输出名称不达标
// （因而地址必然达标）的对，两趟互不重叠且并集完备。
func FindDuplicatesWithThreshold(records []Record, threshold float64) *Result {
	res := &Result{Threshold: threshold}
	if len(records) < 2 || threshold > 100 {
		return res
	}
	recs := prepareRecords(records)
	pairIdx := make([][2]int, 0, 1024)
	findFieldPairs(recs, fieldName, threshold, func(i, j int) {
		p, dn, _, ok := verifyPair(recs, i, j, threshold)
		if !ok || !distMeetsThreshold(max(len(recs[i].name), len(recs[j].name)), dn, threshold) {
			return // 名称不达标的对由地址趟负责
		}
		res.Pairs = append(res.Pairs, p)
		pairIdx = append(pairIdx, [2]int{i, j})
	})
	findFieldPairs(recs, fieldAddress, threshold, func(i, j int) {
		if fieldSimMeetsThreshold(recs, i, j, fieldName, threshold) {
			return // 名称已达标的对已在名称趟处理
		}
		p, _, _, ok := verifyPair(recs, i, j, threshold)
		if !ok {
			return
		}
		res.Pairs = append(res.Pairs, p)
		pairIdx = append(pairIdx, [2]int{i, j})
	})
	sort.Slice(res.Pairs, func(a, b int) bool {
		if res.Pairs[a].ID1 != res.Pairs[b].ID1 {
			return res.Pairs[a].ID1 < res.Pairs[b].ID1
		}
		return res.Pairs[a].ID2 < res.Pairs[b].ID2
	})
	res.Clusters = buildClusters(recs, pairIdx)
	return res
}

// verifyPair 对候选记录对精确评分，达标时返回 Pair 与两字段的编辑距离。
func verifyPair(recs []recordData, i, j int, threshold float64) (Pair, int, int, bool) {
	a, b := &recs[i], &recs[j]
	Ln := max(len(a.name), len(b.name))
	La := max(len(a.address), len(b.address))

	// 综合达标要求 dn/Ln + da/La <= 2*(100-T)/100，
	// 因此单字段距离超过 floor(2*(100-T)*L/100) 时必不达标。
	maxDn := maxPairFieldDist(Ln, threshold)
	if !passesCountFilter(a.nameBigrams, b.nameBigrams, maxDn) {
		return Pair{}, 0, 0, false
	}
	dn := boundedEditDistance(a.name, b.name, maxDn)
	if dn < 0 {
		return Pair{}, 0, 0, false
	}
	maxDa := maxOtherDist(Ln, dn, La, threshold)
	if maxDa < 0 || !passesCountFilter(a.addrBigrams, b.addrBigrams, maxDa) {
		return Pair{}, 0, 0, false
	}
	da := boundedEditDistance(a.address, b.address, maxDa)
	if da < 0 || !meetsThreshold(Ln, dn, La, da, threshold) {
		return Pair{}, 0, 0, false
	}
	nameSim := fieldSimilarity(dn, Ln)
	addrSim := fieldSimilarity(da, La)
	id1, id2 := a.id, b.id
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	return Pair{
		ID1:               id1,
		ID2:               id2,
		NameSimilarity:    nameSim,
		AddressSimilarity: addrSim,
		Score:             combinedScore(nameSim, addrSim),
	}, dn, da, true
}

// fieldSimMeetsThreshold 判断两条记录在指定字段上的相似度是否达到 threshold。
func fieldSimMeetsThreshold(recs []recordData, i, j, field int, threshold float64) bool {
	a, b := &recs[i], &recs[j]
	la, lb := a.fieldLen(field), b.fieldLen(field)
	L := max(la, lb)
	maxD := maxFieldDist(L, threshold)
	if la < lb {
		la, lb = lb, la
	}
	if la-lb > maxD {
		return false
	}
	if !passesCountFilter(a.fieldGrams(field), b.fieldGrams(field), maxD) {
		return false
	}
	d := boundedEditDistance(a.fieldRunes(field), b.fieldRunes(field), maxD)
	return d >= 0
}
