# Design 438a: the reconciliation with the server as built

**Proposed on 2026-10-07, awaiting the owner.** Designs [`438a`](../438a.md) and [`49ce`](../49ce.md) each declare the page frame of package `internal/web`; the owner accepts both, and `49ce` stands implemented on `main`. This document maps the five designs that wait, `438a`, [`e2b6`](../e2b6.md), [`44bf`](../44bf.md), [`9a9c`](../9a9c.md) and [`6160`](../6160.md), onto that code. **The rule of precedence:** the owner's rulings come first; then the code on `main`, by its real names and signatures (`49ce` decision 14); then the bodies of the five designs. Where a design's body and the server disagree, the server's frame, names and addresses stand and the design keeps what lives inside `<main>`: its models, its templates' markup, its partials, its folds, its sheet and its tests. Where the server cannot carry something a design needs, section 2 lists the smallest addition, by signature, with the task that lands it. Section 9 lists the questions that are the owner's; until the owner rules, nothing here binds.

One sentence carries the hard row, the data: a view's model reaches its template through an adapter that `web.Render` calls, a view with no adapter draws the placeholder it draws today, and so each template task lands its templates on fixtures first and turns its routes over when tablo's types exist, as `49ce` lands its steps 1 to 4 before its step 5.

## 1. What stands, what moves, what does not land

### The page and the frame

| `438a` declares | Outcome |
|---|---|
| `Page` with `Heading`, `Question`, `Project`, `Ref`, `Viewer`, `Role`, `Params`, `Nav`, `Legend`, `CSS`, `Script`, `Meta`, `Live`, `Body` | Does not land. `web.Page` stands as built and gains one field, `Body any`, the view's model |
| `Ref`, `NavItem`, `Live` | Do not land: `source.Project` (`Ref`, `Short`, `Date`, `Worktree`), `web.NavItem` and `web.Live` stand |
| `Page.Heading`, `Page.Question` | `Page.View.Title`, and `Page.View.Question`, a field `web.View` gains |
| `Page.Params []Param` | `Param` lands as declared; `(*Page).InForce() []Param` builds the list from the request |
| `Page.Legend`, the address of the gate definition | `{{$.To "gates"}}`; `Page.Legend` stays tablo's gate definition |
| `Page.CSS`, `Page.Script`, `Page.Meta` | Do not land: the frame asks `Link.Static` and writes its own `meta` |
| `Meta` | Lands as declared, in `model.go`: `44bf`'s `ColumnsForm.Hidden` uses it |
| `base.html` with `page` and `main` | Does not land. `templates/base.html` stands: `document`, `page`, `viewer`, `error`. The frame owns the document, `<div id="page">` with the poll, the nav, the context line, `<main>` and the footer |
| The footer that names the view and links the gate definition | Does not land: the frame's footer stands. The nav and each tableau's key line link the gate definition |
| The context line's "no viewer, observer" | The frame's "viewer: an observer" stands |
| `<main id="main" class="v-<view>">` | `<main id="main" class="v-NAME">`, `NAME` the route's name: one token of `base.html` changes (question 1). An error page keeps `class="error"` |
| The poll's attributes on `main` | Stand on `<div id="page">`, as built |

### Rendering

| `438a` declares | Outcome |
|---|---|
| `Renderer`, `New`, `Page`, `Main`, `Part`, `Views []View` | Do not land. `web.Render(w, Shape, *Page)` and `web.Views` stand |
| `Renderer.Main`, `main` alone as the fragment | Does not land. The fragment is `<div id="page">` (`web.Fragment`) |
| `Renderer.Part(w, view, part, data)` | `web.Render(w, web.Part, p)`: with a model, `part/NAME` receives the value of the model's `Part(name, key)`; with none, it receives the `Page`, as the server's probe part does |
| `newFrom(fs.FS)` | `parse(fs.FS)`, unexported, which `frames` calls with `web.Source` |
| `funcs` with `add`, `plural`, `depth` | Three entries in the literal `web.Funcs`, beside `json` |
| `View` as a string, the ten `View…` constants | Do not land. `web.View`, the struct, stands; a view's name is its route's |
| `Level`, `Glance`, `Detail`, `Provenance` | Land as declared, in `env.go` |

### Addresses

| `438a` declares | Outcome |
|---|---|
| `URLs` with `Page(Target)`, `Part(Target)`, `Static` | Does not land. `web.Linker` stands, and an adapter asks the `Page`: `To`, `Self`, `Anchor`, `Reference` |
| `Target` | Does not land. Each field is a pair of `To` or `Self`, as the table below states |
| `Link{Href, Swap}` | Lands as `Control{Href, Swap}`: `web.Link` is the server's. The fields keep their names, so the fixtures stand |
| The partial `swap`: `hx-get`, `hx-target="#main"`, `hx-swap`, `hx-push-url` | Writes ` hx-boost="true"` and nothing else: the frame's `<div id="page">` carries the target and the swap (`49ce`, Controls) |
| `Env.URLs`, `Env.View` | `Env.Page *Page` replaces both |

