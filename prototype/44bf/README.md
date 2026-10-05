# Prototype 44bf: status templates

Task `44bf` renders the global tableau, the contextual tableau, the work-blockage tree and the contributor work queue as `html/template` pages on the server. HTMX fetches fragments for expansion. The data comes from tablo prototype `886d`. The layout is provisional: no HTML mockup exists yet, so the styling is minimal and settles nothing.

## Questions and answers

1. **Does the tableau render as an HTML table?** Yes. One row per task, indented by `depth`; one column per gate in the window; one column per fold (`integrate..release 0`) with its count in the header; the cell holds the data's symbols (`🔴⛔`, `🤖👀`, `—`) and carries the state, reason and note in `title`. A cell before the task's current gate is empty unless `hist=1`. One template serves the global tableau, the task form and the person form. Every symbol, count, gate name and note comes from the JSON.
2. **Does a parent row expand in place?** Yes, with HTMX unverified in a browser (see Verified). The toggle fetches `<tr>` elements and replaces the parent row (`hx-target="closest tr"`, `outerHTML`) with the parent in its new state followed by its visible descendants. Collapse returns the parent row and one `<tr id="r-ID" hx-swap-oob="delete"></tr>` for each descendant, so the children leave the table. A test checks that every page and fragment nests correctly and that a fragment holds rows and nothing else. Page size in bytes, from the test log (global; person form for `ben@example.org`):

   | State                       | Global tableau | Person form |
   |-----------------------------|---------------:|------------:|
   | root collapsed              | 2682 (1 row)   | 2704 (1 row)  |
   | one level open (`open=a1c0`)| 4420 (4 rows)  | 3533 (2 rows) |
   | whole tree (`open=all`)     | 5489 (6 rows)  | 4649 (4 rows) |

   About 2.6 KB is the fixed page (head, style, header row); each row costs about 0.5 to 0.9 KB with nine gate columns. The corpus has six tasks, so the numbers show the shape only: a row costs the same in a large project, and a collapsed page does not grow with the tree.
