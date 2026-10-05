# Prototype 9a9c: the temporal templates

The history view and the audit view, rendered on the server with `html/template` from tablo's view data, with HTMX fragments so that pages stay small. The layout is provisional: no HTML mockup exists yet, so the prototype settles function, not look.

## Answers

1. **Does the history render as a timeline, grouped by date, with the status after each event?** Yes, newest first, one `<section>` per date. The status after an event comes from the data only for a `status` event, which carries gate, state, reason and note as recorded. Every other event (`task`, `authorised`, `reviewed`, `reaffirmed`, `pin`) carries no status, so the server replays the events in order (`replay` in `history.go`), per task. The replay needs the events before a range as a seed: for `W9..W13` the `task` event of `3c5d` shows `defined`, set by a status event at W4 outside the range, so the server also reads the whole history. Tablo should supply the status after each event (see Findings).
2. **Does a long history stay small?** Yes. The page shows the newest `limit` events (default 6). A "more" link carries `before=SEQ`, the number of the oldest event shown, so it stays valid when new events append. The link is a plain `href` and also carries `hx-get` with the same URL; HTMX swaps the fragment in place of the link, and with script off the link loads a whole page. Sizes in bytes (HTML only, HTMX script excluded):

   | Case | Bytes |
   |------|------:|
   | 17 events (the whole fixture), first page of 6 | 3677 |
   | 17 events, one page of all | 8022 |
   | 2000 synthetic events, first page of 6 | 3629 |
   | 2000 synthetic events, one fragment of 6 | 2604 |
   | 2000 synthetic events, 500 on one page | 213565 |

   The synthetic history repeats the fixture in the test (`longSource`); the first page does not grow with the history. The server still folds the whole range to replay statuses, so the cost moves from the page to the server.
3. **Do the range and the filters work as query parameters, with links?** Yes: `range`, `task`, `person`, `before`, `limit` on `/history`. Each event links to its commit (`/commit/HASH`), its person (`/history?person=EMAIL`), its task (`/task/ID`) and the history of that task alone (`/history?task=ID`); every link keeps the range. A "clear" link drops each filter, and a plain GET form sets them with script off. The filtered output matches 8ed1's own `-task` and `-person` output for the same corpus, event for event. The filter runs after the replay, so a person filter shows the status that other people set. `/commit/…` and `/task/…` are not served: they belong to other tasks.
4. **Does the audit group findings by the resolving action, and separate introduced from resolved?** Yes. Groups are keyed by the kind of action and who takes it ("Authorise: ada@example.org"). Each finding shows the rule, task with title, gate, commit with label, a tag for the range and the action text. Findings tagged `resolved` go to a separate "Resolved by the range" list since nothing remains to do; `introduced` and `standing` findings stay in their groups with a tag. Each group heading links to a group fragment (`&group=KEY`), which HTMX swaps in place. The two audits differ (below), so the view joins them.
5. **Do an empty history and a clean audit render as a plain statement?** Yes: `<p class="empty">No events in W13..W13.</p>`, `No findings.` and `No findings in W13..W13.`, inside the normal page, with no list and no more link.

### 8ed1 and dada compared

| | 8ed1 `-view audit` | dada |
|--|--|--|
| Output | one pretty-printed JSON array (not one object per line) | a Markdown table, or the line `No findings.` |
| Fields | `rule`, `severity`, `task`, `message`, `since`, `range` | Finding, Rule, Task, Gate, Commit, Action |
| Gate | none | yes |
| Commit | `since`, a hash | label and hash (`W12 1b8cfb1`) |
| Resolving action | none (`message` states the problem) | prose, with the person inside: `An authority (ada@example.org) commits ...` |
| Resolver | none | only inside the prose |
| Range tag (introduced, standing, resolved) | yes | none |
| Severity | yes | none |
| Rule set | PROPOSED, STALE, H1 (stand-ins) | proposed, S11, H1, H2, H3 |

