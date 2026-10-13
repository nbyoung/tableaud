# What the unit gate asks for

This file is the standing text of the unit gate (📏) in `tableaud`, the local daemon and HTML. The root task's junction entry for `unit` names it, so every brief for a unit gate in this project points here. The brief states the task, the requirements, the identity, the trailers and the status file; this file states what the unit gate proves in this project. Whoever dispatches you adds what belongs to one run.

The gate's criterion is "All prescribed tests pass". The prescribed tests of a task are the numbered tests `T1` … of its accepted design, `design/<id>.md`, as its "Decisions at review" amend them. The last entry there, "The implementation review", records where the code departs from the design's body. The implementation gate already wrote the tests. You prove, test by test, that each one exists, runs and passes, and you record the gate only where that holds.

## The work

1. **Read** the task file, the status file and its note, the design's "Tests" section and its "Decisions at review".
2. **Map every test.** For each `T<n>`, find the Go test function or subtest that proves it, by file and name. Run it by name with `go test -run … -v` and read the output: PASS, FAIL or SKIP. A test that skips has not passed. A test that exists and asserts less than its row states has not passed either: read the assertion, not only the verdict.
3. **Sort what does not pass.**
   - A test the design or a decision assigns to a later gate, or to a later run that waits for a named task, is not prescribed for unit. A status note alone moves no test to a later gate.
   - A test that is prescribed and is missing, weak, skipping or failing for a small reason inside the task's own packages: fix the test or the defect in a commit of its own (`Fix …`, `Add the … test`), with the checks passing first. Change no exported name, no design and no other task's code.
   - Anything larger, or anything that waits on work that does not exist: do not fix it, and do not record the gate.
4. **Run the checks** on the whole tree.

## In this project

- **`node`.** The tests that run `tableaud.js` skip where the host lacks `node`, and a skip is no pass: run where `node` is on the path.
- **Tests that wait for `tablo`.** A test that skips because `tablo`'s views are not released is not prescribed for unit only where the design or a decision says so. Name each one in the status note with the task it waits for.
- **`V1` …** are a person's checks at the validate gate. They are not prescribed for unit.

## Checks

All five pass, and one more run makes sure that no cached result stands in for a run:

```
test -z "$(gofmt -l .)"
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```
```
go test -race -count=1 ./...
```

## Recording the gate

Record `unit` only when every test prescribed for unit passes on the host you run on and the checks pass. The commit changes the status file alone. Its subject reads `Record the <task title, lower case> at unit`, and its body lists the tests by number with their verdicts in a few lines. The note says which tests pass and how they run, which tests wait for which later gate or task, and what the integrate gate needs.

## Declining

Where a prescribed test does not pass and you cannot fix it under step 3, leave the `gate:` line as it stands and carry no `Reviewed:` trailer. Change the note alone, in a commit `Record what the <task title, lower case> unit gate waits for`, so that it says which tests pass, which do not, and what each waits for. Where the note already says exactly that, make no commit.

A declined gate with a precise reason is a good result. A recorded gate over a skipped test is a bad one.
