# What the design gate asks for

This file is the standing text of the design gate (📐) in `tableaud`, the local daemon and HTML. The root task's junction entry for `design` names it, so every brief for a design in this project points here. The brief states the task, the requirements, the identity, the trailers and the status file; this file states what a design is in this project. Whoever dispatches you adds what belongs to one run.

The gate's criterion is "A model and sufficient tests exist". A design fixes what the implementation builds: after the owner accepts it, another model implements it against the tests with no design decision left to take. The owner reviews it by eye, so it is a document a person reads in one sitting, not a code dump.

## What binds you

Three kinds of record bind a design, in this order where they disagree:

1. **The owner's rulings.** The method's texts as they now read ([`README.md`](https://github.com/nbyoung/tableaux/blob/main/README.md), [`SYNTAX.md`](https://github.com/nbyoung/tableaux/blob/main/SYNTAX.md), [`VIEWS.md`](https://github.com/nbyoung/tableaux/blob/main/VIEWS.md) with the log of rulings at its end, [`corpus/RULES.md`](https://github.com/nbyoung/tableaux/blob/main/corpus/RULES.md)), and the "Decisions at review" section of every accepted design. Where a task is implemented, the last entry of that section, "The implementation review", records where the code departs from the design's body.
2. **The code on `main`**, for every task already implemented. Design onto what stands, by its real names and signatures; read the code, not only its design.
3. **The accepted designs** of tasks not yet implemented, in this repository and its siblings. Each has an "Assumed interfaces" table; where a row names your task, your design confirms it or amends it, row by row.

Do not reopen a settled point. Mockups and task descriptions are scaffolding, not records: where one is stale against a ruling, the ruling stands, the mockup stays as drawn, and your document says so in a line.

## Read first

The root [`README.md`](../../README.md); your task file, its parents up to the root, and every task that requires yours or that yours requires; `prototype/<id>/README.md`, which ends in findings for this gate; `.tableaux/gates.yaml`; the accepted designs your task meets, and the code they produced. For a view: its section of `VIEWS.md`, all of "What every view shares", the rulings at the end, and the approved mockups under [`docs/mockups/`](https://github.com/nbyoung/tableaux/blob/main/docs/mockups) of the `tableaux` repository.

## What you write

- `design/<id>.md`: the design.
- `design/<id>/`, optional: drafts that fix a shape better than prose does, such as a golden output, a fixture, a schema, a table of cases or a script.
- `.tableaux/status/<id>.yaml`: the hand-off, as your brief states it.

No `.go` file lands with a design. Go declarations live in fenced code blocks inside the document, or in a `model.md` draft where the set is long, so that the build, the vet and the lint see nothing new. Change nothing else: no task file, no other status, no `README.md`, no `go.mod`, no code, no other design. A flaw you find there is a finding for the document.

## The document

Follow the form of the accepted designs in [`design/`](../../design/). The title reads `# Design <id>: <task title>`. Then, in an order that serves the reader:

1. **A lead paragraph**: what the task delivers and the few choices that shape it, each with the number of its decision. A table of the drafts where there are any.
2. **The model**: the packages and their paths, the exported and the internal surface as Go declarations with a doc comment each, the data that flows through, the errors, and what is deterministic. Decide; do not list options. Say what the design keeps from the prototype and what it drops, and settle each finding the prototype left for this gate.
3. **Conformance to the texts**: the sections of the method's texts the design implements and how. Name each place where a text is silent or two texts disagree; do not restate the texts.
4. **Assumed interfaces**, in two numbered tables. First, what you confirm or amend of the rows other designs state about your task. Second, `A1` …: what you assume of a task designed later or of another project, each with the task that confirms it. Keep each assumption checkable.
5. **Tests**, a numbered table `T1` …: what each test proves and how. The set covers every decision of the model, names its fixtures, and runs with no network and no wall clock. Say which claims fall to a later gate and why, what you tried already, and what you could not check.
6. **Implementation notes**: the order of work in steps that each build alone, the files that land and the files that change, what happens to the prototype, and any dependency with its licence. Where the design shows that a `requires` entry or a junction of the plan is wrong or missing, say so here; do not edit the plan.
7. **Decisions at review**: a numbered list of the questions that are the owner's to decide, each with the choice the text already makes and one sentence on the alternative, so that the owner answers yes or names the alternative. Name every other design a decision binds. Ask only real decisions: a public vocabulary, a dependency, a policy, or a divergence from a text, a mockup or an accepted design.

Aim for 200 to 400 lines. Depth goes where the implementer would otherwise decide.

## In this project

- **tableaud reads no task file.** `internal/source.Source` is the one seam to `tablo`. Where a view lacks a fact, the design asks `tablo` for it in "Assumed interfaces".
- **The standard library alone.** The server, the templates and the embedding add no module. A dependency or a vendored asset is a decision at review, with its licence.
- **The server holds no presentation policy.** What a viewer chooses lives in the address or in the browser. Every state a page can show has a link.
- **No claim about a browser.** A design states the bytes of a page, which a test checks. What a browser, a screen reader or a phone shows falls to the validate gate, as a numbered list (`V1` …) for a person.
- **A cluster that shares the frame.** Check the shared model of `internal/web` against the code on `main` before you declare a type there.

## Style

All prose, and every commit message, takes the simple present tense and the active voice. Long command-line options take two hyphens. Take every symbol from `gates.yaml` and the method's texts; invent none. Never state that something works on a host, a terminal, a browser or an operating system unless you ran it there.

## Checks

A trial that compiles your declarations against the code on `main`, outside the repository, is worth its minutes: say in "Tests" that you ran it. Before each commit, these pass as they do on `main`:

```
test -z "$(gofmt -l .)"
go vet ./...
golangci-lint run
go test ./...
```

Check also that every relative link of the document resolves and that every assumption, test and decision has a number.

## Commits

Two commits, in this order:

1. The design: `design/<id>.md` and `design/<id>/`, with the subject `Design the <task title, lower case>`.
2. The hand-off: the status file alone, with the subject `Record the <task title, lower case> design awaiting review`.

The owner reviews this gate, so the branch stays unmerged and neither commit carries a `Reviewed:` trailer.