| `Target` field | An adapter writes |
|---|---|
| `View`, `Task`, `Person` | `e.Page.To(view, "task", id)`, `e.Page.To(view, "person", email)` |
| `Project` | `e.Page.To("task", "project", url, "task", id)` |
| `Range` | the pair `"ref", "FROM..TO"` |
| `Level` | `e.Page.Self("level", level)` |
| `Open` | `e.Page.Self("open", strings.Join(ids, ","))` |
| `Proposed` | `e.Page.Self("proposed", "on")`, and `e.Page.Self("proposed", "")` for the unfiltered page |
| `Part`, `Key` | `e.Page.Self("part", "NAME")` or `e.Page.Self("part", "NAME:KEY")`; `Env.Fold` alone writes it |
| `Whole` | Nothing: the document of a part address is the whole page (section 4) |
| `Anchor` | `e.Page.Anchor(kind, key)` for a legend entry; for any other element the address, then `"#"`, then the id |

`To` and `Self` return the empty string where the medium holds no page: a further project (`49ce` decision 11), an unknown task in the export (`6160` rule 1). So the partial `task` writes `<code class="id">ID</code>` in place of the link when `TaskRef.Href` is empty. The six places that link a task outside that partial write the link's text without the `a` element in the same case, an id as `code.id`: the row id of `grid.html`, the three id cells of `assignment.html`, "The task definition of …" in `authority.html` and "The task view shows …" in `contextual.html`.

### Templates, files and names

| `438a` and its siblings write | Outcome |
|---|---|
| `templates/<view>.html`, named `gate` and `contextual` | `templates/views/NAME.html`, named by the route: `gates.html`, `context.html`; each replaces its placeholder |
| `templates/partials.html`, `templates/grid.html` | `templates/shared/partials.html` (`438a`), `templates/shared/grid.html` (`44bf`). `Render` globs `shared/`, so `438a` lands no empty `grid.html` |
| A view file defines `body` | It keeps `body` and gains two lines at its top: `{{define "view"}}{{template "main" .}}{{end}}` and `{{define "context"}}{{template "params" .}}{{end}}` |
| `part-<name>` | `part/<name>`, in each definition and each call |
| `$.Ref.Commit`, `$.Ref.Date`, `$.Legend` | `$.Project.Short`, `$.Project.Date`, `{{$.To "gates"}}` |
| The model types and the Go files `view_gate.go`, `view_structural.go`, `adapt_gate.go`, … | Stand as named: they name the view of VIEWS.md, not the route. `438a`'s `page.go` lands as `model.go`, since `page.go` is the server's |
| The fixtures `gate.json`, `contextual.json`, … | Keep their file names. Each lands as an object of two keys: `View`, the route's name, and `Body`, the draft's `Body` unchanged |
| The golden pages | Each lands as the `<main>` element of the rendered document and a newline (section 5, T2) |
| `testdata/shared.css` | Does not land: the server's `TestStyleSheetStartsWithTheSharedBlock` holds the block by its SHA-256 |
| `dom_test.go`, `check_test.go` | Land as designed, in package `web_test` |

`shared/partials.html` is the draft [`partials.html`](partials.html) with three changes, to `task` and `swap` as stated above and to `references` as stated below, and with two templates more:

```
{{define "main"}}{{if .Raw}}{{.Raw}}{{else if .Body}}
<h1>{{.View.Title}}</h1>
<p class="question">{{.View.Question}}</p>
{{template "body" .}}{{else}}<h1>{{.View.Title}}</h1>
<pre>{{json .Data}}</pre>
{{end}}{{end}}

{{define "params"}}{{range .InForce}} · {{.Name}} {{if .Task}}{{template "task" .Task}}{{else}}{{.Value}}{{end}}{{end}}{{end}}
```

`main` is the content of the frame's `<main>`: the model under the view's title and question, or the placeholder, byte for byte what the ten files on `main` draw. The `Raw` branch is `6160`'s and lands with it.

**References.** `438a` prints a reference's `url` as text; `49ce` decision 9 links an absolute `http` or `https` address and binds the templates. `Reference` gains `Href string`, which the adapter fills from `e.Page.Reference(url)`, and the partial `references` and the task rows of `gates.html` write `<a href="{{.Href}}"><code>{{.URL}}</code></a>` when `Href` is not empty. A fixture has no `Href`, so no golden page changes.