One audit view needs all of them in one record: rule, severity, task, gate, commit, range tag, an action kind and a resolver as fields. The prototype joins the two on task and kind (`mergeFindings`) and takes the action kind from a table (`standInActionKind`) and the resolver from a regular expression over dada's prose. Both are stand-ins. Neither audit alone lists the same findings: for `W3..W13` 8ed1 reports `STALE 3c5d` and `PROPOSED 9f31` resolved, and misses `3c5d` proposed since W12, which dada reports (8ed1's README names the miss). dada has no stale rule.

## URL scheme assumed from the Server task 49ce

```
/history?range=A..B&task=ID&person=EMAIL&before=SEQ&limit=N
/audit?range=A..B&stale=DAYS&entry=NAME&group=KEY
```

A request with `HX-Request: true` returns the fragment of the same route (the events and the more link on `/history`, the groups on `/audit`), and the responses carry `Vary: HX-Request`. The scheme in 49ce is not readable from this worktree, so this is an assumption; `entry` selects a dada corpus entry and stands in for the one checked-out repository. Parameter validation here is minimal (`limit`, `before`, `stale` and the fixture names).

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./...                                   # from the worktree root
cd prototype/9a9c
go run . --addr 127.0.0.1:0                     # prints the address; stop with Ctrl-C
go run . --render '/history?range=W9..W13&limit=3'
go run . --render '/history?before=14&limit=3' --fragment
go run . --render '/audit?range=W3..W13&stale=3&entry=weather-station'
```

Options: `--addr HOST:PORT` (default `127.0.0.1:8642`), `--render URLPATH`, `--fragment`. A single hyphen before a long name exits with status 2. HTMX comes from `internal/web` (`/static/htmx/htmx.min.js`); the pages need it only to swap fragments in place and work without it.

## Test data

`testdata/` holds verbatim output, embedded with `go:embed`, so the tests need neither tablo nor the corpus. The corpus is `/home/nbyoung/Projects/Tableaux/tableaux/corpus/build/`; `E` is `go run ./prototype/8ed1 -repo <corpus>/weather-station` and `D` is `go run ./prototype/dada <corpus>/<entry>`, both run in `tablo`.

| File | Source |
|------|--------|
| `history-all.json` | `E -view history -range ..W13` |
| `history-W2..W6.json`, `history-W9..W13.json`, `history-W13..W13.json` | `E -view history -range W2..W6` (and the others) |
| `history-task-9f31.json` | `E -view history -range ..W13 -task 9f31` |
| `history-person-ada.json` | `E -view history -range ..W13 -person ada@example.org` |
| `audit-W3..W13-stale3.json` | `E -view audit -range W3..W13 -stale 3` |
| `audit-W5..W5.json`, `audit-W13..W13.json` | `E -view audit -range W5..W5` (and `W13..W13`) |
| `dada-weather-station.md`, `dada-review-by-non-reviewer.md`, `dada-model-mismatch.md`, `dada-unknown-trailer.md` | `D` for each entry |
| `dada-clean.md` | `D` on the entry `status-complete-early`, which prints `No findings.` |
| `gates.yaml` | `git show main:.tableaux/gates.yaml` in the `weather-station` repository |
| `global-tableau.json` | `prototype/886d/output/global-tableau.json` |

Nobody edited the files by hand, except that the `.jsonl` names became `.json` since 8ed1 prints a JSON array. Titles come from `global-tableau.json`; gate, state and reason symbols come from `gates.yaml`. `Source` is the seam where tablo replaces the fixtures.

## What it shows

```
$ go run . --render '/history?range=W9..W13&limit=3'
<section class="day">
<h2>2026-09-26</h2>
<li class="event" data-seq="3">
<a href="/commit/97e4231"><code>97e4231</code></a>
<a href="/history?person=ben%40example.org&amp;range=W9..W13">ben@example.org</a>
<strong>pin</strong>
<a href="/task/c07d">c07d</a> Node firmware
(<a href="/history?range=W9..W13&amp;task=c07d">history of c07d</a>) <em>pin firmware at cf5b169</em>
<span class="after">status after: 📐 design (no state recorded); no authorised event yet</span>
...
<p id="more"><a href="/history?before=3&amp;range=W9..W13&amp;limit=3" hx-get="/history?before=3&amp;range=W9..W13&amp;limit=3" hx-target="#more" hx-swap="outerHTML">Older events (3 more)</a></p>

