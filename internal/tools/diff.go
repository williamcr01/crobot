package tools

import (
	"fmt"
	"strings"
)

const (
	maxDiffLines = 500
	maxDiffBytes = 20 * 1024
)

type diffResult struct {
	Text      string
	Truncated bool
}

func unifiedDiff(path string, before, after string) diffResult {
	beforeLines := splitDiffLines(before)
	afterLines := splitDiffLines(after)
	ops := diffLineOps(beforeLines, afterLines)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n", path))
	b.WriteString(fmt.Sprintf("+++ %s\n", path))
	b.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines)))

	truncated := false
	writtenLines := 3
	for _, op := range ops {
		if writtenLines >= maxDiffLines || b.Len()+len(op.text)+2 > maxDiffBytes {
			truncated = true
			break
		}
		b.WriteByte(op.prefix)
		b.WriteString(op.text)
		b.WriteByte('\n')
		writtenLines++
	}
	if truncated {
		b.WriteString("... diff truncated\n")
	}

	return diffResult{Text: b.String(), Truncated: truncated}
}

type diffOp struct {
	prefix byte
	text   string
}

func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func diffLineOps(a, b []string) []diffOp {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	ops := make([]diffOp, 0, m+n)
	for i, j := 0, 0; i < m || j < n; {
		switch {
		case i < m && j < n && a[i] == b[j]:
			ops = append(ops, diffOp{prefix: ' ', text: a[i]})
			i++
			j++
		case j < n && (i == m || dp[i][j+1] > dp[i+1][j]):
			ops = append(ops, diffOp{prefix: '+', text: b[j]})
			j++
		case i < m:
			ops = append(ops, diffOp{prefix: '-', text: a[i]})
			i++
		}
	}
	return ops
}
