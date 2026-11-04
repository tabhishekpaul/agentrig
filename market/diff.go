package market

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DiffTrees returns a unified diff of every text file that differs between
// folders a (installed) and b (new), capped at max bytes.
func DiffTrees(a, b string, max int) (string, error) {
	fa, err := files(a)
	if err != nil {
		return "", err
	}
	fb, err := files(b)
	if err != nil {
		return "", err
	}
	var names []string
	for n := range fa {
		names = append(names, n)
	}
	for n := range fb {
		if _, ok := fa[n]; !ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	var out strings.Builder
	for _, n := range names {
		if out.Len() > max {
			out.WriteString("\n… diff truncated\n")
			break
		}
		x, y := read(fa[n]), read(fb[n])
		if bytes.Equal(x, y) {
			continue
		}
		if bytes.IndexByte(x, 0) >= 0 || bytes.IndexByte(y, 0) >= 0 {
			fmt.Fprintf(&out, "Binary file %s differs\n", n)
			continue
		}
		out.WriteString(Unified(n, lines(x), lines(y)))
	}
	return out.String(), nil
}

// files maps slash paths to absolute paths of the regular files under dir.
func files(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			out[filepath.ToSlash(rel)] = p
		}
		return nil
	})
	return out, err
}

func read(p string) []byte {
	if p == "" {
		return nil
	}
	b, _ := os.ReadFile(p)
	return b
}

func lines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

type op struct {
	k byte // ' ', '-', '+'
	s string
}

// Unified renders a diff of a → b with 3 lines of context.
// ponytail: O(n·m) LCS, refused past 4M cells; swap in Myers if big files matter.
func Unified(name string, a, b []string) string {
	head := fmt.Sprintf("--- a/%s\n+++ b/%s\n", name, name)
	if len(a)*len(b) > 4_000_000 {
		return head + fmt.Sprintf("@@ file too large to diff (%d → %d lines) @@\n", len(a), len(b))
	}
	n, m := len(a), len(b)
	dp := make([]int32, (n+1)*(m+1)) // dp[i][j] = LCS of a[i:], b[j:]
	at := func(i, j int) *int32 { return &dp[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i, j = i+1, j+1
		case *at(i+1, j) >= *at(i, j+1):
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}

	// line numbers before each op
	al, bl := make([]int, len(ops)+1), make([]int, len(ops)+1)
	for k, o := range ops {
		al[k+1], bl[k+1] = al[k], bl[k]
		if o.k != '+' {
			al[k+1]++
		}
		if o.k != '-' {
			bl[k+1]++
		}
	}
	const ctx = 3
	var out strings.Builder
	out.WriteString(head)
	for k := 0; k < len(ops); {
		for k < len(ops) && ops[k].k == ' ' {
			k++
		}
		if k == len(ops) {
			break
		}
		start, end := max(0, k-ctx), k
		for x := k + 1; x < len(ops) && x <= end+2*ctx; x++ {
			if ops[x].k != ' ' {
				end = x
			}
		}
		stop := min(len(ops), end+ctx+1)
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", al[start]+1, al[stop]-al[start], bl[start]+1, bl[stop]-bl[start])
		for _, o := range ops[start:stop] {
			out.WriteByte(o.k)
			out.WriteString(o.s)
			out.WriteByte('\n')
		}
		k = stop
	}
	return out.String()
}