3. **Does the blockage tree render with causes at the root?** Yes. Three causes in the data's order (largest first), each with its resolver, action and held count, each expanding to the tasks it holds. Each entry carries exactly one link to its task (`/task/ID`); `9f31` shows three links on the fully open page: two causes and one held line.
4. **Does the work queue render as a personal page?** Yes. One page per person, items in the data's order (`ada`: authorisation owed `3c5d`, work ready `7b2e`, reaffirmation `9f31`, work waiting `9f31`). An item expands to a brief. The data holds no brief, so the brief is a block named "Brief, stand-in" (`standinBrief` in `server.go`) that lists the item's own fields, lists the parts of `VIEWS.md` `### The brief` the data lacks, and prints the reproduce command as VIEWS.md writes it. `ben`'s empty queue shows "Nothing to do."
5. **Does each page work with script off?** Yes by construction and by test. Every toggle is `<a href=PAGE hx-get=FRAGMENT>`. Without script the browser follows `href`, which returns the full page with that row open (`#r-ID` anchors to it). Tests follow every toggle's `href` on four tableau pages, the blockage page and the queue page. The only script on a page is the HTMX include from `base.html`; the prototype writes no script of its own.

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./prototype/44bf
go run ./prototype/44bf --addr 127.0.0.1:0          # prints the address it listens on
go run ./prototype/44bf --render '/tableau?task=4e2b&open=all'
go run ./prototype/44bf --render '/tableau/rows?row=a1c0&open=a1c0' --fragment
```

Options: `--addr HOST:PORT` (default `127.0.0.1:8642`), `--render URLPATH`, `--fragment` (with `--render`: send `HX-Request: true`). A single-hyphen long option exits with status 2 and names the `--` form.

## URL scheme assumed (task `49ce` owns the final one)

| Route | Parameters | Returns |
|---|---|---|
| `/tableau` | none: global; `task=ID` or `person=EMAIL`: contextual; `window=N`, `cell=state-at-gate\|state-at-next`, `hist=1`, `open=ID,ID` or `open=all` | page |
| `/tableau/rows` | the same, plus `row=ID` | `<tr>` elements: row `ID` in the state `open` gives it, then its visible descendants; or, when it is closed, the row and one out-of-band delete per descendant |
| `/blockage` | `open=KEY,KEY`; a key is `kind-task` | page |
| `/blockage/cause` | `key=KEY`, `open=` | one `<li>` |
| `/queue` | `person=EMAIL`, `open=N,N` (item positions) | page |
| `/queue/item` | `person`, `n=N`, `open=` | one `<li>` |
| `/task/ID` | | 501: the task view belongs to another task |

The `open` set in a toggle's link is the set the page held at render time with that row flipped. The fragment route applies it as given and does not flip it again. The route that returns a fragment is a separate path, not the page route with a header; a page route ignores `HX-Request`. Parameters at their default (`window=1`, `cell=state-at-gate`, no `hist`) stay out of generated links. The prototype builds no role entry point, default window per role, column hiding or browser storage (task `db74`).

## Test data

The `testdata/*.json` files are unedited copies of `tablo/prototype/886d/output/` (tablo `00f8f68`), made by the commands in that prototype's README, run against the `weather-station` corpus at `main`:

| File | Command (from `tablo`) |
|---|---|
| `global-tableau.json` | `go run ./prototype/886d tableau` |
| `global-tableau-window0.json` | `go run ./prototype/886d tableau -window 0` |
| `global-tableau-state-at-next.json` | `go run ./prototype/886d tableau -cell state-at-next` |
| `contextual-task-4e2b.json` | `go run ./prototype/886d contextual -task 4e2b` |
| `contextual-person-ben.json` | `go run ./prototype/886d contextual -person ben@example.org` |
| `work-blockage-tree.json` | `go run ./prototype/886d blockage` |
| `queue-ada.json`, `queue-ben.json`, `queue-dan.json` | `go run ./prototype/886d queue -person <name>@example.org` |

The server picks the tableau file by the fields inside it (`view`, `task`, `person`, `window`, `cell_rule`), not by file name. A combination with no file (for example `window=0` with `cell=state-at-next`) returns 404. That is a stand-in for tablo computing any variant.

## What it shows

`--render '/tableau?task=4e2b&open=all'`, trimmed to the cells:

```
Task | undefined..mockup 0 | ⚙️ function | ⚡ performance | ⚓ reliability | 📐 design | 🛠️ implementation | 🧩 unit | integrate..release 0 | Status
4e2b   .  🔴⛔ 🧑 🧑 🧑 🧑 🧑 .      2026-09-17 Barometer ICs on 14-week backorder (rolls up from 9f31)
9f31   .  🔴⛔ 🧑 🧑 🧑 🧑 🧑 .
c07d   .  .    .  .  🟢 🪆 🤖👀 .   2026-09-17 Sleep scheduler in progress
```

(`.` is an empty cell.) The same fragment route returns, with `--fragment`:

```
<tr id="r-a1c0" data-depth="0">
<th scope="row" class="task" style="padding-left:0em"><a class="tog" href="/tableau?open=4e2b#r-a1c0" hx-get="/tableau/rows?open=4e2b&amp;row=a1c0" hx-target="closest tr" hx-swap="outerHTML" aria-expanded="true" rel="nofollow">collapse</a> <a class="id" href="/task/a1c0">a1c0</a> Weather station</th>
...
<tr id="r-4e2b" data-depth="1"> ...
```

A collapse fragment (`row=a1c0` with no `open`):

```
<tr id="r-a1c0" data-depth="0"> ... </tr>
<tr id="r-4e2b" hx-swap-oob="delete"></tr>
<tr id="r-9f31" hx-swap-oob="delete"></tr>
...
```

## Verified, and not

Verified: `go test ./...`, `go vet`, `gofmt` and `golangci-lint` pass; the tests drive the handlers through `httptest` and assert cells, counts, symbols, order, fragment contents, tag nesting and the script-off links. The server ran on a random port and served `/tableau` and the HTMX script (HTTP 200) through `curl`.

Not verified: no browser exists here (`chromium`, `google-chrome` and `firefox` are absent), so no HTMX swap ran. These stay open until someone runs them in a browser: that `outerHTML` on a `<tr>` with a response of several `<tr>` elements swaps as expected; that `hx-swap-oob="delete"` on `<tr>` elements removes the rows (HTMX 2 documents `delete` for out-of-band swaps; `<tr>` fragments need its table-aware parsing); and how any of it reads, looks or sounds with assistive tools or on a phone.

## Stand-ins, omissions and findings for the design gate

- **No brief in the data.** 886d's queue "omits the brief" (its README). The stand-in brief repeats the item and lists what it lacks. tablo needs a brief view and the queue a schema that carries the junction, criteria, requirements and commit instructions.
- **Historical cells.** The data holds the junction marks of historical cells (`4e2b` carries `🧑` at defined and mockup), blank only at `undefined`. The template blanks every cell before the row's current gate and the `hist=1` parameter shows them. This puts the rule in the front end; the design should decide whether tablo returns the cells already blanked and flags the historical ones. With `hist=1` the `undefined` cell stays empty because the data gives it no mark.
- **Cell rule uses `status.gate`.** The row's current gate is the gate its status names, also under `state-at-next`, where the state symbol sits in the next gate's column. A parent's derived gate works the same way.
- **Fold cells.** The fold appears as a header column with its count. The body cells are empty, since the data gives only a total per fold (and per gate in `by_gate`), not a count per row. A design that shows a per-row mark in the fold needs a field for it. Headers print `from..to`, because the data gives no symbol for a folded gate. The count is leaves only, as 886d says.
- **Blockage tree depth.** The data lists the held tasks flat, one level. VIEWS.md says a held task holds its dependents in turn; the data has no second level, so the template shows none. The data has no per-cause key either: the prototype builds `kind-task`, which two causes of one kind and task would share.
- **Stale links after a swap.** A toggle's link carries the open set as of the page render. After a fragment swap the other rows' links still carry the old set, so a no-script follow of one of them (or a reload of its URL) forgets an expansion done through HTMX. The design can add `hx-push-url` on each toggle so that the address bar holds the open set, or have the server render every row's links relative to a state the client sends.
- **Person form.** `collapsed_children` counts children outside the view (`a1c0`: 2). The row shows the count but offers no expansion, since the data does not list them; expanding them needs a second query.
- **`<style>` in the body.** `base.html` has no slot in `<head>`, so the prototype puts its small style block at the top of the body. The design should give `base.html` a head block.
- **Task links.** Each row and entry links to `/task/ID`, which answers 501 here.
- **Questions for VIEWS.md.** Does the fold show a count of leaf tasks or of rows? Does a task's row open by default for the owner role (`db74`)? Does the work queue item for an authorisation (no gate) have a brief?