### The sheet

`static/tableaud.css` lands whole with `438a`, from the draft [`tableaud.css`](tableaud.css) (question 2), with `.v-gate` written `.v-gates`, `.v-contextual` written `.v-context`, and the section comments `/* gate */` and `/* contextual */` written `/* gates */` and `/* context */`. Every rule of a view's section starts with `.v-NAME`. `49ce` A9's "appends its rules under `main.NAME`" becomes: `438a` lands every view's rules under `.v-NAME`, and the sibling tasks add none.

### The data

| `438a` declares | Outcome |
|---|---|
| `func <View>From(data <tablo's type>, e Env) *<View>View` | Stands, with `Env` as section 3 declares it |
| The handler calls the adapter (A1, A3) | `web.Render` calls it, through `adapters`; the handler in `serve` changes by nothing |
| `NewLegend`, `Legend` and its methods, `Opener`, `ByLevel`, `FoldRuns`, `Run`, `Sym`, `Fold`, `TaskRef`, `Person`, `Commit`, `Status`, `Snapshot`, `Reference` | Land as declared; `Opener.Opens(view View, class, id string)` takes the server's `View` |
| `Env.Fold(class, id, part, key)` | Lands with the body of section 3 |
| Each model's `Part(name, key string) (any, bool)` | Stands; the interface has a name, `Parter` |

## 2. What the server gains

| Package | Addition | Lands with |
|---|---|---|
| `web` | `View.Question string`, and the ten questions of VIEWS.md in `Views` | `438a` |
| `web` | `Params.Key string`; `Params.set("part", "NAME:KEY")` cuts at the first colon | `438a` |
| `web` | `Page.Body any` | `438a` |
| `web` | `func (p *Page) InForce() []Param` | `438a` |
| `web` | `type Parter interface { Part(name, key string) (any, bool) }` | `438a` |
| `web` | `var ErrNoPart = errors.New("no such part")` | `438a` |
| `web` | `var adapters = map[string]func(e Env) any{}`, `levelOf`, `legendOf`, `envOf`, `openAll`, `parse`, all unexported | `438a` |
| `web` | `Funcs["add"]`, `Funcs["plural"]`, `Funcs["depth"]` | `438a` |
| `web` | `base.html`: the class of `<main>` is `v-NAME`; `TestFrame` expects it | `438a` |
| `serve` | `ParseQuery` reads `part=NAME` or `part=NAME:KEY`: `NAME` is in the view's `Parts`, `KEY` matches `^[a-z0-9._-]{1,128}$`, anything else answers `400`. `Query` writes `part=NAME:KEY` | `438a` |
| `serve` | `live` clears `Key` with `Part`; `respond` answers a `Render` error that is `web.ErrNoPart` with the `404` page and logs nothing | `438a` |
| `web` | `Views[i].Parts`, per view, in the commit that registers the view's adapter | each template task |
| `serve`, `source` | `source.Request.Proposed`, its place in `Request.Key` and its assignment in `serve.request` go: the adapter filters (question 6) | `e2b6` |
| `web` | `Page.Raw template.HTML`, and the `Raw` branch of `main` | `6160` |
| `web` | `base.html`: `hx-target` and `hx-swap` of `<div id="page">` stand inside `{{if .Scripts}}`; `TestFrameWithoutScripts` refuses `hx-` | `6160` |
| `source` | `Index(ctx context.Context, project, ref string) (Index, error)`, a method of `Source`; `Index` and `IndexTask` | `6160` |
| `sourcetest` | `(*Fixture).Index`, over a new `testdata/index.json` | `6160` |
| `cmd/tableaud` | `case "export"` in `run`, and `export.go` with `runExport` | `6160` |

Beyond these rows `serve` stands untouched: its routes, the tag, the cache, the watcher, the `Linker`, `cases.json` and every test of `serve` and `cmd/tableaud`. `query_test.go` gains the key's rows (section 5).

## 3. The seam: Render, the adapter, the placeholder

