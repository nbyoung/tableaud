package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed testdata
var testdata embed.FS

// errNoData reports a request the fixtures cannot answer. The real server
// calls tablo for any range, so the error stands in for no tablo call.
var errNoData = errors.New("the prototype holds no fixture for this request")

// Event is one object of 8ed1's history output, field for field.
type Event struct {
	Date   string `json:"date"`
	Commit string `json:"commit"`
	By     string `json:"by"`
	Task   string `json:"task"`
	Event  string `json:"event"`
	Gate   string `json:"gate"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// RangeFinding is one object of 8ed1's audit output, field for field.
type RangeFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Task     string `json:"task"`
	Message  string `json:"message"`
	Since    string `json:"since"`
	Range    string `json:"range"`
}

// Symbols maps gate, state and reason keys to the symbols of gates.yaml.
type Symbols struct {
	Gate, State, Reason map[string]string
}

// Source supplies the view data. The fixture source reads testdata; the real
// server runs tablo.
type Source interface {
	// History returns the events of a range and the events before it, which a
	// replay of the status needs (see the README).
	History(rng string) (events, prefix []Event, err error)
	AuditRange(rng string, stale int) ([]RangeFinding, error)
	AuditDada(entry string) ([]DadaFinding, error)
	Titles() map[string]string
	Symbols() Symbols
}

type fixtureSource struct {
	fsys fs.FS
}

func newFixtureSource() Source {
	sub, err := fs.Sub(testdata, "testdata")
	if err != nil {
		panic(err)
	}
	return fixtureSource{fsys: sub}
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9._-]*$`)

func (s fixtureSource) readJSON(name string, v any) error {
	if !safeName.MatchString(name) {
		return errNoData
	}
	b, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return fmt.Errorf("%w: %s", errNoData, name)
	}
	return json.Unmarshal(b, v)
}

func (s fixtureSource) History(rng string) ([]Event, []Event, error) {
	var all []Event
	if err := s.readJSON("history-all.json", &all); err != nil {
		return nil, nil, err
	}
	if rng == "" || rng == "..W13" {
		return all, nil, nil
	}
	var ev []Event
	if err := s.readJSON("history-"+rng+".json", &ev); err != nil {
		return nil, nil, err
	}
	// The range is a contiguous run of the whole history: find where it starts.
	for i := range all {
		if len(ev) == 0 || i+len(ev) > len(all) {
			break
		}
		if all[i] == ev[0] {
			ok := true
			for j := range ev {
				ok = ok && all[i+j] == ev[j]
			}
			if ok {
				return ev, all[:i], nil
			}
		}
	}
	return ev, nil, nil
}

func (s fixtureSource) AuditRange(rng string, stale int) ([]RangeFinding, error) {
	name := "audit-" + rng
	if stale > 0 {
		name += fmt.Sprintf("-stale%d", stale)
	}
	var f []RangeFinding
	err := s.readJSON(name+".json", &f)
	return f, err
}

func (s fixtureSource) AuditDada(entry string) ([]DadaFinding, error) {
	if !safeName.MatchString(entry) {
		return nil, errNoData
	}
	b, err := fs.ReadFile(s.fsys, "dada-"+entry+".md")
	if err != nil {
		return nil, fmt.Errorf("%w: dada-%s.md", errNoData, entry)
	}
	return parseDada(string(b)), nil
}

func (s fixtureSource) Titles() map[string]string {
	var g struct {
		Rows []struct{ ID, Title string } `json:"rows"`
	}
	m := map[string]string{}
	if err := s.readJSON("global-tableau.json", &g); err == nil {
		for _, r := range g.Rows {
			m[r.ID] = r.Title
		}
	}
	return m
}

var symbolLine = regexp.MustCompile(`^\s*- \{ key: (\w+),\s*symbol: ([^,]+),`)

// Symbols reads the one-line flow mappings of gates.yaml. A YAML parser
// replaces this in the product.
func (s fixtureSource) Symbols() Symbols {
	sy := Symbols{Gate: map[string]string{}, State: map[string]string{}, Reason: map[string]string{}}
	b, err := fs.ReadFile(s.fsys, "gates.yaml")
	if err != nil {
		return sy
	}
	var cur map[string]string
	for _, line := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(line) {
		case "gates:":
			cur = sy.Gate
		case "states:":
			cur = sy.State
		case "reasons:":
			cur = sy.Reason
		}
		if m := symbolLine.FindStringSubmatch(line); m != nil && cur != nil {
			cur[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return sy
}
