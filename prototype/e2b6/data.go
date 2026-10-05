package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// The types below read the JSON of tablo prototype 493e (-view authority and
// -view assignment). Only the keys the templates use are declared.

// Commit is a commit that the authority view names as deciding.
type Commit struct {
	Hash      string `json:"hash"`
	Author    string `json:"author"`
	Committer string `json:"committer"`
	Date      string `json:"date"`
	Subject   string `json:"subject"`
}

// AuthorityView is the authority delegation view: three nested levels of
// rows in tree preorder, each row carrying its depth.
type AuthorityView struct {
	View    string `json:"view"`
	Ref     string `json:"ref"`
	OnTrunk bool   `json:"on_trunk"`
	Glance  struct {
		Rows []struct {
			ID               string `json:"id"`
			Title            string `json:"title"`
			Depth            int    `json:"depth"`
			Assignee         string `json:"assignee"`
			Delegated        bool   `json:"delegated"`
			Authorisation    string `json:"authorisation"`
			Children         int    `json:"children"`
			DiffersFromTrunk bool   `json:"differs_from_trunk"`
		} `json:"rows"`
	} `json:"glance"`
	Detail struct {
		Rows []struct {
			ID               string                       `json:"id"`
			Authorities      []string                     `json:"authorities"`
			JunctionDefaults map[string]map[string]string `json:"junction_defaults"`
		} `json:"rows"`
	} `json:"detail"`
	Provenance struct {
		Rows []struct {
			ID     string  `json:"id"`
			By     string  `json:"by"`
			Way    string  `json:"way"`
			Commit *Commit `json:"commit"`
		} `json:"rows"`
	} `json:"provenance"`
}

// Junction is one position of a task at one gate.
type Junction struct {
	Task        string `json:"task"`
	Gate        string `json:"gate"`
	Next        bool   `json:"next"`
	Contributor string `json:"contributor"`
	Reviewer    string `json:"reviewer"`
	Model       string `json:"model"`
}

// JunctionSource says which task states each field of a junction.
type JunctionSource struct {
	Task    string            `json:"task"`
	Gate    string            `json:"gate"`
	Sources map[string]string `json:"sources"`
}

// AssignedTask is a task assigned to a person, with its status.
type AssignedTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status struct {
		Gate   string `json:"gate"`
		Reason string `json:"reason"`
		State  string `json:"state"`
	} `json:"status"`
}

// Subtree is a subtree a person has authority over.
type Subtree struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Descendants int    `json:"descendants"`
}

// Person is one email's data from the three levels of the assignment view.
type Person struct {
	Email          string
	Assigned       int
	ContributesNxt int
	ReviewsNext    int
	Models         []string
	AssignedTasks  []AssignedTask
	ByGate         map[string]int
	AuthorityOver  []Subtree
	Contributes    []Junction
	Reviews        []Junction
	ContribSources []JunctionSource
	ReviewSources  []JunctionSource
}

// AssignmentView is the task assignment view.
type AssignmentView struct {
	Ref     string
	OnTrunk bool
	Columns []string
	People  []*Person
}

type assignmentJSON struct {
	Ref     string `json:"ref"`
	OnTrunk bool   `json:"on_trunk"`
	Glance  struct {
		Columns []string `json:"columns"`
		People  []struct {
			Email           string   `json:"email"`
			Assigned        int      `json:"assigned"`
			ContributesNext int      `json:"contributes_next"`
			ReviewsNext     int      `json:"reviews_next"`
			Models          []string `json:"models"`
		} `json:"people"`
	} `json:"glance"`
	Detail struct {
		People []struct {
			Email         string         `json:"email"`
			Assigned      []AssignedTask `json:"assigned"`
			ByGate        map[string]int `json:"assigned_by_gate"`
			AuthorityOver []Subtree      `json:"authority_over"`
			Contributes   []Junction     `json:"contributes"`
			Reviews       []Junction     `json:"reviews"`
		} `json:"people"`
	} `json:"detail"`
	Provenance struct {
		People []struct {
			Email       string           `json:"email"`
			Contributes []JunctionSource `json:"contributes"`
			Reviews     []JunctionSource `json:"reviews"`
		} `json:"people"`
	} `json:"provenance"`
}

