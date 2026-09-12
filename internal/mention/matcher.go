package mention

import (
	"context"
	"unicode"
	"unicode/utf8"
)

type node struct {
	next    map[rune]*node
	aliases []int
}

type Matcher struct {
	root  node
	count int
}

type Match struct {
	Alias int
	Start int
	End   int
}

func New(aliases []string) *Matcher {
	m := &Matcher{count: len(aliases)}
	for index, alias := range aliases {
		if alias == "" {
			continue
		}
		n := &m.root
		for _, r := range alias {
			r = fold(r)
			if n.next == nil {
				n.next = make(map[rune]*node)
			}
			if n.next[r] == nil {
				n.next[r] = &node{}
			}
			n = n.next[r]
		}
		n.aliases = append(n.aliases, index)
	}
	return m
}

func (m *Matcher) Find(ctx context.Context, body string) ([]Match, error) {
	if err := ctx.Err(); err != nil || len(m.root.next) == 0 {
		return nil, err
	}
	matches := make([]Match, m.count)
	previous := rune(0)
	steps := 0
	for start, current := range body {
		steps++
		if steps%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		allowed := start == 0 || !word(previous)
		previous = current
		if !allowed {
			continue
		}
		n := &m.root
		for offset, r := range body[start:] {
			n = n.next[fold(r)]
			if n == nil {
				break
			}
			if len(n.aliases) == 0 {
				continue
			}
			_, size := utf8.DecodeRuneInString(body[start+offset:])
			end := start + offset + size
			next, nextSize := utf8.DecodeRuneInString(body[end:])
			if end < len(body) && word(next) {
				continue
			}
			left := start
			if left > 0 {
				_, size := utf8.DecodeLastRuneInString(body[:left])
				left -= size
			}
			for _, alias := range n.aliases {
				if matches[alias].End == 0 {
					matches[alias] = Match{Alias: alias, Start: left, End: end + nextSize}
				}
			}
		}
	}
	out := matches[:0]
	for _, match := range matches {
		if match.End != 0 {
			out = append(out, match)
		}
	}
	return out, ctx.Err()
}

func word(r rune) bool {
	r = fold(r)
	return r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'
}

func fold(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	if r < utf8.RuneSelf {
		return r
	}
	result := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		if next < result {
			result = next
		}
	}
	return result
}
