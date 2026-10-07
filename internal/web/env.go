package web

// Level is a disclosure level of VIEWS.md; the levels nest.
type Level int

// The three levels.
const (
	Glance Level = iota
	Detail
	Provenance
)

// Legend gives every symbol its glyph and its text name. An adapter builds
// one from the gate definition view at the commit in view.
type Legend struct {
	gates, states, reasons, marks map[string]Sym
}

// NewLegend returns a legend from four lists of key and glyph. A gate, a
// state and a reason take their key as text name; a mark takes its meaning.
// Each mark is its key, its glyph and its meaning.
func NewLegend(gates, states, reasons [][2]string, marks [][3]string) *Legend {
	l := &Legend{map[string]Sym{}, map[string]Sym{}, map[string]Sym{}, map[string]Sym{}}
	for _, g := range gates {
		l.gates[g[0]] = Sym{Glyph: g[1], Name: g[0]}
	}
	for _, s := range states {
		l.states[s[0]] = Sym{Glyph: s[1], Name: s[0]}
	}
	for _, r := range reasons {
		l.reasons[r[0]] = Sym{Glyph: r[1], Name: r[0]}
	}
	for _, m := range marks {
		l.marks[m[0]] = Sym{Glyph: m[1], Name: m[2]}
	}
	return l
}

// Gate returns the symbol of a gate key. A key the legend lacks returns a
// Sym with no glyph and the key as name, so a page never loses the word.
func (l *Legend) Gate(key string) Sym { return look(l.gates, key) }

// State returns the symbol of a state key.
func (l *Legend) State(key string) Sym { return look(l.states, key) }

// Reason returns the symbol of a reason key.
func (l *Legend) Reason(key string) Sym { return look(l.reasons, key) }

// Mark returns the symbol of a junction mark: "person", "agent", "reviewer",
// "subproject" or "exempt".
func (l *Legend) Mark(key string) Sym { return look(l.marks, key) }

func look(m map[string]Sym, key string) Sym {
	if s, ok := m[key]; ok {
		return s
	}
	return Sym{Name: key}
}

// Opener decides which folds arrive open. Task db74 supplies the one that
// follows the viewer's role; ByLevel is the one these templates ship with.
type Opener interface {
	Opens(view View, class, id string) bool
}

// ByLevel opens by level alone: at every level the folds of class "kids";
// at detail also "detail" and "day"; at provenance also "prov" and "sub";
// never "fold", "group" or "columns".
type ByLevel Level

// Opens implements Opener.
func (b ByLevel) Opens(_ View, class, _ string) bool {
	switch class {
	case "kids":
		return true
	case "detail", "day":
		return Level(b) >= Detail
	case "prov", "sub":
		return Level(b) >= Provenance
	}
	return false
}

// openAll opens every fold. A part is built with it, so that the body of the
// fold a part serves holds every fold inside it open and none lazy.
type openAll struct{}

// Opens implements Opener.
func (openAll) Opens(View, string, string) bool { return true }

// Env is what an adapter needs beyond the data of its view. Render builds it.
type Env struct {
	Page   *Page // the request; every address comes from its methods
	Legend *Legend
	Person string // the person parameter; "" marks nobody
	Open   Opener
	Inline bool // every fold holds its body: the export, and the document of a part address
}

// Fold returns the fold of one details element. part names the lazy part
// that serves its body, or is "" for a fold whose body always stands in the
// page. The fold the address names arrives open. A fold with a part is lazy
// when it arrives closed and Inline is off.
func (e Env) Fold(class, id, part, key string) Fold {
	f := Fold{ID: id, Class: class, Open: e.Open.Opens(e.Page.View, class, id)}
	if part == "" {
		return f
	}
	if e.Page.Params.Part == part && e.Page.Params.Key == key {
		f.Open = true
	}
	if !f.Open && !e.Inline {
		if key != "" {
			part += ":" + key
		}
		f.Src = e.Page.Self("part", part)
		f.Alt = f.Src + "#" + id
	}
	return f
}

// levelOf returns the level a page opens at: the level the address states,
// else glance for the export and for an observer, provenance for the owner on
// the history and the audit, and detail for every other role (VIEWS.md,
// Levels). The role is the one the address states, else the viewer's first.
func levelOf(p *Page) Level {
	switch p.Params.Level {
	case "glance":
		return Glance
	case "detail":
		return Detail
	case "provenance":
		return Provenance
	}
	role := p.Params.Role
	if role == "" {
		role = p.Viewer.Role()
	}
	switch {
	case !p.Scripts, role == "observer":
		return Glance
	case role == "owner" && (p.View.Name == "history" || p.View.Name == "audit"):
		return Provenance
	}
	return Detail
}

// legendOf builds the Legend from Page.Legend, tablo's gate definition. Until
// the gate adapter lands it knows no type and returns NewLegend(nil, nil, nil, nil).
func legendOf(any) *Legend { return NewLegend(nil, nil, nil, nil) }

// envOf returns the Env of one rendering of p in a shape: Legend from
// legendOf, Person from Params.Person, Open as ByLevel(levelOf(p)), or
// openAll{} for a Part; Inline when p.Scripts is false, or when the shape is
// Document and Params.Part is set.
func envOf(p *Page, shape Shape) Env {
	e := Env{
		Page:   p,
		Legend: legendOf(p.Legend),
		Person: p.Params.Person,
		Open:   ByLevel(levelOf(p)),
		Inline: !p.Scripts || (shape == Document && p.Params.Part != ""),
	}
	if shape == Part {
		e.Open = openAll{}
	}
	return e
}

// FoldRuns applies the fold rule to a list: each stretch of more than eight
// consecutive rows with the same key shows its first three and folds the
// rest; every other row shows. It returns one Run per stretch, in order. The
// caller sets the Fold and the Alike text of each Run that has a Rest.
func FoldRuns[T any](rows []T, key func(T) string) []Run[T] {
	var out []Run[T]
	for i := 0; i < len(rows); {
		j := i + 1
		for j < len(rows) && key(rows[j]) == key(rows[i]) {
			j++
		}
		if j-i > 8 {
			out = append(out, Run[T]{Shown: rows[i : i+3], Rest: rows[i+3 : j]})
		} else if n := len(out); n > 0 && len(out[n-1].Rest) == 0 {
			out[n-1].Shown = append(out[n-1].Shown, rows[i:j]...)
		} else {
			out = append(out, Run[T]{Shown: append([]T(nil), rows[i:j]...)})
		}
		i = j
	}
	return out
}