// parseAssignment merges the three levels of the JSON by email.
func parseAssignment(b []byte) (*AssignmentView, error) {
	var j assignmentJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	v := &AssignmentView{Ref: j.Ref, OnTrunk: j.OnTrunk, Columns: j.Glance.Columns}
	by := map[string]*Person{}
	for _, g := range j.Glance.People {
		p := &Person{Email: g.Email, Assigned: g.Assigned, ContributesNxt: g.ContributesNext, ReviewsNext: g.ReviewsNext, Models: g.Models}
		by[g.Email] = p
		v.People = append(v.People, p)
	}
	for _, d := range j.Detail.People {
		if p := by[d.Email]; p != nil {
			p.AssignedTasks, p.ByGate, p.AuthorityOver, p.Contributes, p.Reviews = d.Assigned, d.ByGate, d.AuthorityOver, d.Contributes, d.Reviews
		}
	}
	for _, d := range j.Provenance.People {
		if p := by[d.Email]; p != nil {
			p.ContribSources, p.ReviewSources = d.Contributes, d.Reviews
		}
	}
	return v, nil
}

// Source supplies the view data. The prototype reads copies of tablo's JSON;
// the product calls tablo.
type Source interface {
	// Refs lists the refs the source answers for the authority view.
	Refs() []string
	// Authority returns the authority view at a ref label.
	Authority(ref string) (*AuthorityView, error)
	// Assignment returns the assignment view of one person, or of every
	// person when the argument is empty. A person the project does not
	// know comes back with no people, as tablo answers it.
	Assignment(person string) (*AssignmentView, error)
}

// ErrNoRef reports a ref the source holds no data for.
var ErrNoRef = errors.New("no data for this ref")

//go:embed testdata
var testdata embed.FS

// refFiles maps a ref label to its fixture. W3 is the branch commit of the
// weather-station corpus where every task reads proposed.
var refFiles = map[string]string{
	"main": "testdata/authority-main.json",
	"W3":   "testdata/authority-W3.json",
}

type fixtureSource struct {
	fsys    fs.FS
	persons map[string]string // email to file
}

// newFixtureSource indexes the per-person assignment files by the person they
// name.
func newFixtureSource(fsys fs.FS) (*fixtureSource, error) {
	s := &fixtureSource{fsys: fsys, persons: map[string]string{}}
	files, err := fs.Glob(fsys, "testdata/assignment-*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if strings.HasSuffix(f, "-unknown.json") {
			continue
		}
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var h struct {
			Params struct {
				Person string `json:"person"`
			} `json:"params"`
		}
		if err := json.Unmarshal(b, &h); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		s.persons[h.Params.Person] = f
	}
	return s, nil
}

func (s *fixtureSource) Refs() []string {
	var r []string
	for k := range refFiles {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

func (s *fixtureSource) Authority(ref string) (*AuthorityView, error) {
	f, ok := refFiles[ref]
	if !ok {
		return nil, ErrNoRef
	}
	b, err := fs.ReadFile(s.fsys, f)
	if err != nil {
		return nil, err
	}
	v := new(AuthorityView)
	return v, json.Unmarshal(b, v)
}

func (s *fixtureSource) Assignment(person string) (*AssignmentView, error) {
	f := "testdata/assignment.json"
	if person != "" {
		var ok bool
		if f, ok = s.persons[person]; !ok {
			f = "testdata/assignment-unknown.json"
		}
	}
	b, err := fs.ReadFile(s.fsys, f)
	if err != nil {
		return nil, err
	}
	return parseAssignment(b)
}
