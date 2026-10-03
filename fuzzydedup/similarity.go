package fuzzydedup

// FieldSimilarity 返回单字段相似度：(1 - 编辑距离/较长字段长度) * 100。
// 长度与编辑距离均按 Unicode 码点计算。两个空字段视为完全相等，返回 100。
func FieldSimilarity(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	return fieldSimilarityRunes(ar, br)
}

func fieldSimilarityRunes(a, b []rune) float64 {
	longer := len(a)
	if len(b) > longer {
		longer = len(b)
	}
	if longer == 0 {
		return 100
	}
	d := levenshteinRunes(a, b)
	return (1 - float64(d)/float64(longer)) * 100
}

// Score 表示两条记录的评分结果，均为未舍入的数学值。
type Score struct {
	NameSimilarity    float64
	AddressSimilarity float64
	Total             float64 // NameSimilarity 与 AddressSimilarity 的算术平均值
}

// ScorePair 对两条记录评分：分别计算名称、地址相似度，综合分数为两者的算术平均。
func ScorePair(a, b Record) Score {
	return scoreRunes(
		[]rune(a.Name), []rune(a.Address),
		[]rune(b.Name), []rune(b.Address),
	)
}

func scoreRunes(nameA, addrA, nameB, addrB []rune) Score {
	ns := fieldSimilarityRunes(nameA, nameB)
	as := fieldSimilarityRunes(addrA, addrB)
	return Score{
		NameSimilarity:    ns,
		AddressSimilarity: as,
		Total:             (ns + as) / 2,
	}
}
