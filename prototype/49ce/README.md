# 49ce: the server

A prototype of `tableaud serve`: one route per view, query parameters for the person, the ref and the focus, a watcher that invalidates on HEAD or `.tableaux` changes, and a loopback binding. The pages show the view data as preformatted text. The layout is provisional: no mockup exists yet, and this prototype settles function only.

## Questions and answers

1. **Can the standard library notice a change of HEAD and of `.tableaux/`, cheaply and promptly?** Yes.
   - `watch.go` reads `.git/HEAD`, follows the symbolic ref to the loose ref file or, failing that, to `packed-refs` (parsed once per size and modification time), and fingerprints the names, sizes, modes and modification times under `.tableaux/`. It starts no `git` process and no goroutine.
   - A linked worktree works: `.git` is a file `gitdir: PATH`, `HEAD` sits in that directory, and its `commondir` file names the directory with `refs/` and `packed-refs`.
   - Tests cover a commit, a branch checkout and its return, a detached checkout, `update-ref`, `pack-refs --all --prune` (the state does not change, since the commit does not), a commit and an `update-ref` over a packed ref, a linked worktree (its own commits, the main checkout's commits, packing from the main checkout, `.tableaux` edits in the worktree), an edit, an addition and a removal under `.tableaux/`, and an unrelated file, an untracked directory named `.tableauxish` and an index rewrite, none of which change the state. Each test compares the watcher's commit with `git rev-parse HEAD`.
   - Cost: one read of the weather-station corpus repository (11 files under `.tableaux/`) takes **62 microseconds** on average over 2000 reads and 1.9 ms at worst (`tableaud --measure 2000 --repo .../weather-station`, warm cache, WSL2). `git rev-parse HEAD` on the same repository takes 2 ms. A 20-task temporary repository benchmarks at 65 microseconds (`go test -bench .`).
   - Delay: the watcher is a function the server asks, rereading only when its last read is older than 100 ms. Against a running server a `touch` under `.tableaux/` showed in the next request after 69 ms, a loop of `curl` processes included. The delay a person sees is therefore the page's poll interval (`--poll`, default 2 s) plus at most 100 ms.
   - Limits: the fingerprint reads no content, so an edit that keeps the size and the modification time to the clock's resolution passes unnoticed (nanoseconds on ext4, coarser on some file systems). The code reads no reftable repositories (`extensions.refStorage = reftable`), which Git 2.45 introduced. An idle server costs nothing.

2. **How does an open page learn of the invalidation?** By polling the view route itself with `If-None-Match`, which needs HTMX and no script.
   - Every non-pinned page holds `<main id="view" hx-get="SAME URL" hx-trigger="every 2s" hx-headers='{"If-None-Match":"TAG"}' hx-swap="outerHTML">`. The server answers `304` while the tag stands, and htmx swaps nothing. When the state changes it answers `200` with the new fragment, which carries the new tag and the same attributes, so the poll continues. A pinned page has no poll element.
   - The route answers conditional requests correctly: the tag is a function of view, canonical query, cache key and variant (page or fragment, with `Vary: HX-Request`), so a tag of one variant never stands for the other; `If-None-Match` takes a list, `W/` marks and `*`; parameters validate before the conditional check, so a `304` never hides a `400`; the `304` repeats `ETag` and `Cache-Control`. The tag derives from the cache key and not from the rendered body, so a poll costs a watcher read and no tablo run.
   - htmx 2.0.6 swaps a `304` response by default (its `responseHandling` maps `[23]..` to a swap), which would blank the view with an empty body. The page therefore carries `<meta name="htmx-config">` that maps `304` to no swap. The vendored htmx accepts this configuration; see Findings.
   - Rejected:
     - **Server-sent events.** They need htmx's SSE extension (a second script, not vendored), a long-lived connection per page and a goroutine that watches the tree continuously. They would deliver the change in milliseconds, which a person reading a tableau does not need.
     - **A separate version endpoint.** A poll of `/version` that returns a number cannot swap anything: the page then has to fetch the view in a second request, chained through `hx-trigger` on the version fragment, or reload with `HX-Refresh`, which discards the viewer's disclosure state. Polling the view route with a conditional request does both in one round trip.
     - **Plain browser revalidation** (`Cache-Control: no-cache` plus `ETag`, polling without `If-None-Match`). The browser turns the `304` into a `200` with the cached body, and htmx swaps the same content every 2 s, which resets open `<details>`. The explicit header makes the browser pass the `304` through.

3. **The route scheme.** `GET /gate`, `/task`, `/authority`, `/assignment`, `/queue`, `/blockage`, `/tableau`, `/contextual`, `/history` and `/audit`, the ten views of VIEWS.md in its order, and `GET /` for an index.
   - `as` is the person, `at` the ref (for `/history` a range `FROM..TO`), and the focus parameters are `task`, `window` or `columns`, `historical`, `role`, `level`, `brief` (queue, `TASK:GATE`), `stale` (audit, days, default 7) and `proposed` (authority). `params.go` lists, for each view, the parameters its Parameters paragraph names plus `as`, `at`, `role` and `level`. A known parameter the view does not name has no effect and drops out of the canonical query, so it does not change the cache key; an unknown name is a `400`.
   - Validation: an email for `as`; four hexadecimal digits for `task`; `window` 0 to 50; `columns` gate names, exclusive with `window`; `at` from a character set that excludes every character Git reads in a revision and the leading hyphen (no option injection) and, when it holds `..`, only for `/history`, with exactly one `..`; a parameter given twice; a missing `task` on `/task`; `task` with `as` on `/contextual`. Each failure answers `400` with the reason. A well-formed task, ref or gate that the project lacks answers `404` (a `columns` or `brief` gate that does not exist is a `400`).
   - A normal request returns the full page (document, script tag, fragment). A request with `HX-Request: true` returns the fragment alone.

4. **Where does the data come from?** From `Source` (`source.go`), four methods: `Resolve(ref)`, `Gates(rev)`, `HasTask(rev, id)` and `View(name, params, rev)`. `Rev` is a commit, or the working tree on top of a commit (`Worktree`). The one implementation reads the JSON in `testdata/`, returns the same data for every ref and focus, and takes gate names and task ids from `tableau.json`.
   - Cache key and invalidation:

     | Request | Reads | Key | Cache-Control | Invalidated by |
     |---|---|---|---|---|
     | no `at` | the working tree on HEAD | the watcher's State: HEAD text, HEAD commit, `.tableaux` fingerprint | `no-cache` | a commit, checkout, ref update or `.tableaux` edit |
     | `at=HEAD`, a branch or a tag | a commit | the commit the ref resolves to now | `no-cache` | the ref moving, not a working tree edit |
     | `at=` a hexadecimal commit prefix of seven or more digits (or a range of two) | a commit | the commit | `public, max-age=31536000, immutable` | never; the page has no poll element |

     A test checks that after an edit under `.tableaux/` only the live page's tag changes.
   - **Finding: what tablo needs to give.** The prototypes in `tablo/prototype` read through `git ls-tree` and `git cat-file` at a ref, so they cannot see the working tree. The live view needs either a library import with a working-tree reader (`Rev.Worktree`) or the binary with a `--worktree` flag; the server cannot feed uncommitted `.tableaux` files to a tablo process that reads Git objects. Running the binary costs one process per cache miss (the cache key makes misses rare); a library import gives one process and a shared parse. Either way tablo must also give `Resolve`, `Gates` and `HasTask` cheaply, or the server reads them from the view data it already holds.
   - **Finding: the live view reads the working tree, not HEAD.** VIEWS.md's default is "`HEAD` of the checkout". The watcher notices working tree edits, and a person who edits `.tableaux/` and sees nothing change reads that as a bug. The design should choose; the prototype follows the working tree when `at` is absent and offers `at=HEAD` for the committed state. Authorisation reads the trunk whatever the ref (VIEWS.md); the server leaves that to tablo.

5. **Binding and Host.** The default is `127.0.0.1:8642`, and a bind to another address prints a warning. The server answers every request whose `Host` is `localhost`, a loopback address (`127.0.0.0/8`, `::1`) or a name given with `--allow-host`, with or without a port, and answers every other `Host` (the empty one included) with `403`, on every path including `/static/`. Tests cover 9 accepted and 10 refused forms (`127.0.0.1.evil.example`, `localhost@evil.example`, `0.0.0.0`, `[::]`), and a request over a real loopback socket with a chosen `Host`. `/static/` serves `web.Static` (the vendored htmx) through `http.FileServerFS`. The check stops DNS rebinding for any browser, since it sends the attacker's name; it does not stop a local process, and it is no authentication.

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./...                                   # no network, browser, corpus or port
go run ./prototype/49ce --addr 127.0.0.1:0      # prints the address; serves the repository in .
go run ./prototype/49ce --render '/tableau?as=ben@example.org&window=2'
go run ./prototype/49ce --render '/tableau' --fragment
go run ./prototype/49ce --measure 2000 --repo ../tableaux/corpus/build/weather-station
```

Options: `--addr HOST:PORT`, `--repo DIR`, `--poll DURATION`, `--allow-host NAMES`, `--render URLPATH`, `--fragment`, `--measure N`, `--labels`. A single-hyphen long option exits 2 and names the `--` form. `--labels` resolves refs by the testdata's commit names (`W1` to `W13`) instead of `git rev-parse`; `--render` always does, since the fixtures are not the repository's own data. The optional word `serve` before the options is accepted.

## Test data

`testdata/` holds copies, unedited, of the output of the tablo prototypes on the weather-station corpus at `main` (from the `tablo` directory):

| File | Command |
|---|---|
| `gate.json`, `task.json`, `authority.json`, `assignment.json` | `go run ./prototype/493e -repo <corpus>/weather-station -ref main -view gate\|task\|authority\|assignment -task 9f31` |
| `tableau.json` | `go run ./prototype/886d tableau` |
| `contextual.json` | `go run ./prototype/886d contextual -task 4e2b` |
| `blockage.json` | `go run ./prototype/886d blockage` |
| `queue.json` | `go run ./prototype/886d queue -person ada@example.org` |
| `history.json` | `go run ./prototype/8ed1 -repo <corpus>/weather-station -view history -range W1..main` |
| `dada.txt` | `go run ./prototype/dada <corpus>/weather-station main` (the audit) |
| `labels.txt` | `weather-station.labels.txt` beside the corpus |

The prototype takes gate names and task ids from `tableau.json` and the ten titles from VIEWS.md; it hard-codes no view fact.

## What it shows

```
$ tableaud --render '/tableau?as=ben@example.org&window=2' --fragment
<!-- ETag: "e68ff46ff9ac7a09"  Cache-Control: no-cache -->
<main id="view" hx-get="/tableau?as=ben%40example.org&amp;window=2" hx-trigger="every 2s" hx-headers="{&#34;If-None-Match&#34;:&#34;\&#34;e68ff46ff9ac7a09\&#34;&#34;}" hx-swap="outerHTML">
<h1>Global tableau</h1>

$ tableaud --render '/tableau?at=68d9021206ec627755e2ead99b6be3f2c06ae815'
<!-- ETag: "acab0e721be848dd"  Cache-Control: public, max-age=31536000, immutable -->
...                                      (no hx-get: a pinned page never polls)

$ tableaud --render '/task?task=ffff'
tableaud: 404 task ffff: not found
$ tableaud --render '/tableau?window=x'
tableaud: 400 window: "x" is not a whole number from 0 to 50
$ tableaud -addr x
tableaud: use --addr, not -addr: a long option takes two hyphens          (status 2)
```

Run against a live server on a free port with `curl`: `200` with an `ETag` and `Cache-Control: no-cache`; the same request with `If-None-Match` and `HX-Request` gives `304`; `Host: evil.example` gives `403`; `/static/htmx/htmx.min.js` gives `200`; a `touch` under `.tableaux/` turns the `304` into `200` within 69 ms.

## Verified and not verified

Verified: the Go tests (handlers through `httptest`, one real loopback socket), the `curl` run above, the watcher against repositories that `git` built (Git 2.47.3), including a linked worktree. Not verified: any browser. No `chromium`, `google-chrome` or `firefox` is installed, so no run shows htmx polling, outerHTML swapping of the poll element, the `304` configuration taking effect, or that a browser passes a `304` through to htmx when the script sets `If-None-Match` itself. The htmx claims come from reading its 2.0.6 source (the default `responseHandling`) and its documented behavior.

## Stand-ins and omissions

- Every route returns the same fixture whatever the ref and focus. The parameters are validated and keyed, not applied.
- The pages show data as `<pre>`. No view templates, disclosure state, styling or export.
- `role` and `level` are validated and keyed and have no effect. The fixture holds no per-role data.
- The page has no link back to the same query on another view; the index links the views with no parameters.
- No reftable support in the watcher; submodule `.tableaux` directories are not watched.

## Findings for the design gate

- **tablo has to read the working tree** for a live view, or the design drops the live view and follows HEAD only (see question 4).
- **`as` and `person`.** VIEWS.md has a viewer (the person who runs the tool) and a `person` parameter that focuses a view (a dispatcher names an agent). The brief's `as` covers one of them. The server cannot know the viewer from a request, so either `as` is the viewer, defaulting to the Git identity of the checkout, and a `person` parameter focuses, or `as` does both. The prototype implements `as` alone.
- **base.html has no head block.** The page needs a `<meta name="htmx-config">` in the head so that htmx ignores a `304`; this prototype therefore uses its own layout. The product's base should offer a head block.
- **The poll interval is a choice** (2 s here): the cost per poll is 60 microseconds of server work and a `304` with no body, so 1 s is affordable, and the interval is the delay a person sees.
- **A changed page replaces `#view`.** Any client state inside it (open `<details>`) resets on a change unless the disclosure task keeps it outside the swapped region, in the URL (`level`) or in an idiomorph-style swap. This prototype does not decide it.
- **An unknown ref is a `404` and a malformed one a `400`**; a `columns` gate the project lacks is a `400`. The design can fold these.
- **Tags derive from the key, not the body.** Two states with the same key but different data cannot occur if tablo is a pure function of the files and the commit; a tablo that reads the clock (the audit's stale age) breaks it, and the audit's key would then need the day.
