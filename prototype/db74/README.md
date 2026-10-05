# Prototype db74: progressive disclosure

Task `db74` builds the role-specific entry points and the levels of the two tableaux: the owner opens the root collapsed, a contributor opens at the parents of their tasks in a gate window, the viewer hides and shows columns, and every state has a link. This prototype answers the risky questions. The layout is provisional: the HTML is a plain table with text controls, because no mockup exists yet. Task `44bf` owns the full templates and the lazy loading of rows.

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./prototype/db74
go run ./prototype/db74 --addr 127.0.0.1:0                    # prints the address; open /tableau?as=ben@example.org
go run ./prototype/db74 --render '/tableau?as=ben@example.org&hide=reliability'
go run ./prototype/db74 --render '/tableau?as=ben@example.org' --fragment   # HX-Request: true
```

Options: `--addr HOST:PORT` (default `127.0.0.1:8642`), `--render URLPATH`, `--fragment`, `--as EMAIL` (the person for a URL without `as`), `--window N` (columns either side of the next gates, default 1). `-addr` exits with status 2 and names `--addr`.

## The answers

### 1. One URL for every state

```
/tableau?as=EMAIL&open=ID,ID&hide=GATE,GATE&show=GATE,GATE
```

- `as`: the person. Absent, the server's `--as` person applies, and nobody is an observer.
- `open`: the open rows, task ids sorted. Absent means the role's default; `open=` means every row collapsed. A closed row's descendants leave the set when the row closes, so a collapsed state has one spelling.
- `hide` and `show`: the column choice as a difference from the default window (question 4). Gates sit in gate order. Either parameter present, even `hide=` empty, means "a choice is made".
- Every parameter that equals its default drops out. `/tableau?as=ben@example.org` is Ben's landing page and stays that short. `@` and `,` stay unescaped.

The server decodes the query, normalises it (unknown ids and gates dropped, sets ordered, hide beats show) and renders. It reads no cookie, header or file besides the URL; a test sends a stored-choice cookie and a language header and gets the same bytes. Each expand, collapse, expand-all, collapse-all, hide, show and default-columns control is an `<a href>` to the next state with `hx-get` (the same URL), `hx-target="#tableau"`, `hx-swap="outerHTML"` and `hx-push-url="true"`, so with HTMX the fragment swaps in and the address bar holds the link, and without it the browser follows the link. The page also prints its own canonical link.

Tests: `TestEveryLinkRoundTrips` walks the controls from each person's entry point (4000 states, bounded) and checks that every `href` equals `Link(Decode(href))` and that `hx-get` equals `href`. `TestControlsLeadToExpectedStates` lists hand-written next states. `TestFragmentAgreesWithFullPage` checks that the full page contains the fragment byte for byte (a history restore request gets the full page). The ordering rules mean a link stays the same when rows reorder: ids, not positions.

### 2. The stored column choice

A server that "holds no column policy" gets the choice from the URL, so the browser has to put the choice in the URL. `cols.js` (922 bytes with its comments, 665 minified by whitespace; inline in the full page, not in the fragment) does it:

- On load, a URL without `hide` or `show` and a non-empty stored choice (`localStorage` key `tableaud.cols`, a string such as `hide=reliability&show=release`) goes through `location.replace` to the same URL with the choice appended. This costs one extra request and shows the default for a moment. `replace` adds no history entry, so Back does not loop.
- A URL with `hide` or `show` (a shared link) wins and the script does nothing.
- A click on a hide, show or default-columns control (`<a data-c="...">`) stores that control's choice, or clears the store for the explicit empty choice `hide=`. Row controls carry no `data-c`.

**Decision: opening a shared link does not overwrite the stored choice.** Following a link is reading; changing a column is deciding. The first column control the viewer then uses starts from the shared state, so it stores the shared choice with that change. The stored choice is a difference from the default (question 4), so one stored string serves the owner's page and a contributor's page.

The logic sits in the pure function `tableaudCols(search, stored)`. Go implements the same function (`ApplyStored`) and both follow the cases in `testdata/cols-cases.json`; the test runs only the Go side. `TestScriptShape` checks that `cols.js` holds the pieces the function needs, which proves little. `TestStoredChoiceReproducesTheState` checks that the string a control stores, appended to a URL without a choice, gives that control's own URL.

Script off: the server shows the default window for a URL without a choice, and every control still works as a full page load. Nothing remembers the choice; a link still carries it. HTMX off is the same.

### 3. The entry point follows the role

`RoleOf` reads the data only:

| Role | Rule | Entry |
|------|------|-------|
| owner | the person is the assignee of the root row in authority delegation (`493e -view authority -level glance`) | global tableau, root collapsed, window around every next gate |
| contributor | the person's contextual tableau has a row with `role: corner` | that tableau, open at the ancestors of the corner rows, window around the next gates in it |
| observer | anyone else: unknown email, known email with no corner row, no email | global tableau, root collapsed, labelled `observer` |

Ada (the owner, who also has corner rows) enters as owner. Ben opens `a1c0` and `4e2b` over `9f31` and `c07d`; Dan opens `a1c0` over `4e2b`, `7b2e`, `3c5d`. `opus@example.org` (886d returns `rows: null`) and strangers observe.

What the data must state for this to be sound:

1. **The owner.** No tableau view states it. The prototype borrows the root's assignee from the authority view, and "assignee of the root" is a convention, not a stated role. A `viewer.role` or `owner` field would settle it.
2. **The role.** The prototype derives "contributor" from a `corner` row. A person who only reviews or holds authority is an observer here. A role field in the person form (`owner`, `contributor`, `reviewer`, `authority`, none) would end the guessing, and tell a stranger from a known person with no tasks.
3. **The parent id.** Rows carry `depth` and `parent` (a boolean); the prototype rebuilds each parent id from depth and order. A `parent` id per row removes that.
4. **The contributor's expansion.** A parent outside the focus (`4e2b` in Dan's view) has `collapsed_children: 2` and no child rows. Dan cannot expand it, and the page says "(2 outside this view)". Whether such a row links to the global tableau is a design choice.
5. **Owner and contributor together.** The prototype has no role switch. Ada never sees her contributor corner. f394's `r` key has no URL form here; `view=` or `role=` would add one.

### 4. The default window and a link after the project moves

The server computes the window from the data: `next_gates_in_view` with `--window` columns either side, over the whole view whatever is expanded. The prototype reads the all-columns files (`window20`) and folds the outside itself: each folded run shows the count of leaf tasks (by status gate) over the view, a per-row count of the leaves under that row, and a control per gate. `TestDefaultWindowEqualsTablo` shows the computed window and fold counts equal what tablo computed in the `window1` and `window0` files for the global tableau, Ben, Dan and Ada.

**Decision: a link names a difference from the default** (`hide=` and `show=`), not the absolute columns. Reason: the stored choice has to outlive the window. A viewer who hides ⚓ wants it hidden next month, not the sender's absolute window frozen. Absolute columns would pin a shifted project to the old window, and the stored choice would say "show undefined..unit" after the work has moved past them, which is a policy, held in the browser. A difference also stays short (empty for the default, one word for a hide).

The cost: after the project moves on, an old link opens as "today's default with the sender's differences", not as the sender's exact columns. `TestLinkFollowsTheWindow` moves the next gates by three columns. The link `?as=ada@example.org&hide=design&show=release` shows `undefined..unit` plus `release` before, and `reliability..validate` plus `release`, with `design` hidden, after. A sender who needs a frozen picture needs the static export, not a link.

### 5. A hidden column against a folded one

Yes, and the two differ in markup and in control. A hidden column stays in place as a `th.hidden` marker with the control `show GATE` and an empty `td.hidden` in each row. A folded column outside the window joins a run `th.folded` that holds a count and one `show` control per gate with that gate's own count; each row has a `td.folded` with the count of its leaves. `TestHiddenMarkerDistinctFromFold` asserts both. Hiding a column that only a choice showed (outside the window) returns it to the fold, not to a marker.

## What it shows

Ada, the owner, `--render '/tableau?as=ada@example.org'`, trimmed to the facts:

```
ada@example.org | role: owner | global-tableau | window undefined..unit
  shown ❔ undefined [hide -> /tableau?as=ada@example.org&hide=undefined]
  ... shown 🧩 unit
  folded 0: [🖼️integrate 0 -> ...&hide=&show=integrate] [🌍validate 0 -> ...] [🚀release 0 -> ...]
 row a1c0                       (the root alone; its expand control leads to ?as=ada@example.org&open=a1c0)
 link: /tableau?as=ada@example.org
