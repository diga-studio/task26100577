package dedup

// editDistance 计算两个码点序列的 Levenshtein 距离（增删改代价均为 1）。
// 时间与空间复杂度为 O(len(a)*len(b)) 与 O(min(len(a),len(b)))。
func editDistance(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	// 现在 len(a) >= len(b)，滚动数组长度为 len(b)+1。
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minInt3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// boundedEditDistance 在距离超过 maxDist 时提前返回 -1，否则返回精确距离。
// 利用 |len(a)-len(b)| 下界与带状 DP 剪枝，代价为 O(maxDist*min(len(a),len(b)))。
func boundedEditDistance(a, b []rune, maxDist int) int {
	if maxDist < 0 {
		return -1
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	// 现在 len(a) >= len(b)。
	if len(a)-len(b) > maxDist {
		return -1
	}
	if len(b) == 0 {
		if len(a) <= maxDist {
			return len(a)
		}
		return -1
	}
	if maxDist == 0 {
		if len(a) == 0 {
			return 0
		}
		for i := range a {
			if a[i] != b[i] {
				return -1
			}
		}
		return 0
	}
	n := len(b)
	const inf = 1 << 30
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		// 带范围：j ∈ [i-maxDist, i+maxDist]，之外的距离必超过 maxDist。
		lo := i - maxDist
		if lo < 1 {
			lo = 1
		}
		hi := i + maxDist
		if hi > n {
			hi = n
		}
		if lo > hi {
			return -1
		}
		if lo > 1 {
			curr[lo-1] = inf
		}
		curr[0] = i
		rowMin := inf
		for j := lo; j <= hi; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := inf
			if j-1 >= lo-1 && j-1 <= n {
				del = curr[j-1] + 1
			}
			ins := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt3(del, ins, sub)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		if hi < n {
			curr[hi+1] = inf
		}
		if rowMin > maxDist {
			return -1
		}
		prev, curr = curr, prev
	}
	if prev[n] > maxDist {
		return -1
	}
	return prev[n]
}

func minInt3(a, b, c int) int {
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
