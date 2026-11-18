package diff

import (
	"errors"
	"reflect"
	"testing"
)

func TestGoldens(t *testing.T) {
	for _, tc := range []struct {
		old, new []string
		want     []Edit
	}{
		{nil, nil, nil},
		{[]string{""}, nil, []Edit{{Delete, ""}}},
		{nil, []string{""}, []Edit{{Insert, ""}}},
		{[]string{""}, []string{""}, []Edit{{Equal, ""}}},
		{[]string{"a"}, nil, []Edit{{Delete, "a"}}},
		{nil, []string{"a"}, []Edit{{Insert, "a"}}},
		{[]string{"a"}, []string{"b"}, []Edit{{Delete, "a"}, {Insert, "b"}}},
		{[]string{"a", "b"}, []string{"b", "a"}, []Edit{{Delete, "a"}, {Equal, "b"}, {Insert, "a"}}},
		{[]string{"a\n"}, []string{"a"}, []Edit{{Delete, "a\n"}, {Insert, "a"}}},
		{[]string{"a\r\n"}, []string{"a\n"}, []Edit{{Delete, "a\r\n"}, {Insert, "a\n"}}},
		{[]string{"```\n", "}\n", "```"}, []string{"```\n", "new\n", "}\n", "```"}, []Edit{{Equal, "```\n"}, {Insert, "new\n"}, {Equal, "}\n"}, {Equal, "```"}}},
	} {
		got, err := Lines(tc.old, tc.new, 10000)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q -> %q: %+v, %v; want %+v", tc.old, tc.new, got, err, tc.want)
		}
	}
}

func TestExhaustiveSmallScriptsAreShortestAndApply(t *testing.T) {
	sequences := [][]string{nil}
	for length := 1; length <= 5; length++ {
		for bits := 0; bits < 1<<length; bits++ {
			var seq []string
			for bit := 0; bit < length; bit++ {
				if bits&(1<<bit) == 0 {
					seq = append(seq, "a\n")
				} else {
					seq = append(seq, "b")
				}
			}
			sequences = append(sequences, seq)
		}
	}
	for _, old := range sequences {
		for _, newTokens := range sequences {
			got, err := Lines(old, newTokens, 10000)
			if err != nil {
				t.Fatal(err)
			}
			x, y, cost := 0, 0, 0
			for _, edit := range got {
				if edit.Kind == Equal || edit.Kind == Delete {
					if x >= len(old) || old[x] != edit.Text {
						t.Fatalf("old script invalid: %q %q %+v", old, newTokens, got)
					}
					x++
				}
				if edit.Kind == Equal || edit.Kind == Insert {
					if y >= len(newTokens) || newTokens[y] != edit.Text {
						t.Fatalf("new script invalid: %q %q %+v", old, newTokens, got)
					}
					y++
				}
				if edit.Kind != Equal {
					cost++
				}
			}
			if x != len(old) || y != len(newTokens) {
				t.Fatal("incomplete script")
			}
			dp := make([][]int, len(old)+1)
			for i := range dp {
				dp[i] = make([]int, len(newTokens)+1)
				dp[i][0] = i
			}
			for j := range dp[0] {
				dp[0][j] = j
			}
			for i := 1; i <= len(old); i++ {
				for j := 1; j <= len(newTokens); j++ {
					if old[i-1] == newTokens[j-1] {
						dp[i][j] = dp[i-1][j-1]
					} else {
						dp[i][j] = min(dp[i-1][j], dp[i][j-1]) + 1
					}
				}
			}
			if cost != dp[len(old)][len(newTokens)] {
				t.Fatalf("nonshortest script for %q -> %q: %+v", old, newTokens, got)
			}
		}
	}
}

func TestResourceLimitsNeverReturnPartialScripts(t *testing.T) {
	old, newTokens := []string{"a", "b", "c"}, []string{"x", "y", "z"}
	want, err := Lines(old, newTokens, 10000)
	if err != nil {
		t.Fatal(err)
	}
	succeeded := false
	for budget := -1; budget < 200; budget++ {
		got, err := Lines(old, newTokens, budget)
		if budget <= 1 && !errors.Is(err, ErrLimit) {
			t.Fatalf("budget %d did not stop comparison", budget)
		}
		if err != nil && (!errors.Is(err, ErrLimit) || got != nil) {
			t.Fatalf("budget %d: partial script %+v, %v", budget, got, err)
		}
		if err == nil {
			succeeded = true
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("budget %d changed script: %+v", budget, got)
			}
		}
	}
	if !succeeded {
		t.Fatal("comparison never completed with sufficient budget")
	}
	if got, err := Lines(make([]string, 1000), make([]string, 1000), 20); !errors.Is(err, ErrLimit) || got != nil {
		t.Fatalf("long equal snake bypassed budget: %+v, %v", got, err)
	}
}
