package web

import (
	"strconv"
	"strings"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["audit"] = func(e Env) any {
		var d *view.Audit
		switch v := e.Page.Data.(type) {
		case *view.Audit:
			d = v
		case view.Audit:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return AuditFrom(d, e.withLegend(d.Legend))
	}
}

// acts words each of tablo's ten acts as the action of a group.
var acts = map[string]string{
	"authorise":      "Authorise",
	"review":         "Review",
	"reaffirm":       "Reaffirm",
	"record_handoff": "Record the hand-off",
	"clear_handoff":  "Clear the hand-off",
	"revise":         "Revise the file",
	"move_pin":       "Move the pin",
	"checkout":       "Check out the submodule",
	"await":          "Await the requirement",
	"none":           "None: the plan allows it",
}

// AuditFrom builds the model of the audit from tablo's view. The adapter
// groups the findings by rule and resolver, errors first, then warnings,
// then information, each severity in tablo's order; anchors each group by
// its rule, with "-2", "-3" for the rule's further resolvers; words the act
// of each finding; and names the focus of the groups the person the page is
// seen by resolves. The three counts and the person with the most to resolve
// are tablo's.
func AuditFrom(d *view.Audit, e Env) *AuditView {
	v := &AuditView{Errors: d.Errors, Warnings: d.Warnings, Information: d.Information, Stale: e.Page.Params.Stale}
	if v.Stale <= 0 {
		v.Stale = 7
	}
	if d.Most != nil {
		v.Most, v.MostCount = d.Most.Email, d.Most.Count
	}
	if d.AuditDetail != nil {
		for _, s := range d.Silent {
			kind := lowerFirst(s.Kind)
			if s.Rule != "" {
				kind = s.Rule + ", " + kind
			}
			v.Silent = append(v.Silent, "No finding of "+kind+".")
		}
	}

	type group struct {
		anchor, rule string
		findings     []view.Finding
	}
	var groups []*group
	byKey := map[string]*group{}
	perRule := map[string]int{}
	for _, sev := range []string{"error", "warning", "information"} {
		for _, f := range d.Findings {
			if f.Severity != sev {
				continue
			}
			rule := f.Rule
			if rule == "" {
				rule = f.Key
			}
			k := sev + "\x00" + rule + "\x00" + f.Resolver
			g, ok := byKey[k]
			if !ok {
				perRule[rule]++
				anchor := strings.ToLower(rule)
				if n := perRule[rule]; n > 1 {
					anchor += "-" + strconv.Itoa(n)
				}
				g = &group{anchor: anchor, rule: rule}
				byKey[k] = g
				groups = append(groups, g)
			}
			g.findings = append(g.findings, f)
		}
	}
	var resolved []Resolved
	for _, g := range groups {
		resolved = append(resolved, Resolved{Anchor: g.anchor, Resolver: g.findings[0].Resolver})
	}
	e = AuditFocus(e, resolved)

	for _, g := range groups {
		first := g.findings[0]
		fg := FindingGroup{
			Anchor: g.anchor, Severity: first.Severity, Action: actOf(first.Act), Rule: g.rule,
			Title: first.Kind, Resolver: e.person(first.Resolver),
			Fold: e.Fold("detail", g.anchor, "", ""),
			Prov: e.Fold("prov", "prov-"+g.anchor, "rule", g.anchor),
		}
		kind := FindingKind{
			Severity: fg.Severity, Rule: g.rule, Title: fg.Title, Href: "#" + g.anchor,
			Count: len(g.findings), Resolver: fg.Resolver,
		}
		seenTask, seenCommit, seenCommand := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, f := range g.findings {
			row := Finding{Message: f.Kind, Action: actOf(f.Act), Resolver: e.person(f.Resolver)}
			for _, t := range f.Tasks {
				ref := e.taskRef(t)
				row.Tasks = append(row.Tasks, ref)
				if !seenTask[t.ID] {
					seenTask[t.ID] = true
					kind.Tasks = append(kind.Tasks, ref)
				}
			}
			if f.Gate != "" {
				s := e.Legend.Gate(f.Gate)
				row.Gate = &s
			}
			if f.FindingDetail != nil {
				row.Files = append([]string(nil), f.Files...)
				row.Message = f.Message
			}
			if p := f.FindingProvenance; p != nil {
				if fg.Sentence == "" {
					fg.Sentence = p.Sentence
					fg.Source = p.Source.Text
					if fg.Source == "" {
						fg.Source = p.Source.URL
					}
				}
				for i := range p.Commits {
					if c := p.Commits[i]; !seenCommit[c.Hash] {
						seenCommit[c.Hash] = true
						fg.Commits = append(fg.Commits, commitValue(&c))
					}
				}
				for _, c := range commands(p.Commands) {
					if !seenCommand[c] {
						seenCommand[c] = true
						fg.Commands = append(fg.Commands, c)
					}
				}
			}
			fg.Rows = append(fg.Rows, row)
		}
		v.Kinds = append(v.Kinds, kind)
		v.Groups = append(v.Groups, fg)
	}
	return v
}

// actOf words an act as an action.
func actOf(act string) string {
	if w, ok := acts[act]; ok {
		return w
	}
	return act
}