```go
// Env is what an adapter needs beyond the data of its view. Render builds it.
type Env struct {
	Page   *Page   // the request; every address comes from its methods
	Legend *Legend
	Person string // the person parameter; "" marks nobody
	Open   Opener
	Inline bool // every fold holds its body: the export, and the document of a part address
}

// adapters holds, by route name, the function that builds a view's model
// from Page.Data. It returns nil when the data is not tablo's type for the
// view. A view with no entry, or with a nil model, draws its placeholder.
var adapters = map[string]func(e Env) any{}

// levelOf returns the level a page opens at: the level the address states,
// else glance for the export and for an observer, provenance for the owner on
// the history and the audit, and detail for every other role (VIEWS.md,
// Levels). The role is the one the address states, else the viewer's first.
func levelOf(p *Page) Level

// legendOf builds the Legend from Page.Legend, tablo's gate definition. Until
// the gate adapter lands it knows no type and returns NewLegend(nil, nil, nil, nil).
func legendOf(any) *Legend

// envOf returns the Env of one rendering of p in a shape: Legend from
// legendOf, Person from Params.Person, Open as ByLevel(levelOf(p)), or
// openAll{} for a Part; Inline when p.Scripts is false, or when the shape is
// Document and Params.Part is set.
func envOf(p *Page, shape Shape) Env
```

`Render` gains two steps, in this order, before it picks the template:

1. When `p.Body` is nil, `p.Raw` is empty, `p.Err` is nil and `adapters` holds the view, it copies the page, sets the copy's `Body` to the adapter's answer for `envOf(&copy, shape)`, and renders the copy. It never writes to the caller's page.
2. For `Part` with no error: when `Body` is not nil, the data of `part/NAME` is the value of `Body.(Parter).Part(p.Params.Part, p.Params.Key)`, and a model that is no `Parter` or answers false yields an error that wraps `ErrNoPart`. When `Body` is nil, the data is the page, as today.

An adapter file registers its view in an `init`, which `web.Funcs`' own comment allows:

```go
func init() {
	adapters["gates"] = func(e Env) any {
		d, ok := e.Page.Data.(<tablo's type>)
		if !ok {
			return nil // an untyped nil: the fixture's JSON draws the placeholder
		}
		return GateFrom(d, e)
	}
}
```

**What a route serves, and when.** A template task lands in two parts. The first holds everything that needs no type of tablo: the models, the view files, the fixtures and their tests. It registers no adapter and sets no `Parts`, so every route answers the placeholder it answers on `main`, and every test of `serve` passes unchanged. The second lands when tablo's view types exist: the `adapt_*.go` files, the `init` above, the view's `Parts`, and the tests over the corpus. From then a route draws its view from tablo's data. The server's tests still serve `sourcetest`, whose data is decoded JSON and no type of tablo; the adapter answers nil for it, so they draw the placeholder still and pass unchanged. The placeholder branch of `main` and the `json` function go in the commit that makes `sourcetest` answer in tablo's types, which is `49ce`'s step 5 and no task of this document. No throwaway adapter over the prototypes' JSON lands at any point (question 3).

An adapter reads the request from `e.Page`: `Params.Open` and `Params.OpenSet`, `Params.Proposed`, `Params.Brief`, `Params.Person`, `Viewer`. `e.Page.Scripts` false means the export: a `Control` then takes `Swap` false, `Grid.Control` stays nil and `44bf`'s `Unfolded.Branches` is on.

## 4. Folds, parts and keys

```go
// Fold returns the fold of one details element. part names the lazy part
// that serves its body, or is "" for a fold whose body always stands in the
// page. The fold the address names arrives open. A fold with a part is lazy
// when it arrives closed and Inline is off.
func (e Env) Fold(class, id, part, key string) Fold {
	f := Fold{ID: id, Class: class, Open: e.Open.Opens(e.Page.View, class, id)}
	if part == "" {
		return f
	}
	if e.Page.Params.Part == part && e.Page.Params.Key == key {
		f.Open = true
	}
	if !f.Open && !e.Inline {
		if key != "" {
			part += ":" + key
		}
		f.Src = e.Page.Self("part", part)
		f.Alt = f.Src + "#" + id
	}
	return f
}
```

- **The key travels in `part`.** `part=edges:requires`, `part=node:9f31`, `part=day:2026-10-03`, after the form of `brief=TASK:GATE`. The scheme keeps its fourteen names, and a key with no part cannot occur. Every key of the four designs fits the pattern: a task id, a date, `requires`, `dependents`, and the anchors `cause-N`, `next-ID`, `KIND-ID`, `p-…-c-GATE`, `e-HASH-ID`, `h4-2` (question 4).
- **`Whole` needs no parameter.** `49ce` states that a part address without `HX-Request` answers the whole document with that part drawn open. `envOf` turns `Inline` on for that document, so every fold holds its body, as `Target.Whole` asks, and `Fold` opens the one the address names. The fallback link of a lazy fold is therefore the address `hx-get` asks, with the fold's id as fragment.
- **An unknown part answers `400`, an unknown key `404`.** The parser knows the names; the model knows the keys.
- **The opener.** `Opener` and `ByLevel` land as designed. `envOf` is the one place that chooses an opener, and task `db74` changes that function.
- **The legend as symbols.** `source.Result.Legend` carries the gate definition of the same load, which confirms `438a` A5. `legendOf` turns it into the `*Legend`; its body lands with `adapt_gate.go`.
- **The lazy fold and the frame.** A lazy `details` sets its own `hx-target` and `hx-swap`, so it does not inherit the frame's; `tableaud.js` keeps open folds across a swap of `#page` by their ids, which confirms `438a` A4.