```

Ben, `?as=ben@example.org&hide=reliability&show=release` (a shared link: ⚓ hidden, 🚀 shown from the fold):

```
ben@example.org | role: contributor | contextual-tableau | window defined..unit
  folded 0: [❔undefined 0 -> ...&hide=reliability&show=undefined,release]
  shown defined, mockup, function, performance
  hidden ⚓ [show reliability -> /tableau?as=ben@example.org&hide=&show=release]
  shown design, implementation, unit
  folded 0: [integrate] [validate]
  shown 🚀 release [hide -> /tableau?as=ben@example.org&hide=reliability]
 rows a1c0 4e2b 9f31 c07d
```

## Test data

All in `testdata/`, copied unedited from tablo (`/home/nbyoung/Projects/Tableaux/tablo`, commit `00f8f68`), run on `weather-station` at `main`:

| File | Source |
|------|--------|
| `global-tableau-window1.json`, `global-tableau-window0.json`, `contextual-person-ben-window1.json` | `prototype/886d/output/` (`global-tableau.json`, `global-tableau-window0.json`, `contextual-person-ben.json`) |
| `global-tableau-window20.json` | `go run ./prototype/886d tableau -window 20` |
| `contextual-person-{ada,ben,dan,opus}-window20.json` | `go run ./prototype/886d contextual -person P@example.org -window 20` |
| `contextual-person-{ada,dan}-window1.json` | the same with `-window 1` |
| `authority-glance.json` | `go run ./prototype/493e -repo <corpus>/weather-station -ref main -view authority -level glance` |
| `cols-cases.json` | written for this prototype: the shared cases of the stored-choice function |

The server renders from the `window20` files (every gate as a column), `authority-glance.json` and nothing else. The other files check its window and counts against tablo's.

## Verified and not verified

Verified by `go test ./...` through `httptest`: every state in the tables above renders as stated, the entry points per person, every control link round trips, fragment and full agree, the window and fold counts equal tablo's, the link follows a shifted window, the command line. Checked by hand with `curl`: the server starts on a free port, serves `/tableau` and the vendored HTMX script, and answers a fragment request.

Not verified: no browser exists here (no `chromium`, `google-chrome` or `firefox`) and no `node`. The script, `localStorage`, `location.replace`, the click handler, and the HTMX swap with `hx-push-url` never ran. The redirect round trip, the flash of the default page, and whether an HTMX history restore runs the inline script again are unknown. Only the Go twin of the pure function ran.

## Stand-ins and omissions

- The owner comes from the authority view's root assignee; the role from a `corner` row; each parent id from depth and order (above).
- The `window20` files stand in for a tablo call with a `columns` list. The server computes the window and the fold counts that tablo gave in `window1`; the product should ask tablo for them, or for all gates, and apply the choice on top.
- The role label and the person's choice of view are fixed by the person; there is no switch of role and no `task` form (tablo's `contextual-task-4e2b` is not read).
- No historical-junction switch, no `ref`, no levels beyond expansion (glance is the collapsed root, detail the expanded rows), no provenance, no lazy loading, no export.
- Styling is absent, and the controls are words (`hide`, `expand`).

## Findings for the design gate

1. Make the owner and the role explicit in tablo's view data, and the parent id per row (above). Without them the entry point rests on three conventions.
2. The URL grammar is a difference from the default for columns and an explicit list for rows. The design must decide whether `open` should also be relative (it is absolute: new tasks do not open on their own).
3. The stored choice is applied by a redirect, which costs a round trip and a flash. A cookie would avoid it but gives the server the choice outside the URL, which the brief rules out; deciding how to live with the flash is the design gate's question. The same function ought to run in the static export, where there is no server: the script would hide and show columns there.
4. A shared link means "default plus differences", not the sender's exact columns, once the project moves. If exact reproduction matters, add an opt-in absolute form (`cols=undefined..unit`), or rely on the static export.
5. A contributor cannot expand a parent outside the focus. A link from such a row to the global tableau, or a `view` parameter, is a design choice.
6. VIEWS.md says the contextual tableau's window spans the next gates "of the tasks in view": for Ben that includes the spine's `a1c0` (next gate mockup), which widens his window to `defined..unit` though his own next gate is implementation. f394 windows around his own tasks alone (`design..unit`). The design must choose.
7. Ada's person view marks `9f31` and `7b2e` as corners: for the owner who contributes, the role switch of f394 has no counterpart yet.
