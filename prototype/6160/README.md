# Prototype 6160: static export

Task `6160` Static export, gate `function`.

`tableaud export --out DIR` writes every view at every disclosure level as a self-contained HTML bundle, for continuous integration to publish where no daemon runs. This prototype builds the export three ways, measures them, and renders the same templates through a tiny daemon handler for comparison. The layout is provisional: no HTML mockup exists yet, so the styling is a few lines of inline CSS and the prototype settles function, not look.

## Run

```
export PATH=$PATH:/usr/local/go/bin
go test ./prototype/6160
go run ./prototype/6160 export --out /tmp/public                  # --mode details (default)
go run ./prototype/6160 export --out /tmp/public --mode fragments
go run ./prototype/6160 export --out /tmp/public --mode states
go run ./prototype/6160 export --estimate 200 --branching 5      # state counts for a tree of 200 tasks
```

Options: `--out DIR` (required; the directory must be absent or empty), `--mode details|fragments|states`, `--data DIR` (read the JSON from a directory instead of the embedded `testdata/`), `--estimate N` and `--branching B`. The command refuses a long name behind one hyphen (`-out`) with exit status 2. `--addr` and `--render` do not apply to an export, and `--base` proved unnecessary (question 2). The word `export` before the options is optional.

Exit status: 0 on success, 1 when the directory is not empty, the data is bad or a view fails to render, 2 for a usage error.

## The test data

`testdata/` holds copies of the JSON that the tablo prototypes print, taken at `main` of the `weather-station` corpus (`edb30d2`). Nobody edited them by hand. The tests render from these copies and need neither `tablo` nor the corpus.

| File | Source (run from `tablo/`) |
|------|---------------------------|
| `tableau.json` | `886d/output/global-tableau.json`, from `go run ./prototype/886d tableau` |
| `queue-ada.json`, `queue-ben.json`, `queue-dan.json` | `886d/output/queue-*.json`, from `go run ./prototype/886d queue -person ada@example.org` |
| `task-<id>.json` for `a1c0 4e2b 9f31 c07d 7b2e 3c5d` | `go run ./prototype/493e -repo <corpus>/weather-station -ref main -view task -task <id>` |
| `gate.json` | `go run ./prototype/493e -repo ... -ref main -view gate` |
| `assignment-ada.json`, `-ben`, `-dan`, `-opus` | `go run ./prototype/493e -repo ... -ref main -view assignment -person <person>@example.org` |

The export reads task ids from the tableau rows and people from the file contents (`params.person` in an assignment, `person` in a queue). It hard-codes no symbol, title, count or id. A task that the tableau lists but that has no file is an error.

## What the bundle holds (details mode)

17 files: `index.html`, `manifest.json`, `tableau.html`, `gate.html`, six `tasks/<id>.html`, and for each of the four people `people/<email>/assignment.html` plus, for the three who have one, `people/<email>/queue.html`. The pages are one of a kind (tableau, gate), one per task and one per person.

```
$ go run ./prototype/6160 export --out /tmp/public
wrote 17 files, 45526 bytes to /tmp/public (details mode)
```

```
<!-- tasks/9f31.html, trimmed: every link is relative -->
<nav><a href="../index.html">Index</a><a href="../tableau.html">Global tableau</a><a href="../gate.html">Gate definition</a></nav>
<h1>9f31 Sensor board</h1>
<dt>Status</dt><dd>function stalled (blocked): Barometer ICs on 14-week backorder, 2026-09-28</dd>
<dt>Parent</dt><dd><a href="4e2b.html">4e2b</a> Sensor node</dd>
<details class="detail"><summary>Detail</summary> ... </details>
<details class="provenance"><summary>Provenance</summary> ... </details>
```

## The questions and the answers

### 1. What does "every view at every disclosure level" mean without a server?

The levels are glance, detail and provenance, each including the one before. The tableau adds a second dimension: each parent expands or collapses, top-down. I built all three.

| | (a) `details` | (b) `fragments` | (c) `states` |
|---|---|---|---|
| Mechanism | every level in the page behind native `<details>` | glance in the page; detail, provenance and children in files that HTMX fetches by relative `hx-get` | one page per state, links between states |
| Script | none | HTMX (51 KB) | none |
| `file://` | works | needs a web server (see below) | works |
| weather-station files | 17 | 55 | 851 |
| weather-station bytes | 45,526 (7,161 gzip) | 106,841 (28,380 gzip), of which 51,649 are HTMX and its licence | 2,733,109 (110,528 gzip) |
| Pages for the tableau | 1 | 1, plus 14 fragments | 813 |