## 5. Each task, as it now reads

### 438a, Legend and task templates

**Order.** (1) The additions of section 2 that `438a` lands, `model.go`, `env.go`, and their tests: T6, T11, the failing case of T1, the key's rows in `serve/query_test.go`. (2) `shared/partials.html`, the sheet, `views/gates.html` and `views/task.html`, `view_gate.go`, `view_task.go` with `(*TaskView).Part`, the five fixtures, `dom_test.go`, `check_test.go`: T1 to T14 pass, and `prototype/438a/` leaves the tree. (3) When tablo's `493e` is implemented: `adapt_gate.go` with `legendOf`'s body, `adapt_task.go`, the two entries of `adapters`, the `Parts` of `task` (`status`, `authorisation`, `edges`, `sources`, `reviews`, `models`, `events`), and T15. The task's status note says that step 3 waits, as `49ce`'s says of its step 5.

**Files that change on `main`:** `web/views.go`, `params.go`, `page.go`, `render.go`, `templates/base.html` (one token), `web_test.go` (one line), `doc.go` (a sentence on the models), `static/tableaud.css`, the two view files, `serve/query.go`, `serve/server.go`, `serve/query_test.go`. `438a`'s notes "the new base replaces `templates/base.html`" and "a new test replaces `TestTemplatesParse`" lapse: `49ce` did both.

| Test | As it now reads |
|---|---|
| T1 | `Render` succeeds for every view, and every view's set defines `view` and `context`; `parse` fails, and names the file, on a copy of `web.Templates` that lacks `views/task.html` |
| T2 | Each of the five fixtures decodes `Body` into the model of its `View`, renders as a `Document` in a page of the fixed project (`Tableaux tooling`, `main`, `3cdae52`, `2026-10-05`) with a linker that spells `V/VIEW`, and its `<main>` element equals `testdata/NAME.html` byte for byte. Against the draft's `<main>` each golden differs in two places only: the class `v-gates` for `v-gate`, and the question of VIEWS.md where a draft writes `q` |
| T3 | As designed, on the document, with "no `script` outside `head`" for "at most one `script`": the frame loads HTMX and `tableaud.js` |
| T5 | As designed; the fallback link's address is the `hx-get` address and a fragment |
| T6 | `Env.Fold` with `serve.Linker` on `/task?task=9f31`: `Src` is `/task?task=9f31&part=edges:requires` and `Alt` the same with `#prov-requires`; open, closed, with and without a part, with and without `Inline`, and the fold the address names arrives open; `ByLevel` as designed; `levelOf` over the level, the role, the export and the owner on the audit |
| T7 | `Render` as a `Part`, with `Params.Part` and `Params.Key` set, is a substring of the document that holds the fold open, with no `details`, `html` or `main`; an unknown key is `ErrNoPart` and writes nothing; `task-lazy` and `TaskView.Part` as designed |
| T8 | For every fixture the `Document` holds the `Fragment` byte for byte, the fragment starts with `<div id="page"`, and its `<main>` starts with `<main id="main" class="v-NAME">` |
| T9 | As designed, over every file under `templates/` but `base.html`, beside the server's `TestTemplatesStayNeutral` |
| T10 | The sections are named by route and each rule starts with `.v-NAME`; the first clause is the server's hash test; the three classes without a rule are `d0`, `note` and `v-tableau`, as designed |
| T14 | As designed, and a `TaskRef` with no `Href` renders as `code.id` |
| T15 | Exists and skips until step 3 |

T4 and T11 to T13 stand as worded. `438a` adds one test: the context line of a task page at `task=9f31&person=ben@example.org` ends "· task `9f31` · person ben@example.org", the id a link.

### e2b6, Structural templates

**Order.** After `438a`'s step 2: `view_structural.go`, `views/authority.html`, `views/assignment.html`, the four fixtures: T1 to T3, T5, T6, T8, T9 pass and `prototype/e2b6/` leaves the tree. When tablo's `493e` is implemented: the two adapters, their `adapters` entries, the `Parts` (`node`, `deciding`, `kids`, `commits`; `group`), the removal of `source.Request.Proposed`, and T4, T7, T10 to T12, which exist and skip until then.

The filter's two links are `Control` values from `Self("proposed", "")` and `Self("proposed", "on")`. A glance row whose person has no section on the page links `To("assignment", "person", EMAIL)`. An email the project does not know answers `200` with the empty form (`49ce`, validation), which settles A2.

