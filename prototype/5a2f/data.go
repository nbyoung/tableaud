package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed testdata
var testdata embed.FS

// Tableau is the global tableau of tablo prototype 886d.
type Tableau struct {
	Ref     string `json:"ref"`
	Columns []struct {
		Gate   string `json:"gate"`
		Symbol string `json:"symbol"`
	} `json:"columns"`
	Rows []Row `json:"rows"`
}

// Row is one task of the tableau.
type Row struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Depth  int    `json:"depth"`
	Parent bool   `json:"parent"`
	Status struct {
		Gate   string `json:"gate"`
		State  string `json:"state"`
		Reason string `json:"reason"`
	} `json:"status"`
	Cells []struct {
		Gate    string `json:"gate"`
		Kind    string `json:"kind"`
		Symbols string `json:"symbols"`
	} `json:"cells"`
}

// Queue is the work queue of one person.
type Queue struct {
	Person string `json:"person"`
	Items  []struct {
		Kind  string `json:"kind"`
		Task  string `json:"task"`
		Title string `json:"title"`
		Gate  string `json:"gate"`
		Cause string `json:"cause"`
	} `json:"items"`
}

// Legend is the gate definition of tablo prototype 493e at the glance and
// detail levels: every gate, state, reason and mark with its symbol, name and
// meaning.
type Legend struct {
	Detail struct {
		Gates []struct {
			Key      string `json:"key"`
			Criteria string `json:"criteria"`
		} `json:"gates"`
		Reasons []struct {
			Key      string `json:"key"`
			Synopsis string `json:"synopsis"`
		} `json:"reasons"`
		States []struct {
			Key      string `json:"key"`
			Synopsis string `json:"synopsis"`
		} `json:"states"`
	} `json:"detail"`
	Glance struct {
		Gates []struct {
			Key    string `json:"key"`
			Name   string `json:"name"`
			Symbol string `json:"symbol"`
		} `json:"gates"`
		Marks []struct {
			Meaning string `json:"meaning"`
			Symbol  string `json:"symbol"`
		} `json:"marks"`
		Reasons []struct {
			Key    string `json:"key"`
			Symbol string `json:"symbol"`
		} `json:"reasons"`
		States []struct {
			Key    string `json:"key"`
			Symbol string `json:"symbol"`
		} `json:"states"`
	} `json:"glance"`

	syms []Sym // every cell symbol, longest first
}

// Sym is one symbol with the text that replaces it for people who do not see it.
type Sym struct {
	Glyph string
	Name  string
	Kind  string // state, reason, mark or unknown
	Key   string
}

// Data is everything the prototype renders.
type Data struct {
	Tableau Tableau
	Queue   Queue
	Legend  *Legend
}

func loadJSON(name string, v any) error {
	b, err := testdata.ReadFile("testdata/" + name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// LoadData reads the copies in testdata/.
func LoadData() (*Data, error) {
	d := &Data{Legend: &Legend{}}
	for name, v := range map[string]any{
		"global-tableau.json": &d.Tableau,
		"queue-ada.json":      &d.Queue,
		"gate.json":           d.Legend,
	} {
		if err := loadJSON(name, v); err != nil {
			return nil, err
		}
	}
	d.Legend.index()
	return d, nil
}

// humanise turns a data key such as at_risk into words.
func humanise(key string) string { return strings.ReplaceAll(key, "_", " ") }

func (l *Legend) index() {
	for _, s := range l.Glance.States {
		l.syms = append(l.syms, Sym{s.Symbol, humanise(s.Key), "state", s.Key})
	}
	for _, r := range l.Glance.Reasons {
		l.syms = append(l.syms, Sym{r.Symbol, humanise(r.Key), "reason", r.Key})
	}
	for _, m := range l.Glance.Marks {
		l.syms = append(l.syms, Sym{m.Symbol, m.Meaning, "mark", m.Meaning})
	}
	sort.SliceStable(l.syms, func(i, j int) bool {
		return len(plain(l.syms[i].Glyph)) > len(plain(l.syms[j].Glyph))
	})
}

// plain drops the emoji variation selector so that a symbol with and without
// it compares equal.
func plain(s string) string { return strings.ReplaceAll(s, "️", "") }

// GateName gives the name of a gate from the definition.
func (l *Legend) GateName(key string) string {
	for _, g := range l.Glance.Gates {
		if g.Key == key {
			return g.Name
		}
	}
	return humanise(key)
}

// Tokenize splits a cell's symbol string into known symbols. Text that no
// definition covers comes back as an unknown symbol, which the tests reject.
func (l *Legend) Tokenize(cell string) []Sym {
	rest := plain(cell)
	var out []Sym
	for rest != "" {
		matched := false
		for _, s := range l.syms {
			if g := plain(s.Glyph); strings.HasPrefix(rest, g) {
				out = append(out, s)
				rest = rest[len(g):]
				matched = true
				break
			}
		}
		if !matched {
			r := []rune(rest)
			out = append(out, Sym{string(r[0]), "unrecognised symbol", "unknown", ""})
			rest = string(r[1:])
		}
	}
	return out
}

// Criteria gives the criteria of a gate from the detail level.
func (l *Legend) Criteria(key string) string {
	for _, g := range l.Detail.Gates {
		if g.Key == key {
			return g.Criteria
		}
	}
	return ""
}

// Synopsis gives the synopsis of a state or a reason from the detail level.
func (l *Legend) Synopsis(kind, key string) string {
	if kind == "state" {
		for _, s := range l.Detail.States {
			if s.Key == key {
				return s.Synopsis
			}
		}
	}
	for _, r := range l.Detail.Reasons {
		if kind == "reason" && r.Key == key {
			return r.Synopsis
		}
	}
	return ""
}