How I counted the states. A node on display holds a level (3 choices) and, if it is a parent, shows or hides its children. For a leaf the count is 3; for a parent it is 3 x (1 + the product over its children), so the tableau of `weather-station` (a1c0 over 4e2b with 9f31 and c07d, then 7b2e and 3c5d) has 813 states with levels and 3 with expansion alone. `TestStateCounts` enumerates all 813, writes each as a file, and checks that every link between states lands in the set. A task, the gate and an assignment have 3 states each.

For a project of 200 tasks, with up to five children per node: the expansion states alone number 5,323,599,370 (`--estimate 200`), and with a level per node about 3 x 10^95. At three children per node the expansion states alone are about 2 x 10^14. Mode (c) is therefore out for the tableau; it works only per task (600 files, about 1.8 MB for 200 tasks). The estimates for the other two scale the measured bytes per task (3.7 KB a page in (a), 650 B a tableau row):

- (a) about 225 files and 0.95 MB (a few hundred KB gzip).
- (b) about 1,050 files and 1.1 MB, 51 KB of it HTMX; each click costs a request and a few hundred bytes to 2 KB.
- (c) tasks, gate and people alone: about 650 files and 1.9 MB; the tableau cannot be written.

`file://`. (a) and (c) load no script and fetch nothing: a browser follows a link to a file, so they work from `file://`. I checked by resolving every link against a `file:` path and testing that the file exists (`walk` in the tests), and not in a browser. (b) fetches with `XMLHttpRequest`, and browsers treat `file:` pages as opaque origins that may not fetch; I expect it to fail from `file://` and to work from any web server, including a project site. I could not run a browser here (none is installed), so that stays unverified.

Recommendation: (a), native `<details>`. It needs no script, no server and no state, keeps one URL per page, and at 200 tasks it stays under 1 MB. (b) saves nothing at this size, because the fragments sum to the same bytes as the details and add 51 KB of script and a server requirement; keep it only if a project grows so large that one tableau page is too heavy, and then fragment the tableau alone. (c) fails on the tableau's state space. The loss in (a): the URL names a page, not an expansion, so a viewer cannot share "the tableau with 4e2b open". The daemon still can, with its query. The page default opens the root only.

### 2. Is the bundle self-contained and relocatable?

Yes. Every link is a relative file path computed from the page that holds it (`relPath` in `links.go`), so no `--base` is needed, and no `<base>`, script CDN, font, image or stylesheet link exists. The tests:

- `TestLinkWalk` walks every file from `index.html`, resolves every `href`, `src` and `hx-get`, and fails on an absolute or external address, a `..` that leaves the bundle, or a target the bundle lacks. It also fails on a file that no link reaches. It runs in all three modes. (The standard `url` resolver clamps `..` at the root and so hides an escape; the test resolves paths itself.)
- `TestRelocatable` serves the written bundle under `/`, `/repo/` and `/a/b%20c/d/`, crawls it with real HTTP requests that resolve links as a browser does, and fails if any request leaves the prefix, returns anything but 200, or leaves a file unreached.
- `TestScripts` finds no `<script` in modes (a) and (c), and exactly one per page in (b).

Two points for the design. A project site served as `/repo` without the trailing slash resolves `index.html`'s links against the parent; GitHub Pages and most hosts redirect to `/repo/`, and a link back to `index.html` stays correct from every page. Second, a fragment's links must resolve against the page that fetches it, not against the fragment's own file, because HTMX inserts the HTML into that page. The export therefore renders a fragment's links relative to its host page (`hostPage`), and a fragment can have one host only. The layout works because each fragment has a single host.

### 3. Can the same templates serve the daemon and the export?

Yes. `NewDaemon` (`daemon.go`) is a 40-line handler with routes `/`, `/tableau?s=...`, `/task?id=...&level=...`, `/gate`, `/assignment?as=...`, `/queue?as=...`, `/fragment?...` and `/static/`. It calls the same `App.Render` as the export and passes `DaemonLinker`, where the export passes `ExportLinker`. `TestDaemonAgainstExport` fetches every page and fragment from the daemon, reads the matching file in the bundle, replaces the value of every `href`, `src` and `hx-get` with a placeholder, and requires equal text. All 16 pages of (a), all 52 pages and fragments of (b) and all 850 pages of (c) pass, and the tests check that the raw outputs do differ, so the comparison has teeth. `TestDaemonLinksResolve` crawls the daemon; every link answers 200 (the daemon does not serve `manifest.json`).

What the templates must not do for this to hold:

- Write or assemble an address: no literal `/`, `http` or `../`, no concatenation. Ask `$.Task id`, `$.Person view email`, `$.Static name`; the Linker answers. (`TestTemplatesStayNeutral` scans for the literal forms.)
- Branch on the medium or the request: no `HX-Request`, no query value, no "export" flag. The mode (details, fragments, states) is data that both media share.
- Read the clock or anything outside the data (`now`, random, environment).
- Rely on an address's shape: a fragment's links resolve against its host page, and a state link is an opaque string the Linker returns.
- Include a script of its own, apart from HTMX.
- Print a reference's `url` as a link: the data holds repository paths (`docs/sensor-board.md`) that name no file in the bundle, so the templates print them as text.