| Test | As it now reads |
|---|---|
| T1 | As `438a` T2: the `<main>` element, against the draft with the class and the question changed and `hx-boost="true"` where the draft writes the four swap attributes |
| T5 | Both links carry `href` and `hx-boost="true"`, and no other `hx-` attribute |
| T6 | Through `Render` as a `Part`, with the key in `Params.Key` |

### 44bf, Status templates

**Order.** After `438a`'s step 2: `view_status.go`, `shared/grid.html`, `views/tableau.html`, `views/context.html` (the draft `contextual.html`), `views/blockage.html`, `views/queue.html`, the seven fixtures: T1 to T4, T6 to T12 pass on fixtures and `prototype/44bf/` leaves the tree. When tablo's `886d` is implemented: the three adapters, their entries, the `Parts` (`row` on both tableaux; `facts`; `item`, `brief`), and T13. T5's `DefaultUnfolded` case runs from the start; its adapter cases and the address arithmetic of T6 exist and skip until then.

The server passes no `Unfolded`: the adapter builds it from `e.Page.Params`, `Unfolded{IDs: Open}` when `OpenSet` is on and `DefaultUnfolded(levelOf(e.Page), root)` otherwise, with `Branches` on for the export. `Expand` and `Collapse` are `Control` values from `Self("open", IDS)`. A gate column links `e.Page.Anchor("gate", KEY)`. The `columns` form gains `hx-boost="true"`, as `49ce`'s Controls table states.

| Test | As it now reads |
|---|---|
| T1 | As `438a` T2; the drafts' `V/gate` under the two key lines reads `V/gates`, and the swap attributes read `hx-boost="true"` |
| T6 | An `Expand` link carries `href` and `hx-boost="true"`; with `Branches` it is an anchor with no `hx-` attribute |
| T12 | Through `Render` as a `Part` |

### 9a9c, Temporal templates

**Order.** After `438a`'s step 2: `view_temporal.go`, `views/history.html`, `views/audit.html`, the four fixtures: T1 to T5, T8, T9, T11, T12 pass and `prototype/9a9c/` leaves the tree. With it goes the old layout at the top of `base.html`, since `438a`, `44bf` and `9a9c` hold the last prototypes that execute it. When tablo's `8ed1` and `dada` are implemented: the two adapters, their entries, the `Parts` (`day`, `sub`, `commit`; `rule`), and T6, T7, T10, T13, which exist and skip until then.

A range travels as the pair `"ref", "FROM..TO"`. The data cache answers a part from the page's own result (`49ce` T17), so a lazy day costs no call to tablo and A7's second sentence asks nothing of tablo.

| Test | As it now reads |
|---|---|
| T1 | As `438a` T2 |
| T11 | Through `Render` as a `Part`; the key of `day` is the date |

### 6160, Static export

Decision 12 of `6160` already takes the server's names. What remains:

| `6160` declares | Outcome |
|---|---|
| `export.View`, `Target`, `Linker` | Do not land. The unexported `paths` implements `web.Linker` over `web.Link`: rule 1 answers `""`; rules 2 to 6 read `l.View`, `l.Params.Task`, `l.Params.Person` and `l.Params.Project` and nothing else of `Params`; rule 9 is `Reference`, which answers `"", false` |
| Rule 7, the fragment | The linker never sees it: `Page.Anchor` and the adapters append it. T1 appends the fragment column of `paths.tsv` to the linker's answer |
| `Site`, `Rev`, `TaskRef`, `Index` | Do not land. `Run(ctx context.Context, src source.Source, o Options) (Result, error)`; `Result.Rev` gives way to `Result.Project source.Project` |
| `Site.Resolve` | `src.Describe(ctx, "", o.Ref, "")` |
| `Site.Index` | `src.Index(ctx, "", o.Ref)` |
| `Site.Render` | `web.Render(w, web.Document, p)` with `p.Link` the `paths`, `p.Scripts` false, `p.Live` nil, `p.Viewer` empty, `p.Version` `o.Tableaud`, and `Project`, `Data` and `Legend` from `src.View` |
| `Site.Frame` | The same call with `p.Raw` set to the output of `choose.html` and no `Data` |
| `Site.Asset` | `fs.ReadFile(web.Static, "static/"+name)` |
| `tableaud [-C <dir>] export` | `tableaud export [-C DIR] --out DIR [--ref REF]`: `-C` follows the word, as `serve` takes it |
| "The adapter in `cmd/tableaud` that copies fields" | Does not land: `runExport` passes `openSource`'s `Source` to `Run` |

