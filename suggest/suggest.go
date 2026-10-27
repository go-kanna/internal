// Package suggest finds the name a misspelled one was probably meant to be.
package suggest

// Closest returns the candidate nearest to name when one is near enough to be a
// misspelling of it: an edit or two away, fewer for a short name. Of candidates
// equally near, the first wins.
func Closest(name string, candidates []string) (string, bool) {
	limit := max(1, len(name)/3)

	best, bestDist := "", limit+1
	for _, c := range candidates {
		if d := Distance(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best, bestDist <= limit
}

// Distance is the fewest insertions, deletions, substitutions, and swaps of
// neighbouring bytes that turn a into b. A swap counts as one edit, since that
// is the typo people make.
func Distance(a, b string) int {
	// prev2, prev, and cur are three consecutive rows of the usual table.
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}
