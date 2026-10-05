# Prototype 438a: the legend and the task template

Task `438a` Legend and task templates, gate `function`.

The gate definition view (the legend) and the task definition view (a page per task) as `html/template` pages rendered on the server from tablo's view data, with HTMX fetching fragments for the folds. The layout is provisional: no HTML mockup exists yet, so the prototype settles function, not look.

## Questions and answers

1. **Does the 493e data render as the two views with `html/template` alone, with every symbol, name, criterion and date from the data?** Yes. The templates hold no symbol, gate name, state, criterion or date; a lookup over the gate view supplies the symbol and name of every gate, state, reason and mark that the task view names by key. Glance is the page at first (the legend lists; the task page shows id, title, assignee, parent, children, next gate and the status line); detail and provenance are the folds (below). Two gaps: the task view does not carry symbols, so a task page needs the gate view too, and `VIEWS.md`'s glance omits the authorisation state, which a proposal needs at glance (see the findings).
2. **Which mechanism fits each fold?** Both are built (`?folds=inline|fetch` forces one for every fold; the default mixes them). The sizes decide:
   - Criteria and synopses: inline. A criterion is about 40 bytes, and the `hx-*` attributes plus the fallback link of a fetched fold cost more: the legend is 4994 bytes with criteria inline and 6815 with each fetched.
   - Task description and references, requirements and dependents: inline (about 300 bytes each; the page grows by less than a fetch attribute set would add).
   - Junctions (3.5 to 4.1 KB), git facts (0.5 to 0.9 KB) and history (0.3 to 1.3 KB): fetched. For `9f31`: default page 3032 bytes, all folds inline 8562, all folds fetched 2787. The junction fold alone is 3806 bytes, which is why it is fetched. For the root `a1c0` the default page (2736) is smaller than the all-fetched page (2807), because its folds are tiny.
   - Every fragment route returns exactly its part: no `<html>`, no `<details>`, none of the page's other content (tests check the legend entry byte for byte, the history event by event, and that the history lacks the description and the title).
3. **Does the page work with script off?** Yes by construction and by test, for every fetched fold on every fixture and in both modes. A fetched fold is a native `<details>` whose body holds a plain link to the fold's own route; HTMX replaces the body on the first toggle (`hx-trigger="toggle once"`, `hx-target="find .fold-body"`). The same route without `HX-Request: true` returns a full page with the same fragment (`Vary: HX-Request`). No script of our own exists; the page loads only the HTMX include from `base.html`. The nested "show where each field resolves from" link in the junction fold is likewise a link with `hx-get`.
4. **Does every reference resolve to a link?** Yes. A test visits every task page with all folds inline and classifies every `href`. Gate names, states and reasons link to `/gate#gate-KEY`, `#state-KEY`, `#reason-KEY`; marks link to `#mark-SYMBOL`; each anchor exists in the legend (the test checks). Requirement targets, dependents, parent, children and ancestors that supply a field link to `/task/ID` and resolve for the fixtures. Assignees, contributors, reviewers, authors, committers and event actors link to `/person/EMAIL`; commits link to `/commit/FULLHASH`. This prototype does not serve the last two; the test checks only their form. A subproject task links to `/task/ID?sub=URL&at=PIN`, also unserved.
5. **Does the task page stay truthful for the hard cases?** Yes, for the four fixtures plus `4e2b`/`7b2e` as link targets:
   - Proposed (`3c5d`): a dashed note "Proposed. This task is not authorised." at the top, naming the authorities; the history still lists the earlier `authorised` event and the later `task` edit that made it proposed again; the git-facts fold names the deciding commit.
   - A junction that does not apply (`c07d`, reliability): the row reads "The gate does not apply." with the mark `—`, and no contributor.
   - An inherited field with its source (`9f31`, mockup reviewer): at provenance level, "reviewer ben@example.org (from 4e2b)" with the ancestor linked; a default reads "(default)", a field set on the task "(set on this task)", and `reviewer: assignee` reads "(the task's assignee)".
   - A recursive junction with snapshot and pin (`c07d`): subproject `firmware`, the task `f1a0` read at pin `cf5b169` (linked), at gate Design; the status line says it is read from the subproject; the git-facts fold says there is no status file of its own.
   - An unmet and due requirement (`c07d`): "unmet, not met, due" on the requirement, in red, linked to `9f31`, which lists `c07d` as a dependent in the same words. A `pending` condition (`3c5d`) reads "pending, not met, not due".
   - A parent (`a1c0`): "Parent none: this is the root", the children linked, "Rolled up from the child 3c5d", and "Derived from the status of 3c5d; no commit decides it."