```go
// Index lists what a bundle makes a page for, and what its manifest states.
type Index struct {
	Tasks       []IndexTask // in display order; Tasks[0] is the root
	People      []string    // the emails the project names in a position, sorted by their bytes
	Date        time.Time   // the commit's author date, in the zone the commit records
	Trunk       string      // the trunk's name from version.yaml
	TrunkCommit string      // the commit the trunk stands at
}

// IndexTask is one task of the index.
type IndexTask struct {
	ID, Title, Parent string // Parent is empty for the root
	Children          int
}
```

`sourcetest/testdata/index.json` states the weather station: `a1c0` Weather station; `4e2b` Sensor node, `7b2e` Gateway and `3c5d` Dashboard under it; `9f31` Sensor board and `c07d` Node firmware under `4e2b`; in the order `a1c0`, `4e2b`, `9f31`, `c07d`, `7b2e`, `3c5d`; the people `ada`, `ben`, `dan` and `opus` at `example.org`; the trunk `main` at the commit of `labels.txt`'s last line; the date `2026-09-30`.

The export's request of the audit sets `Today` to `Index.Date`'s day in UTC (A9). The page's context line reads "… · viewer: an observer", the frame's words, where A6 says a page prints no viewer (question 5). The frame's footer stands on every page: the version, and the page's own file name as its link. `levelOf` answers glance and `Inline` is on, so every page holds every level and no lazy fold; a `kids` fold stands open, as `ByLevel` opens it at every level.

**Order** as designed, with `Check`, `plan`, `slug`, the manifest and `write` untouched. Inside the package `Run` calls `run(ctx, src, render, o)`, where `render` is a `func(io.Writer, *web.Page) error` that `Run` binds to `web.Render` as a `Document`; the unit tests call `run` with a `fakeSource` and a fake `render` in the test file.

| Test | As it now reads |
|---|---|
| T1 | Each line of `paths.tsv` as a `web.Link` through `paths.Page`, the fragment appended by the test; a hyphen address expects `""` |
| T4, T7, T13 | "`fakeSite`" reads "the `fakeSource` and the fake `render`"; T13's failure comes from the fake `render` on the twentieth page |
| T6 | The scan finds no `hx-` because `base.html` writes the frame's two attributes under `Scripts` alone |
| T15, T16 | At integrate, with the tablo `Source` and `web.Render`; T16's line holds "viewer: an observer" |

## 6. Assumed interfaces, row by row

| Row | Outcome |
|---|---|
| `438a` A1 | Amended: `web.Render` with a `Shape`; the fragment is `<div id="page">` |
| `438a` A2 | Amended: `web.Linker` and the `Page` methods; the anchor is the caller's to append |
| `438a` A3 | Amended: `part=NAME:KEY`; `Render` builds the model with `openAll` and calls `Part`; an unknown name answers `400`, an unknown key `404`; no `Whole` parameter |
| `438a` A4 | Confirmed, with the frame in place of `Page.Meta` and `Page.Live` |
| `438a` A5 | Confirmed: `Page.Viewer`, `Params.Role`, `Page.Legend` |
| `438a` A6 | Amended: `web.Render` as a `Document` with `Scripts` false |
| `49ce` A8 | Confirmed by `6160` as section 5 maps it |
| `49ce` A9 | Amended: the rules stand under `.v-NAME` and land whole with `438a`; `Parts` land with each adapter |
| `e2b6` A1, `44bf` A1, `9a9c` A1 | Confirmed: `proposed`, `open`, `brief`, `stale`, the range and the parts are the server's parameters |
| `e2b6` A2 | Settled: `200` and the empty form |
| `6160` A1 | Amended: a `switch`, and `-C` after the word |
| `6160` A2 | Confirmed: `web.Render` |
| `6160` A3 | Amended: one `web.Link` per question; an empty answer writes text |
| `6160` A4 | Confirmed by `Scripts` false, once `base.html` guards its two attributes |
| `6160` A5 | Amended: `Page.Raw` |
| `6160` A6 | Amended: the line prints "viewer: an observer" |
| `6160` A7 | Amended: `Source.Index`, which this document declares and the tablo adapter of `49ce` step 5 must implement |

## 7. Where the designs and the code disagree among themselves

