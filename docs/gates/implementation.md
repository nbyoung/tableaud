# What the implementation gate asks for

This file is the standing text of the implementation gate (🧱) in `tableaud`, the local daemon and HTML. The root task's junction entry for `implementation` names it, so every brief for an implementation in this project points here. The brief states the task, the requirements, the identity, the trailers and the status file; this file states what an implementation is in this project. Whoever dispatches you adds what belongs to one run.

The gate's criterion is "Artifacts suffice for unit, integration and validation tests". Here that means: the code the design declares and the tests the design prescribes, its `T1` … table, exist in the repository, build, and are complete enough that the unit gate needs no new code of substance. Run the tests as you write them and fix what fails. You record this gate only; the unit gate is a later junction with its own run.

## What binds you

1. **The design**, `design/<id>.md`, with its drafts under `design/<id>/`. Read it in full. It fixes the packages, the declarations, the file names, the tests and the order of work.
2. **Its "Decisions at review".** Where a decision amends the text above it, the decision wins. Where the body and a decision disagree and the decision does not settle the point, take the reading the decision implies and name the place.
3. **What stands on `main`.** A task your design builds on may be implemented, and its implementation review may change a name or a seam your design assumes. The code on `main` and the last entry of that task's "Decisions at review" win over your design's body.
4. **The method's texts**, where the design cites them: [`README.md`](https://github.com/nbyoung/tableaux/blob/main/README.md), [`SYNTAX.md`](https://github.com/nbyoung/tableaux/blob/main/SYNTAX.md), [`VIEWS.md`](https://github.com/nbyoung/tableaux/blob/main/VIEWS.md) and [`corpus/RULES.md`](https://github.com/nbyoung/tableaux/blob/main/corpus/RULES.md) of the `tableaux` repository.

You implement the design; you do not redesign. Where the design proves wrong or silent on a point you cannot build around, choose the smallest reading that keeps its stated interfaces, record the choice, and go on.

## What you leave alone

- `design/<id>.md` and every other design. The owner records departures in "Decisions at review" after the review.
- Under `.tableaux`, everything but your task's status file.
- Mockups and task descriptions. They are scaffolding, not records: where one is stale, leave it and say so.
- `README.md`, except where the design's implementation notes say it changes.
- Every other task's prototype, except to copy from it.

## The prototype

A prototype goes when its task records `implementation`. Remove `prototype/<id>/` of your own task in a commit of its own, `Remove the <task title, lower case> prototype`, just before the status commit. Where something outside the prototype still imports or executes it, leave it and say so in the status note.

## Code

- Match the surrounding code: its comment density, its naming and its idiom. Every package has a package comment, and every exported name has a doc comment that starts with the name.
- Prose, in comments, the README, commit messages and status notes, takes the simple present tense and the active voice.
- No cgo, and no dependency beyond what the design lists.
- Copy the drafts the design names into `testdata/` where its tests read them; the originals stay under `design/<id>/`.
- No test uses the network, sleeps on the wall clock, or writes outside `t.TempDir()`. A test that needs a tool the host may lack skips with a reason.

## In this project

- **Golden pages.** `go test -update ./internal/web` rewrites the pages under `internal/web/testdata/`, and `go test -update ./internal/export` the files of the export. After an update, read `git diff`: only what your design changes may differ.
- **Fixtures.** Every test answers from the JSON fixtures of `internal/source/sourcetest`. No test reads a repository of the host, and none starts a browser.
- **Tests that wait for `tablo`.** Where the design or a decision says a test waits for `tablo`'s views, write the test so that it exists and skips with that reason, and name it in the status note.
- **The script.** `static/tableaud.js` stays under the limit its test holds. The tests that run it need `node` and skip without it.
- **The style sheet.** Where a design says a block stays byte for byte, it does.

## Checks

Before every commit, all five pass, as CI runs the first four:

```
test -z "$(gofmt -l .)"
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```

Do not pipe a check through a filter that hides its exit status.

## Commits

Commit in the steps of the design's order of work, where each step builds and passes the checks alone. Where steps interlock so that no one of them builds alone, land the smallest set that does in one commit. Write no stub whose only purpose is to make a step build apart from the next. A subject line is imperative and capitalised, with no full stop, in the style of `git log` on `main`: "Add the watcher", "Implement the Markdown writer".

The last commit records the gate. It changes the status file alone, and its subject reads `Record the <task title, lower case> implementation`. The note says, in one line, what stands, what the unit gate runs, and what waits and on which task.

Record the gate only when the criterion holds. Where part of the design cannot land, leave the `gate:` line as it stands, write a note that says what stands and what lacks, and carry no `Reviewed:` trailer.
