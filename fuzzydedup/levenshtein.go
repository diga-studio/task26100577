package fuzzydedup

// Levenshtein 返回两个字符串之间的 Levenshtein 编辑距离。
// 插入、删除、替换的代价均为 1，按 Unicode 码点计算，而不是 UTF-8 字节。
func Levenshtein(a, b string) int {
	return levenshteinRunes([]rune(a), []rune(b))
}

// levenshteinRunes 是 Levenshtein 的码点切片版本，供批处理时复用预解码的码点。
func levenshteinRunes(a, b []rune) int {
	// 保证 a 是较短的一方，作为 Myers 的 pattern。
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(a) <= 64 {
		return myersLevenshtein(a, b)
	}
	return dpLevenshtein(a, b)
}

// myersLevenshtein 使用 Myers 位并行算法，要求 len(pattern) <= 64，
// 时间复杂度 O(len(text))，每步仅常数个 64 位运算。
func myersLevenshtein(pattern, text []rune) int {
	m := len(pattern)
	var ascii [128]uint64
	var wide map[rune]uint64
	for i, r := range pattern {
		bit := uint64(1) << uint(i)
		if r < 128 {
			ascii[r] |= bit
		} else {
			if wide == nil {
				wide = make(map[rune]uint64)
			}
			wide[r] |= bit
		}
	}

	top := uint64(1) << uint(m-1)
	pv := ^uint64(0)
	var mv uint64
	score := m
	for _, r := range text {
		var e uint64
		if r < 128 {
			e = ascii[r]
		} else {
			e = wide[r]
		}
		xv := e | mv
		xh := (((e & pv) + pv) ^ pv) | e
		ph := mv | ^(xh | pv)
		mh := pv & xh
		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}
		ph = (ph << 1) | 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv
	}
	return score
}

// dpLevenshtein 是经典两行动态规划，用于码点长度超过 64 的长串。
func dpLevenshtein(a, b []rune) int {
	prev := make([]int, len(a)+1)
	cur := make([]int, len(a)+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= len(b); j++ {
		cur[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			del := prev[i] + 1
			ins := cur[i-1] + 1
			sub := prev[i-1] + cost
			cur[i] = min3(del, ins, sub)
		}
		prev, cur = cur, prev
	}
	return prev[len(a)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// eqProfile 是 Myers 算法的 pattern 侧预计算表：eq[sym] 给出符号 sym
// 在 pattern 中各位置的位掩码。符号通过批级符号表编码为紧凑的整数，
// 使得内层循环只需一次数组索引。
type eqProfile struct {
	eq     []uint64 // 长度为符号表大小
	length int      // pattern 的码点数
}

func newEqProfile(encoded []uint16, numSymbols int) eqProfile {
	p := eqProfile{eq: make([]uint64, numSymbols), length: len(encoded)}
	for i, sym := range encoded {
		p.eq[sym] |= 1 << uint(i)
	}
	return p
}

// myersDense 使用预计算的 eqProfile 计算编辑距离，text 为符号编码后的另一方。
// 要求 profile.length <= 64。
func myersDense(profile eqProfile, text []uint16) int {
	m := profile.length
	top := uint64(1) << uint(m-1)
	pv := ^uint64(0)
	var mv uint64
	score := m
	for _, sym := range text {
		e := profile.eq[sym]
		xv := e | mv
		xh := (((e & pv) + pv) ^ pv) | e
		ph := mv | ^(xh | pv)
		mh := pv & xh
		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}
		ph = (ph << 1) | 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv
	}
	return score
}