1. **`<main class="context">` takes the shared block's `.context` rule** on `main` today: the contextual tableau's whole view draws muted, at `.875rem`. The shared block is fixed by its hash, so the class must differ; `v-NAME` does it.
2. **The frame writes `hx-target` and `hx-swap` whatever `Scripts` says**, so an export page breaks `6160`'s `htmx` rule and its T6 until `base.html` guards them.
3. **`serve.request` passes `Proposed` to tablo; `e2b6` decision 4 filters in the adapter**, since tablo's filter drops the ancestors.
4. **`438a` prints a reference as text; `49ce` decision 9 links an absolute one.**
5. **`438a` lands the sheet whole; `49ce` A9 has each task append.**
6. **`6160` writes `-C` before the word; `serve` takes it after.**
7. **`6160` A6 prints no viewer; the frame prints "viewer: an observer".**
8. **`44bf` uses `Meta`, which `438a` declares for the head** the frame now owns.
9. **Two drafts carry stale facts**: [`shared.css`](shared.css) lacks the block's last newline, so it does not hash to the server's constant; nine of the twenty fixtures write `q` for the question.

## 8. The trial

A throwaway copy of `main` at `a817d79`, outside the repository, applies sections 1 to 4 for `438a`'s steps 1 and 2. On it:

- `gofmt -l`, `go vet ./...` and `golangci-lint run` report nothing, and `go test ./...` passes: every existing test of `cmd/tableaud`, `serve`, `source`, `sourcetest` and the seven prototypes unchanged, and `web`'s with the one line of `TestFrame`.
- T1 to T14 pass as reworded, and the five golden `<main>` elements differ from the drafts' in the class and the question alone.
- A stand-in adapter registered inside a test shows the seam: the model reaches the page, `Render` leaves the caller's page alone, a part receives the value of `Part`, an unknown key is `ErrNoPart`, the document of a part address is inline, data of another type draws the placeholder, and `Raw` replaces the view.
- The key's syntax parses and writes canonically through `ParseQuery`, `Query` and `Page.Self`.
- The built daemon answers `/gates?task=9f31` with the placeholder under `<main id="main" class="v-gates">` and the context line "· task `9f31`", and `/task?task=9f31&part=status` with `400`, since no view lists a part yet.
- A second copy applies the same mechanical changes to the drafts of `e2b6`, `44bf` and `9a9c`: the three `model.md` files compile with `Link` renamed, the nine view and shared files parse under `Render`, and all twenty fixtures pass T2 to T5, T8 to T10. Their `<main>` elements differ from the drafts' in four ways and no other: the class, the question, `V/gates`, and `hx-boost`.

Not run: any adapter, `legendOf`'s body and T15, since tablo exports no view type; `respond`'s `404` for `ErrNoPart` over HTTP, since no model reaches `serve` without an adapter; the `Part` methods of the eight sibling models; anything of `6160`, whose mapping rests on reading `web`, `source` and its design; any browser, so `hx-boost` on a `Control`, the lazy fold beside the frame's `hx-target`, and the sheet under `v-NAME` stand unseen.

## 9. Questions for the owner

1. **The class on `main` and the sheet's scoping.** The mapping writes `class="v-NAME"` with the route's name and scopes each view's rules behind `.v-NAME`; it amends `49ce` decision 8 and T21 by the prefix and changes one token of `base.html`. The alternative keeps `main.NAME`, rewrites the sixty scoped rules of the draft sheet, and adds a rule that undoes `.context` on the contextual tableau.
2. **The sheet lands whole, with `438a`.** The draft is complete and T10 tests it as one file. The alternative has each task append its section, as `49ce` A9 words it, which splits the draft four ways and lets T10's last clause hold at every commit.
3. **Before its adapter exists a route serves the placeholder**, and a template task lands in two parts, the second when tablo's types exist. The alternative writes adapters over the prototypes' JSON that `sourcetest` serves, which shows real pages now and throws the adapters away when tablo's types arrive.
4. **The routes name what a string names; VIEWS.md names the Go types.** View files, the class, the sheet's sections and the fixtures' `View` take `gates` and `context`; `GateView`, `ContextualView`, `view_gate.go` and the fixture file names stand. The alternative renames the types and files to `GatesView` and `ContextView`, which touches every model draft.
5. **The export's context line reads "viewer: an observer".** The alternative guards the viewer clause of `base.html` so that a page without scripts prints none, as `6160` A6 words it.
6. **The adapter filters the authority tree, and `Proposed` leaves `source.Request`.** This follows `e2b6` decision 4. The alternative keeps the field and asks tablo's `493e` for a filter that keeps and marks the ancestors.
7. **The key travels as `part=NAME:KEY`.** The alternative is a fifteenth query name, `key`, which the parser must refuse without `part`.
8. **`Source` gains `Index` as a required method**, after the ruling that made `Reference` one. The alternative is an interface of its own beside `Source`, which the daemon's source need not implement.
9. **An absolute reference is a link on the daemon**, as `49ce` decision 9 binds the templates, and text in the export, as `6160` decision 6 states. The alternative keeps `438a`'s text everywhere.
