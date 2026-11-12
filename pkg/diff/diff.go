// Package diff compares exact string tokens with a deterministic Myers script.
package diff

import (
	"errors"
	"slices"
)

// Kind is an operation in an edit script.
type Kind uint8

const (
	// Equal consumes and produces the same token.
	Equal Kind = iota
	// Delete consumes a token from the old sequence.
	Delete
	// Insert produces a token in the new sequence.
	Insert
)

// Edit consumes Text from the old sequence, produces it in the new sequence,
// or both for Equal. Tokens are never normalized or split.
type Edit struct {
	Kind Kind
	Text string
}

// ErrLimit means comparison exhausted its work budget. No partial script is
// returned. Budget units count allocated frontier cells, diagonal visits and
// token comparisons, bounding trace storage and the number of operations.
// Comparing one token costs one unit regardless of its byte length; callers
// that need byte limits must also bound their input sizes.
var ErrLimit = errors.New("diff work limit exceeded")

// Lines returns a shortest edit script. Equal-cost frontier choices prefer
// deletion; the exact script is deterministic, including repeated tokens.
// A positive budget is required for nonempty inputs.
func Lines(before, after []string, budget int) ([]Edit, error) {
	n, m := len(before), len(after)
	if n == 0 && m == 0 {
		return nil, nil
	}
	if budget <= 0 {
		return nil, ErrLimit
	}
	remaining := budget
	var trace [][]int
	for d := 0; ; d++ {
		// Check before arithmetic/allocation: the next frontier itself must
		// fit within the budget, even if its first diagonal finds the end.
		if remaining < 1 || d > (remaining-1)/2 {
			return nil, ErrLimit
		}
		width := 2*d + 1
		remaining -= width
		current := make([]int, width)
		trace = append(trace, current)
		previous := func(k int) int {
			if d == 0 {
				return 0
			}
			return trace[d-1][k+d-1]
		}
		for k := -d; k <= d; k += 2 {
			if remaining == 0 {
				return nil, ErrLimit
			}
			remaining--
			x := 0
			if d > 0 {
				if k == -d || k != d && previous(k-1) < previous(k+1) {
					x = previous(k + 1)
				} else {
					x = previous(k-1) + 1
				}
			}
			y := x - k
			for x < n && y < m {
				if remaining == 0 {
					return nil, ErrLimit
				}
				remaining--
				if before[x] != after[y] {
					break
				}
				x++
				y++
			}
			current[k+d] = x
			if x >= n && y >= m {
				return unwind(before, after, trace, d), nil
			}
		}
	}
}

func unwind(before, after []string, trace [][]int, depth int) []Edit {
	x, y := len(before), len(after)
	var out []Edit
	for d := depth; d > 0; d-- {
		k := x - y
		previous := func(k int) int { return trace[d-1][k+d-1] }
		previousK := k - 1
		if k == -d || k != d && previous(k-1) < previous(k+1) {
			previousK = k + 1
		}
		previousX := previous(previousK)
		previousY := previousX - previousK
		for x > previousX && y > previousY {
			x--
			y--
			out = append(out, Edit{Kind: Equal, Text: before[x]})
		}
		if x == previousX {
			y--
			out = append(out, Edit{Kind: Insert, Text: after[y]})
		} else {
			x--
			out = append(out, Edit{Kind: Delete, Text: before[x]})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		out = append(out, Edit{Kind: Equal, Text: before[x]})
	}
	slices.Reverse(out)
	return out
}