## URL scheme assumed (task `49ce` owns the final one)

```
/gate                                  the legend; entries have ids gate-KEY, state-KEY, reason-KEY, mark-SYMBOL
/gate/{gates|states|reasons}/KEY       one entry (fragment; page without HX-Request)
/gate/provenance                       version and commits (fragment)
/task/ID                               the task page
/task/ID/{about|requirements|junctions|provenance|history}   a fold (fragment; page without HX-Request)
/task/ID/junctions?level=provenance    the junctions with the source of each field
/person/EMAIL   /commit/HASH           link targets, not served here
```

`?folds=inline` and `?folds=fetch` on `/gate` and `/task/ID` force the fold mechanism; they exist to measure. Fragment routes take `HX-Request: true`; an unknown parameter is ignored, since validation belongs to another task. Key names are in the path, so the three kinds of legend key (`undefined` is both a gate and a state) do not collide.

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./prototype/438a            # no network, browser, corpus or port
go test -v -run 'Sizes|Mechanisms|Links' ./prototype/438a   # the sizes and link counts
go run ./prototype/438a --render /task/9f31
go run ./prototype/438a --render /task/9f31/history --fragment
go run ./prototype/438a --addr 127.0.0.1:0      # prints the address; ctrl-C stops it
go run ./prototype/438a --render /gate --data DIR  # read the JSON copies from another directory
```

Options: `--addr`, `--render`, `--fragment`, `--data`. A long name behind one hyphen exits with status 2 and names the `--` form. The tests drive the handlers through `httptest`; one test runs the command. The server was also started on port 0 and fetched with `curl` (page, fragment, HTMX script all 200), then stopped.

## Test data

Copied unedited from tablo prototype `493e` at `main` of the weather-station corpus (ref `edb30d2`), from `/home/nbyoung/Projects/Tableaux/tablo`, built binary `493e`:

```
493e -repo <corpus>/weather-station -ref main -view gate               > testdata/gate.json
493e -repo <corpus>/weather-station -ref main -view task -task ID      > testdata/task-ID.json   (ID: 9f31 c07d a1c0 3c5d 4e2b 7b2e)
```

The output has all three levels (the default is provenance, which includes glance and detail). `4e2b` and `7b2e` are there so that the links from `9f31`, `a1c0` and `3c5d` resolve. `f1a0` is a subproject task: `493e` finds no such task in the parent repository.

## What it shows

Task `c07d` at first, trimmed (`--render /task/c07d`):

```
<h1><code>c07d</code> Node firmware</h1>
<dt>Parent</dt><dd><a href="/task/4e2b"><code>4e2b</code> Sensor node</a>, order 2</dd>
<dt>Status</dt><dd><a href="/gate#gate-design">📐 Design</a> <a href="/gate#state-nominal">🟢 nominal</a>
 2026-09-17, <a href="/person/ben@example.org">ben@example.org</a>: Sleep scheduler in progress
 <br>Read from the subproject <code>firmware</code>, task <a href="/task/f1a0?sub=firmware&amp;at=cf5b169..."><code>f1a0</code></a>, at pin <a href="/commit/cf5b169..."><code>cf5b169</code></a>, gate <a href="/gate#gate-design">📐 Design</a>.</dd>
...
<li class="req unmet"><a href="/task/9f31"><code>9f31</code> Sensor board</a> from 📐 Design to 🛠️ Implementation: Pin map and sensor bus.
<strong>unmet</strong>, not met, due.</li>
<details class="fold" id="fold-junctions" hx-get="/task/c07d/junctions" hx-trigger="toggle once" hx-target="find .fold-body" hx-swap="innerHTML">
<summary>Junctions</summary>
<div class="fold-body"><a href="/task/c07d/junctions">Show this on its own page</a></div>
```

The fragment `--render '/task/c07d/junctions?level=provenance' --fragment`, text only, trimmed:

```
⚓ Reliability prototype — The gate does not apply. (set on this task)
🛠️ Implementation 🪆 subproject firmware, task f1a0 (set on this task)
    Snapshot: the subproject task f1a0 read at pin cf5b169 of firmware, at gate 📐 Design.
