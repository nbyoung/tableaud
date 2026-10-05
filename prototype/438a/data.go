package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
)

// The types below decode the JSON of tablo prototype 493e. Only the keys the
// two views read appear here.

// Commit names one Git commit with its actors.
type Commit struct {
	Hash      string `json:"hash"`
	Author    string `json:"author"`
	Committer string `json:"committer"`
	Date      string `json:"date"`
	Subject   string `json:"subject"`
}

// GateView is the output of `-view gate`.
type GateView struct {
	Ref    string `json:"ref"`
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
			Severity int    `json:"severity"`
			Synopsis string `json:"synopsis"`
		} `json:"states"`
	} `json:"detail"`
	Glance struct {
		Folded int `json:"folded"`
		Gates  []struct {
			Key    string `json:"key"`
			Name   string `json:"name"`
			Symbol string `json:"symbol"`
		} `json:"gates"`
		Marks []struct {
			Symbol  string `json:"symbol"`
			Meaning string `json:"meaning"`
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
	Provenance struct {
		LastChanged map[string]Commit `json:"last_changed"`
		Tableaux    string            `json:"tableaux"`
		Trunk       string            `json:"trunk"`
	} `json:"provenance"`
}

// Reference is a text and a URL; the text may be absent.
type Reference struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Requirement is one `requires` entry, or one dependent.
type Requirement struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	From      string `json:"from"`
	To        string `json:"to"`
	Text      string `json:"text"`
	Met       bool   `json:"met"`
	Due       bool   `json:"due"`
	Condition string `json:"condition"`
}

// Junction is one resolved junction.
type Junction struct {
	Gate        string      `json:"gate"`
	Symbol      string      `json:"symbol"`
	Kind        string      `json:"kind"`
	Marks       []string    `json:"marks"`
	Contributor string      `json:"contributor"`
	Reviewer    string      `json:"reviewer"`
	Model       string      `json:"model"`
	References  []Reference `json:"references"`
	Subproject  *struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"subproject"`
}

// Snapshot is the subproject task read at the pinned commit.
type Snapshot struct {
	Gate string `json:"gate"`
	Pin  string `json:"pin"`
	Task string `json:"task"`
	URL  string `json:"url"`
}

// Status is the status line: a leaf's own, a parent's roll-up or a snapshot's.
type Status struct {
	Gate     string    `json:"gate"`
	State    string    `json:"state"`
	Reason   string    `json:"reason"`
	Note     string    `json:"note"`
	Date     string    `json:"date"`
	Recorder string    `json:"recorder"`
	Commit   string    `json:"commit"`
	Derived  bool      `json:"derived"`
	From     string    `json:"from"`
	Snapshot *Snapshot `json:"snapshot"`
}

// Event is one history entry.
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

// TaskView is the output of `-view task -task <id>`.
type TaskView struct {
	Ref    string `json:"ref"`
	Glance struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
		NextGate string `json:"next_gate"`
		Parent   *struct {
			ID    string `json:"id"`
			Order int    `json:"order"`
			Title string `json:"title"`
		} `json:"parent"`
		Status Status `json:"status"`
	} `json:"glance"`
	Detail struct {
		Authorisation struct {
			State string `json:"state"`
			Way   string `json:"way"`
		} `json:"authorisation"`
		Authorities []string      `json:"authorities"`
		Children    []string      `json:"children"`
		Dependents  []Requirement `json:"dependents"`
		Description string        `json:"description"`
		Junctions   []Junction    `json:"junctions"`
		References  []Reference   `json:"references"`
		Requires    []Requirement `json:"requires"`
	} `json:"detail"`
	Provenance struct {
		Authorisation struct {
			State  string  `json:"state"`
			Commit *Commit `json:"commit"`
			By     string  `json:"by"`
			Way    string  `json:"way"`
		} `json:"authorisation"`
		Command         string  `json:"command"`
		Events          []Event `json:"events"`
		JunctionSources []struct {
			Gate    string            `json:"gate"`
			Sources map[string]string `json:"sources"`
		} `json:"junction_sources"`
		Reviews []struct {
			Gate     string `json:"gate"`
			Reviewer string `json:"reviewer"`
			Commit   string `json:"commit"`
			Date     string `json:"date"`
		} `json:"reviews"`
		StatusCommit *Commit `json:"status_commit"`
	} `json:"provenance"`
}

// Store holds the decoded view data: the stand-in for tablo.
type Store struct {
	Gate  *GateView
	Tasks map[string]*TaskView
}

// Load reads gate.json and every task-<id>.json from fsys.
func Load(fsys fs.FS) (*Store, error) {
	s := &Store{Gate: new(GateView), Tasks: map[string]*TaskView{}}
	if err := readJSON(fsys, "gate.json", s.Gate); err != nil {
		return nil, err
	}
	files, err := fs.Glob(fsys, "task-*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		t := new(TaskView)
		if err := readJSON(fsys, f, t); err != nil {
			return nil, err
		}
		s.Tasks[t.Glance.ID] = t
	}
	return s, nil
}

func readJSON(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path.Base(name), err)
	}
	return nil
}
