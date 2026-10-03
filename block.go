package dedup

import (
	"slices"
	"sort"
)

// 字段标识：名称或地址。
const (
	fieldName = iota
	fieldAddress
)

// recordData 是记录的预处理形式：码点序列与字段 bigram 签名。
type recordData struct {
	id          string
	name        []rune
	address     []rune
	nameBigrams []uint32
	addrBigrams []uint32
}

func (r *recordData) fieldLen(field int) int {
	if field == fieldName {
		return len(r.name)
	}
	return len(r.address)
}

func (r *recordData) fieldGrams(field int) []uint32 {
	if field == fieldName {
		return r.nameBigrams
	}
	return r.addrBigrams
}

func (r *recordData) fieldRunes(field int) []rune {
	if field == fieldName {
		return r.name
	}
	return r.address
}

// prepareRecords 把输入记录预处理为内部形式，bigram 经整数化以便快速比较。
func prepareRecords(records []Record) []recordData {
	intern := map[[2]rune]uint32{}
	recs := make([]recordData, len(records))
	for i, rec := range records {
		name := []rune(rec.Name)
		addr := []rune(rec.Address)
		recs[i] = recordData{
			id:          rec.ID,
			name:        name,
			address:     addr,
			nameBigrams: bigramIDs(name, intern),
			addrBigrams: bigramIDs(addr, intern),
		}
	}
	return recs
}

// bigramIDs 返回字段的 bigram 多重集（升序、保留重复），每个 bigram 映射为整数。
func bigramIDs(s []rune, intern map[[2]rune]uint32) []uint32 {
	if len(s) < 2 {
		return nil
	}
	grams := make([]uint32, len(s)-1)
	for i := 0; i+1 < len(s); i++ {
		key := [2]rune{s[i], s[i+1]}
		id, ok := intern[key]
		if !ok {
			id = uint32(len(intern))
			intern[key] = id
		}
		grams[i] = id
	}
	sort.Slice(grams, func(a, b int) bool { return grams[a] < grams[b] })
	return grams
}

// bigramOverlap 计算两个升序 bigram 多重集的交集大小。
func bigramOverlap(a, b []uint32) int {
	common := 0
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			common++
			i++
			j++
		}
	}
	return common
}

// passesCountFilter 是编辑距离不超过 maxD 的必要条件（bigram 计数过滤）。
func passesCountFilter(a, b []uint32, maxD int) bool {
	need := max(len(a), len(b)) - 2*maxD
	if need <= 0 {
		return true
	}
	return bigramOverlap(a, b) >= need
}

// findFieldPairs 用前缀过滤（ppjoin 风格、按编辑距离改造）找出指定字段上
// 相似度可能达到 threshold 的全部记录对，通过 visit 流式回调，不物化候选集。
// 该过程是完备的：任何单字段相似度达标的记录对都会被访问到。
func findFieldPairs(recs []recordData, field int, threshold float64, visit func(i, j int)) {
	n := len(recs)
	if n < 2 {
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return recs[order[a]].fieldLen(field) < recs[order[b]].fieldLen(field)
	})
	maxLen := recs[order[n-1]].fieldLen(field)
	maxDGlobal := maxFieldDist(maxLen, threshold)

	// 全局文档频率，用于把稀有 bigram 排进前缀；gram 编号稠密，用切片代替 map。
	df := map[uint32]int{}
	for i := range recs {
		grams := recs[i].fieldGrams(field)
		for k := 0; k < len(grams); k++ {
			if k == 0 || grams[k] != grams[k-1] {
				df[grams[k]]++
			}
		}
	}
	maxGram := uint32(0)
	for g := range df {
		if g > maxGram {
			maxGram = g
		}
	}
	rank := make([]int32, maxGram+1)
	for g, c := range df {
		rank[g] = int32(c)
	}

	type posting struct {
		idx   int32
		count int32
	}
	index := map[uint32][]posting{}
	indexed := make([]int, 0, n) // 全部已入索引的记录，供退化情形扫描
	acc := make([]int32, n)
	touched := make([]int, 0, 256)

	for _, i := range order {
		li := recs[i].fieldLen(field)
		grams := recs[i].fieldGrams(field)
		qi := len(grams)
		maxD := maxFieldDist(li, threshold)
		required := qi - 2*maxD // 计数过滤阈值：|A∩B| >= qi - 2*maxD

		if required <= 0 || qi == 0 {
			// 计数过滤退化为恒真，扫描全部已索引记录（仅短字段出现）。
			for _, j := range indexed {
				if li-recs[j].fieldLen(field) <= maxD {
					visit(j, i)
				}
			}
		} else {
			// 按稀有度排序的 bigram 序列（同频时按编号，保证相同 gram 相邻）。
			ppos := qi - required + 1
			if ppos > qi {
				ppos = qi
			}
			rare := make([]uint32, qi)
			copy(rare, grams)
			slices.SortFunc(rare, func(a, b uint32) int {
				if rank[a] != rank[b] {
					return int(rank[a]) - int(rank[b])
				}
				return int(a) - int(b)
			})
			prefix := rare[:ppos]

			// 探测：累加每个已索引记录与当前记录前缀的交集大小。
			touched = touched[:0]
			for k := 0; k < len(prefix); {
				g := prefix[k]
				ci := int32(1)
				for k+1 < len(prefix) && prefix[k+1] == g {
					k++
					ci++
				}
				k++
				for _, p := range index[g] {
					j := int(p.idx)
					if acc[j] == 0 {
						touched = append(touched, j)
					}
					if ci < p.count {
						acc[j] += ci
					} else {
						acc[j] += p.count
					}
				}
			}
			for _, j := range touched {
				// 前缀相交即成为候选（鸽笼保证完备）；
				// 再用完整多重集做计数过滤与长度过滤。
				if acc[j] >= 1 &&
					li-recs[j].fieldLen(field) <= maxD &&
					passesCountFilter(grams, recs[j].fieldGrams(field), maxD) {
					visit(j, i)
				}
				acc[j] = 0
			}

			// 把当前记录的前缀写入索引；索引前缀按全局最大距离取，
			// 保证与任意更长记录的探测前缀满足鸽笼完备性。
			ipos := 2*maxDGlobal + 1
			if ipos > qi {
				ipos = qi
			}
			for k := 0; k < ipos; {
				g := rare[k]
				c := int32(1)
				for k+1 < ipos && rare[k+1] == g {
					k++
					c++
				}
				k++
				index[g] = append(index[g], posting{idx: int32(i), count: c})
			}
		}
		indexed = append(indexed, i)
	}
}
