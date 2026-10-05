# Prototype e2b6: structural templates

Task `e2b6` Structural templates, gate `function`. The authority delegation view and the task assignment view as `html/template` pages that the server renders from tablo's view data, with HTMX fetching fragments for expansion. The layout is provisional: the prototype settles function, not look, and the stylesheet is empty.

## Questions and answers

1. **Does a tree of unknown depth render and expand with `html/template` and HTMX, with the state in the URL?** Yes, in both forms.
   - Fragments (default): the page shows the root collapsed. A node's toggle is a link (`href` for script off) that also carries `hx-get` to `/authority/node/{id}?state=open|close`. The fragment is the node's own `<li>` with the open items below it, swapped over the node (`outerHTML`). The fragment's own template calls itself, so depth does not matter: the test opens a chain of 60 levels node by node and a reload of the final URL shows all 61.
   - State: the `open` query parameter lists the open node ids. A stale link cannot corrupt it: the fragment route reads the browser's current address from `HX-Current-URL`, adds or removes the node, renders, and answers with `HX-Push-Url` set to the new page address. A fragment address without `HX-Request` redirects (303) to the page in the same state.
   - Whole tree (`mode=full`): one recursive template over the tree with native `<details>`; `open` only sets the initial state, since native toggling cannot write the URL without script of our own (not tried).
   - Sizes in bytes (`go test -v -run TestPageSizes`), synthetic tree of 1093 tasks (depth 6, three children each; `synthetic` in the test, not corpus data):

     | Form | Bytes |
     |------|------:|
     | whole tree, native details, nothing open | 403335 |
     | fragment page, root only | 1453 |
     | one fragment (root's children) | 2559 |
     | fragment page, root open | 3422 |

     For weather-station at `main` with the root open: 3668 whole, 3111 fragment page (six tasks: too small to matter). Both forms render with the same row template, so the whole tree costs about 370 bytes per task at glance; it grows with the level.
2. **Does the proposed-only filter keep the path from the root to each proposed task, and work as a plain link?** Yes. tablo's `-proposed` output lists the proposed task alone, without its ancestors (see `testdata/authority-main-proposed.json`), so a tree built from it would have no root. The server therefore filters the full view itself: a node stays when it is proposed or has a proposed descendant. The filtered tree is a tree (the test checks every shown task has its parent shown, on the corpus and on a deep synthetic tree) and the path opens by default. The filter link is `/authority?proposed=1`, a plain `href`; toggles inside the filtered view keep `proposed=1`. The test follows it with a plain GET.
3. **Does the authority view state who may accept what?** Yes. Each row shows the assignee (a dot where it equals the parent's), "accepted by" the nearest authority (the parent's assignee, from `detail.rows[].authorities[0]`; "·" where it equals the parent's own), and the state authorised or proposed in words. Detail adds the whole chain and each parent's junction defaults. Provenance names the deciding commit (hash, date, subject, author, committer), the way it accepts, and says why a proposed task stays proposed when neither author nor committer is an authority (`3c5d`). Off the trunk (`ref=W3`) every task reads proposed, the page says the commit is off the trunk's first-parent line, and provenance says no commit decides.
4. **Does the assignment view give one page per email, an index and an honest unknown page?** Yes. `/people` is the index (counts and models from the whole view); `/person/{email}` lists assigned tasks with gate and state, contributes next, reviews next, the models of an agent, the subtrees it has authority over, and at detail and provenance every junction and the task that states each field. The list lengths equal the counts, tested for every person. An agent with no task (`opus@example.org`) shows its model and zero counts. An unknown email gets a page that says the project does not know it, why, and links every known email. I chose status 404 for it, since the resource does not exist; the body is a full page, not a bare error. The design gate may prefer 200.
5. **Does every task and person link to its definition and page?** Yes. A test removes every anchor from each page and finds no email outside one; every task id on a person page sits in an anchor. Assumed URL scheme (task `49ce` owns the final one):

   | Route | Page |
   |-------|------|
   | `/authority` | authority delegation; query `ref`, `level`, `proposed=1`, `mode=full`, `open=id,id` |
   | `/authority/node/{id}` | the node's fragment; same query plus `state=open\|close` |
   | `/people` | task assignment, every person |
   | `/person/{email}` | task assignment, one person; query `level` |
   | `/task/{id}` | the task definition: not served here (answers 501) |

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./...
go run ./prototype/e2b6 --addr 127.0.0.1:0          # prints the address
go run ./prototype/e2b6 --render '/authority?proposed=1'
go run ./prototype/e2b6 --render '/authority/node/a1c0?state=open' --fragment [--current '/authority?open=a1c0']
```

`--render` prints the page to standard output and the status and `HX-Push-Url` to standard error. `--current` is an addition: it sets `HX-Current-URL`. The tests use `httptest` with no network, browser or port.

## Test data

Copies of the output of tablo prototype `493e`, not edited. `R` is the weather-station corpus, `W3` is the commit `91aa7747f22ef4bb66becb6eab5d5f1f9c28836d` of `weather-station.labels.txt`.

| File | Command (`go run ./prototype/493e -repo R ...`) |
|------|---------|
| `authority-main.json` | `-ref main -view authority` |
| `authority-main-proposed.json` | `-ref main -view authority -proposed` |
| `authority-W3.json` | `-ref <W3> -view authority` |
| `assignment.json` | `-ref main -view assignment` |
| `assignment-{ada,ben,dan,opus}.json` | `-ref main -view assignment -person <name>@example.org` |
| `assignment-unknown.json` | `-ref main -view assignment -person zed@example.org` |

The server embeds them, so the binary runs without tablo. Every id, title, count, state and commit comes from them. The assignment pages look task titles up in the authority view at `main`.

## What it shows

`--render '/authority?proposed=1&level=provenance'` (trimmed):

```
<li id="n-a1c0"> ... a1c0 Weather station  ada@example.org  no authority above  authorised
  deciding commit 68d9021, 2026-09-15, "Plan the weather station"; ... way: commit; the author is the authority
  <li id="n-3c5d"> ... 3c5d Dashboard  dan@example.org  accepted by ada@example.org  proposed
    authority chain: ada@example.org
    deciding commit 1b8cfb1, 2026-09-27, "Revise the dashboard scope"; ... neither author nor committer is an authority, so the task stays proposed
```

`--render /people`: `ada 3 2 0 —`, `ben 2 0 0 —`, `dan 1 1 0 —`, `opus 0 0 0 claude-opus-5-5`. `--render /person/zed@example.org` (status 404): "The project does not know zed@example.org: no task names this email as its assignee, and no junction names it as a contributor or a reviewer", then the four known emails as links.

## Verified and not verified

Verified: the tests above, `gofmt`, `go vet`, `golangci-lint`, and one `curl` against a running server on a free port (page 200; fragment with `HX-Push-Url: /authority?open=a1c0`).

Not verified: any browser. No browser is installed here, so I did not run HTMX itself: the `hx-*` attributes, the `outerHTML` swap, `HX-Push-Url` handling and the vendored script's use of `HX-Current-URL` follow HTMX's documented behaviour but ran nowhere. The same holds for screen readers and phones. HTMX does not swap 4xx responses by default; no fragment route returns one on the normal path.

## Stand-ins and omissions

- Data comes from fixtures, not tablo; `Source` is the seam. The assignment view has fixtures at `main` only, so `ref` applies to the authority view (`main`, `W3`) alone.
- The task definition page does not exist (`/task/{id}` answers 501). Parameter validation, caching, the watcher, role entry points and column choices belong to other tasks; an unknown `level` falls back to glance and an unknown `ref` answers 404.
- Styling: none. Marks use words, not colour alone.
- Closing a node removes only that node from `open`; ids of hidden descendants stay in the URL and reappear on reopening.

## Findings for the design gate

- **Filtered data has no path.** `-proposed` returns the proposed tasks without their ancestors and with their depths; a front end that wants a tree needs the full view anyway. Either tablo keeps the ancestors under the filter (marked as context) or the front end filters. Rows are a preorder list with `depth`; the parent is implicit.
- **The root has no authority.** `authorities` is empty for `a1c0`, so the owner of the root accepts nothing in the data; the page says "no authority above". VIEWS.md says the owner is an authority.
- **Titles missing in junctions.** Assignment junctions carry task ids only; titles needed a second view.
- **`by` is empty for a proposed task** and the commit named is the latest change, not a deciding one. The page words this; the field's meaning needs a line in VIEWS.md.
- **Native `<details>` cannot keep its state in the URL** without script; the fragment form can, and costs one request per expansion. The whole-tree form is a candidate only for the static export, where no server answers.
- **State in `HX-Current-URL`** keeps one source of truth, but a proxy or a privacy setting that drops the header breaks it; the fallback is the `open` set in the fragment URL (stale after other expansions).
- **Unknown email:** 404 or 200 with an explanation (above).
- **Window and level:** the assignment page ignores `window` since the fixtures use the default; the per-person page lists every gate at detail.