`internal/web/templates/base.html` fails the first rule: it holds `<script src="/static/htmx/htmx.min.js">`, an absolute path that breaks under `/repo/` and under `file://`. The prototype uses its own layout (`templates/layout.html`) with `{{.Static ...}}`. The product's base needs the same change.

### 4. Is the export deterministic and complete?

- Deterministic: `TestDeterministic` exports twice per mode into two directories and compares every byte. The code sorts every list, ranges only over maps (which `html/template` sorts), and writes no timestamp; the only dates are the data's.
- Complete: `index.html` lists every page, grouped (`TestIndexLinksEveryPage`), and the walk reaches every file, fragments and assets included. `manifest.json` lists every file but itself with its size and SHA-256, sorted (`TestManifest`).
- Failure: the export renders the whole bundle in memory and writes it to a hidden staging directory beside the output, then renames it into place. A non-empty output directory fails with exit 1 and `output directory X is not empty`, and the directory stays as it was. A malformed or missing task file, a missing gate file and a template error each exit 1 with a message that names the view (`view task definition c07d: ...`) and leave nothing behind, not even the staging directory (`TestFailures`). An existing empty directory is accepted and survives a failure.

Limit: the final step removes an empty output directory and renames the staging directory over its name; two steps, so a crash between them leaves no output directory. Nothing in it would be half-written.

### 5. What does the bundle say about the views that take a person and the viewer's column choice?

The bundle writes, for each person the data names, the assignment page (glance, detail and provenance) and the work queue where tablo gives one. For `weather-station` that is four assignment pages and three queue pages; `opus@example.org` has no queue in the data and so gets none. Pages of one kind refer to the others by link: a person's page links the tasks.

What stays out for lack of a server:

- The contextual tableau in its person form (`as=ben@example.org`) and the contributor's default expansion (parents of their tasks, in a gate window around their next gates). The data exists (`886d/output/contextual-person-ben.json`) but the export would need one page per person per state; as an option the export could write the default state of each person (a handful of pages in details mode), but I did not build it.
- The viewer's remembered column choice. The tableau data arrives with the default window already applied: columns outside the window come as a fold count (`folded`, with `by_gate`) and nothing per task, so even a script could not unhide them. A static page shows the data's window and the fold count only. Remembering a choice needs a script and `localStorage`, which the export avoids. To offer a choice, tablo would have to return all columns (a `window=all` request) and the page would hide the extra ones with CSS; the export would then carry every column in each row.
- The `as` identity of the viewer as a parameter, and anything that depends on `--at` other than the one ref the data was read at.

## Findings for the design gate

1. `internal/web/templates/base.html` hard-codes an absolute script path. The product needs a `{{.Static}}`-style link function in the base (question 3).
2. A fragment is bound to one host page: HTMX resolves its relative links against the host. A flat layout (every page in one directory) would remove the constraint at the price of long file names; the nested layout with one host per fragment works now.
3. Without HTMX a fragment button does nothing. A plain `href` fallback would open a bare fragment whose links resolve against the wrong directory, so the prototype leaves it out.
4. Task `references` hold repository paths (`docs/sensor-board.md#power-budget`) and sometimes absolute URLs. Neither names a file in the bundle. The design must decide whether a project may configure a repository base URL for the export to link to; the default is text.
5. The tableau data folds columns server-side with no all-columns form, so the export cannot offer a viewer's column choice (question 5).
6. tablo has no enumeration of views to export: the prototype takes the task ids from the tableau rows and the people from the file names. The product needs a list of people (authorities, assignees, contributors, reviewers, models) from tablo.
7. The default mode (a) shares no URL for an expansion. VIEWS.md says every state of expansion links; the static export honours that for pages and levels of single tasks but not for the tableau tree. The design must accept this or choose an upper bound on the tableau's states.
8. `provenance` for a rolled-up tableau row has a `rolled_up_from` but no recorder or commit, and the three people views keep their levels in three shapes (`glance.people`, `detail.people`, `provenance.people`); the templates read them separately.

## What I verified and what I did not

Verified by `go test ./...`, `go vet`, `gofmt` and `golangci-lint`: everything above that names a test. Sizes come from `TestSizes` and from `tar | gzip -9` on the written bundles. The 200-task numbers are estimates: counts come from the same code on a generated tree (`--estimate`), bytes from linear scaling of the measured pages.

Not verified: any behaviour in a browser (none is installed), HTMX swapping a fragment, how `file://` treats `fetch` or XHR, a screen reader, a phone, a real project site under a path prefix beyond the `httptest` crawl, and a project of 200 tasks.