🧩 Unit test 🤖 👀 contributor opus@example.org (set on this task); model claude-opus-5-5; reviewer ben@example.org (the task's assignee)
```

Page sizes in bytes (`go test -v -run Sizes`), with the fragment sizes in the order about, requirements, junctions, git facts, history:

| task | default | all inline | all fetched | fragments |
|------|--------:|-----------:|------------:|-----------|
| 9f31 | 3032 | 8562 | 2787 | 266 303 3806 892 1319 |
| c07d | 3118 | 8467 | 3021 | 101 320 4134 911 791 |
| a1c0 | 2736 | 6537 | 2807 | 190 63 3465 498 325 |
| 3c5d | 3044 | 7624 | 2839 | 206 323 3465 652 950 |

Legend: 4994 bytes with criteria inline, 6815 fetched. Each page carries about 700 bytes of inline CSS and the HTMX include.

## Verified and not verified

Verified: the HTML each route returns, by `httptest` assertions and by `curl` against the running server; fragment content; the no-script fallbacks (the link target serves the fold); link resolution; `go vet`, `gofmt`, `golangci-lint`.

Not verified: any behaviour in a browser. No `chromium`, `google-chrome` or `firefox` exists here. In particular, that HTMX 2 fires `toggle once` from a `<details>` element, replaces the fold body on first open and does not refetch after a close and reopen, rests on HTMX's documentation, not on a run. The same holds for how a screen reader or a phone announces a fold. Following `/gate#gate-design` does not open the matching `<details>` (the `:target` outline marks it); opening it needs a script or a design that renders linked entries open.

## Stand-ins and omissions

- Data: the JSON copies stand in for `tablo`. `--data DIR` reads other copies.
- The watcher, parameter validation, caching, `ref`, `person`, `window`, `columns` and `role` do nothing. The legend ignores `task` (the references that expand a gate for one task).
- The `/person/` and `/commit/` targets and subproject tasks are not served.
- The marks list is the list the data gives; the legend has no 👓 `review` reason, see the findings.
- Inline CSS stands in for the stylesheet the mockups will fix.

## Findings for the design gate

- **The task view carries no symbols.** A task page needs the gate view beside it for the symbol and name of every gate, state and reason it names. Either `tablo` joins them or every front end loads both.
- **The legend data lacks the reserved `review` reason.** `VIEWS.md` lists the reasons overloaded, blocked and 👓 review, and says `review` has a reserved meaning; the gate view lists only the first two, and the `review` symbol and synopsis are nowhere in the data.
- **Glance omits the authorisation.** `VIEWS.md` puts the authorisation state at detail, so a proposed task would look like an accepted one at glance. The prototype shows a note at the top whatever the level. The design should decide that the proposed state belongs to glance.
- **A proposal can follow an authorisation.** For `3c5d` the history has `authorised` on 2026-09-21 and a later `task` event on 2026-09-27, and the authorisation is `proposed` again. The page shows both; a reader may wonder why. The data does not say that the edit cancelled the authorisation.
- **Children are bare ids.** `detail.children` has no titles, whereas parent, requirements and dependents have them. A child link shows an id only.
- **The `url` form is not in the data.** `VIEWS.md` has three url forms (submodule path, same-repository path, absolute URL with commit) and says provenance shows how the `url` fixes the commit. The task view gives `url: firmware` and the pin but not the form, so the page cannot state how the pin was fixed.
- **`junction_sources` mixes vocabularies.** A source is `default`, a task id or, for the reviewer at `c07d`'s unit gate, the word `assignee`. The prototype maps the three; the contract should name them.
- **A subproject task needs a URL.** The snapshot names a subproject `url`, a task and a pin; no route names a task in another repository. The prototype assumes `/task/ID?sub=URL&at=PIN`.
- **The glance status of a snapshot.** At `c07d` the status commit is the submodule pin and `status_commit` in provenance is null; the page says so. The status's gate (design) comes from the snapshot, which the data does not state.
- **Models.** `VIEWS.md` lists the model check (the `Model:` trailer against the junction's model) under the task data; the task view carries only the model the junction states, so the page cannot say whether a commit agreed.
- **Fold costs.** Short folds are cheaper inline: criteria, synopses and a requirement list. The fetch pays from about 1 KB. The design can fix a rule, for example "a fold over 1 KB is fetched" and keep the choice in the template, not the data.
- **Anchors.** A reference to a legend entry is an anchor into a page of closed folds. The design has to decide whether the target opens (a script, or a legend entry page per gate at `/gate/gates/KEY`, which exists here as a fragment route with a full-page fallback).