$ go run . --render '/audit?range=W3..W13&stale=3&entry=weather-station'
2 findings to resolve, 1 introduced by the range; 1 resolved by the range.
Authorise: ada@example.org [1]
  proposed 3c5d Dashboard 📝 defined 1b8cfb1 W12 [not in the range data]
  An authority (ada@example.org) commits `Authorised: 3c5d`
Reaffirm: no resolver named [1]
  STALE warning 3c5d Dashboard 55f57c1 [introduced]
Resolved by the range
  PROPOSED 9f31 Sensor board 91aa774 [resolved]
```

(The audit output above is condensed from the HTML.) The tests assert through `httptest` the events, their order, the replayed statuses, the more links and fragments (the link and the `hx-get` agree, a split date reads "(continued)"), the filters against 8ed1's own filtered output, the groups, the group fragments, the empty and clean cases, the page sizes and the command line.

## Stand-ins and omissions

- Replayed status: `replay` folds `status` events per task. It also shows whether an `authorised` event came ("authorised event seen"), which is all the events say.
- Action kind per rule (`standInActionKind`) and the resolver (a pattern over dada's prose).
- The join of the two audits on task and kind, and `entry` for dada's fixture entry.
- `gates.yaml` read with a regular expression over its one-line mappings; a YAML parser replaces it.
- No watcher, caching, task page, commit page or provenance level; no committer, model, parent roll-up or subproject events.
- The date order is 8ed1's order; the page does not re-sort.

## Findings for the design gate

1. **Tablo should supply the status after each event.** Only `status` events carry it. The replay costs the whole history before a range, per task, and a person or task filter in tablo (`-person`) drops the events the replay needs, so the fold must run before the filter. The record should hold gate, state, reason and note after the event, and the symbols (gates.yaml holds them; 8ed1 gives none).
2. **No data states whether a task stands authorised.** `task` events do not say. The naive reading (proposed until an `authorised` event) is wrong for `c07d`, whose owner commit authorises it with no `authorised` event, so the prototype shows only "authorised event seen". The history needs an `authorised` flag after each event, or the audit's derived fact.
3. **The parent's roll-up is absent.** VIEWS.md's history replays the status "for a parent from its children"; 8ed1 holds no hierarchy, so the prototype omits it.
4. **The committer, the model and the task title are absent** from the events (the schema has no field). Titles came from 886d's global tableau.
5. **8ed1 prints a JSON array, not one object per line.** Nothing breaks, but the brief and its README say lines. dada prints only a Markdown table (`No findings.` when clean); the prototype parses it. The cell for the action holds the resolver inside prose, so a front end cannot group by resolver without parsing prose. The audit needs one JSON record with rule, severity, task, gate, commit, range tag, action kind and resolver, and both audits need the same rule set.
6. **8ed1's audit misses `3c5d` proposed since W12** that dada reports, and dada has no stale rule and no range. A range audit needs dada's derivations applied at both ends of the range.
7. **Within a commit the events order arbitrarily** (`reviewed`, `status`, `pin` of `97e4231`): a replay sees `reviewed` before the `status` of the same commit. The design should say how a commit's events order.
8. **The "more" cursor.** `before=SEQ` counts events from the start of the range, so it needs the range's full event count; tablo should offer a cursor or limit so the server does not read the whole range for a first page. Whether the audit also needs paging is open: its findings stay few, so the page lists every group in full.
9. **Open in VIEWS.md:** whether a range audit keeps `standing` and `resolved` (the prototype shows both, apart); whether `stale` counts from the wall clock or the range end (8ed1 uses the range end).

## Verified and unverified

Verified: `go test ./...`, `go vet`, `gofmt` and `golangci-lint` pass; output was read through `--render` and `httptest`. I never opened the pages in a browser: none is known to be installed, so HTMX swapping, the look, the keyboard and screen reader use, and the phone layout are unverified.
