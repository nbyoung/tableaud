# Prototype 5a2f: accessibility and theming

Task `5a2f` (Accessibility and theming) at the `function` gate. The prototype builds one tableau page and one legend page as a test bed and answers five questions about them. The layout is provisional: no HTML mockup exists yet, so the prototype settles function, not look.

## Verified and not verified

No browser, screen reader or phone exists on this machine (`chromium`, `google-chrome`, `firefox`, `node`, `lynx` and `w3m` are absent). Every claim below rests on Go tests that parse the HTML and CSS the server returns, plus a read of the vendored HTMX source. Nothing ran in a browser. The script of the treegrid never ran at all: there is no JavaScript engine here, so it has no test beyond a size and keyword check. A person must run it (see the manual check procedure).

## Run and test

```
export PATH=$PATH:/usr/local/go/bin
go test ./...                                  # from the worktree root
go test -v -run 'Contrast|PageSize|Treegrid' ./prototype/5a2f   # ratios, sizes
go run ./prototype/5a2f --addr 127.0.0.1:0     # prints the address; then open /tableau and /legend
go run ./prototype/5a2f --render '/tableau?mode=grid&layout=stacked'
go run ./prototype/5a2f --render '/tableau/rows?open=a1c0,4e2b' --fragment
```

Routes: `/tableau` with `mode=table|grid`, `layout=scroll|stacked` and `open=ID,ID` (no `open` opens the root; `open=` opens none), `/tableau/rows` (the HTMX fragment) and `/legend`. A single-hyphen long option (`-addr`) exits with status 2 and names `--addr`.

## Test data

Copies in `testdata/`, rendered as they are:

| File | Source command (run from `tablo`) |
|---|---|
| `global-tableau.json` | copy of `prototype/886d/output/global-tableau.json` (`go run ./prototype/886d -repo .../weather-station -ref main`) |
| `queue-ada.json` | copy of `prototype/886d/output/queue-ada.json` (`-view queue -person ada@example.org`) |
| `gate.json` | `go run ./prototype/493e -repo /home/nbyoung/Projects/Tableaux/tableaux/corpus/build/weather-station -ref main -view gate` |

Every symbol, gate name, state, reason, mark meaning and criterion comes from these files. The code holds no symbol; a test fails if it finds one in a source file, template or style sheet.

## Questions and answers

### 1. Does every symbol carry a text alternative from the data? Yes, as far as markup goes.

Markup: a visible symbol in `<span class="sym" aria-hidden="true">` beside visually hidden text (`class="vh"`), one label per cell:

```
<td data-kind="status" data-state="stalled"><span class="sym" aria-hidden="true">🔴</span><span class="sym" aria-hidden="true">⛔</span><span class="label vh"><span class="ctx vh">Sensor board, </span>Functional prototype: stalled, blocked</span></td>
```

The cell reads "Sensor board, Functional prototype: stalled, blocked", not two emoji names. Reasons for the choice over `role="img"` with `aria-label`: the text is real content (it survives translation tools, copy, find and a missing stylesheet); the stacked layout reveals it with CSS; an `aria-label` on a `span` with no role is ignored by some readers, and `role="img"` inside a cell hides the cell's own text structure. The cost is one extra span per cell (about 100 bytes) and a convention: the hidden text names the gate, so the label stays right when the cell is read out of its column.

The text comes from the data: a gate's `name`, a state's `key` (underscore to space), a reason's `key`, a mark's `meaning`. A tokenizer splits the cell's symbol string (for example `🔴⛔` or `🧑👀`) against the definition, longest symbol first and ignoring the variation selector. An unknown symbol renders as "unrecognised symbol" and a test fails on it.

Tests: for every page and variant, each `aria-hidden` element must consist only of symbols from the definition (read independently from `gate.json` in the test), and the nearest cell or list item must carry each symbol's name as readable text. No symbol may appear outside `aria-hidden`. The legend must show every symbol. A mutation check (removing the label span) made the test fail.

### 2. Keyboard alone, with and without script

Two variants, same data: `mode=table` (links in the tab order) and `mode=grid` (ARIA `treegrid`, roving `tabindex`, arrow keys).

| | Plain table (`mode=table`) | Treegrid (`mode=grid`) |
|---|---|---|
| Works without script | Yes, fully: each expander is an `<a href>` that reloads the page with the new `open` set. | Expansion falls back to the same links, but the page is one tab stop per link only; there is no arrow-key movement. Screen readers still supply their own table keys. |
| Script | HTMX (51 KB, already vendored) only to avoid the reload. | 2262 bytes of inline script, no library; it also works without HTMX. |
| Tab stops | Every expander is a tab stop: 1 per parent row plus the scroller. A long tree costs many Tab presses. | One stop for the grid, arrow keys inside. Fewer presses. |
| Screen reader | Plain table semantics, which readers know well; level said in hidden text ("level 3"). | `treegrid` semantics with `aria-level` and `aria-expanded`; support varies and it needs focus mode. |
| Risk | Low. | Higher: the script is untested here, and a wrong roving index strands focus. |

Focus after an HTMX swap (plain table): each expander has a stable `id` (`tog-4e2b`) and the fragment returns the same id. The vendored HTMX 2.0.6 saves the focused element before a swap and, when it has an `id`, refocuses the element with that id after the swap (`preventScroll` follows `htmx.config.defaultFocusScroll`). So the focus should stay on the link. The test proves the fragment contains the same id, that the fragment's rows equal the full page's rows for the same `open` set, and that a `role="status"` out-of-band message ("5 of 6 tasks shown.") ships with the fragment. Not run: that HTMX actually restores focus and that a screen reader speaks the status message. Without the id, focus would drop to the body. A treegrid needs no swap for expansion (all rows are in the page and the script toggles `hidden`), so its focus never moves; if a later version swaps rows into a treegrid, the response must carry the roving `tabindex` and `htmx:afterSwap` must restore it.

Recommendation: the plain table with link expanders and HTMX as enhancement. It works without script, it needs no custom key handling, and its cost (many tab stops) is real but bounded by progressive disclosure: only open branches show rows. Offer the treegrid later as an opt-in only after a person tests it with a screen reader. Neither variant is verified in a browser.

### 3. Light and dark themes, no flash, colour not alone

CSS alone: `:root { color-scheme: light dark }`, custom properties for every colour, a `@media (prefers-color-scheme: dark)` block that redefines them, and `<meta name="color-scheme" content="light dark">` so the canvas is right before the stylesheet arrives. No script touches the theme (a test rejects `matchMedia`), and the legend page has no script at all. A page that loads CSS from a `<link>` renders after the sheet loads, so no light frame shows in a dark system; this reasoning is not observed. The cost: no manual toggle without script or a cookie.

Contrast (WCAG 2 formula, tested in `style_test.go`; `go test -v` prints every pair). Text pairs: `fg`, `muted` and `link` on `bg`, `surface` and each of the five state backgrounds, all at least 5.22 in dark and 6.15 in light; the lowest text pairs are link on at_risk in dark (5.22) and link on stalled in light (6.15). Borders and the focus ring reach at least 3.41 (non-text, 3 to 1 required). The test fails under 4.5 (3 for non-text), checks that every colour variable sits in a tested pair, that both themes define the same variables, and that no literal colour sits outside the two variable blocks. A mutation (`--fg` to `#9a9a9a`) made it fail at 2.81.

Colour not alone: the state circles differ in colour only, so each state also gets a cell border pattern (dotted, solid, dashed, double, groove, keyed by the state key from the data) and its word in the markup, which the stacked layout shows. A test requires one distinct pattern per state in the data. Whether the patterns are distinguishable at a glance in a wide table needs a person.

Motion: `scroll-behavior` and transitions sit only inside `prefers-reduced-motion: no-preference` (tested). `forced-colors: active` keeps the border patterns and a heavier border, and sets the focus colour to `Highlight`; its appearance is not seen.

### 4. A wide tableau at 320 pixels

Both layouts exist from the same markup (a test asserts the markup differs only in the wrapper class and in `role` attributes that keep table semantics when `display: block` removes them).

- Scroll with a sticky task column: `.scroller { overflow-x: auto }`, `th.task { position: sticky; inset-inline-start: 0 }`, the scroller is a named region with `tabindex="0"` so a keyboard can scroll it. Cost: nine gate columns do not fit; the reader scrolls sideways, but the task name stays beside every cell and columns still compare across rows.
- Stacked (`layout=stacked`, below 40em): one card per task, each non-blank cell as a line with its symbols and its now visible label ("Functional prototype: stalled, blocked"); blank cells hide. Cost: no comparison across tasks, a long page, and the roles above.

Tested: the viewport meta (`width=device-width, initial-scale=1`, zoom allowed), no CSS `width`, `min-width` or `max-width` over 320 pixels (largest: 14rem = 224), the sticky rule, the stacked rules, page size (default tableau 10.6 KB, 14.3 KB with more rows open, treegrid 17.4 KB, legend 4.5 KB, stylesheet 5.2 KB, all under the test limits of 24 KB and 8 KB). Not tested: that the table really reads at 320 pixels, that sticky works inside the scroller on a phone, and tap target size.

Recommendation: the scrolling table with the sticky task column as the default, because the tableau exists to compare tasks across gates, and the stacked card as the layout below 40em only if a phone test shows that scrolling fails. This recommendation is a judgement, not a measurement.

### 5. Structure

Tested on every page and variant: one `h1`; headings in order without gaps; `lang`; a non-empty title; one `caption` and one `thead` per table; `scope` on every `th`; unique ids; `aria-controls`, `aria-labelledby` and `#` links that resolve; no positive `tabindex`; link text that is non-empty, not generic and leads to one place ("Show Sensor board in the tableau", "Expand Sensor node"); the first link is a skip link to `main`; landmarks `main` and `nav`; a `:focus-visible` outline in both themes and no rule that removes it. The expander's visible marker is drawn by CSS (no character), so it carries no symbol; its name is hidden text, which voice-control users cannot say (a cost to settle at design).

## What the pages show

`/tableau?open=a1c0,4e2b` (trimmed):

```
<th scope="col"><span aria-hidden="true">⚙️</span><span class="vh">Functional prototype</span></th>
<tr id="row-9f31" class="depth-2">
<th scope="row" class="task depth-2"><span class="toggle none"></span><span class="title">Sensor board</span> <span class="tid">9f31</span><span class="vh">, level 3</span></th>
<td data-kind="marks"><span class="sym" aria-hidden="true">🧑</span><span class="sym" aria-hidden="true">👀</span><span class="label vh"><span class="ctx vh">Sensor board, </span>Mockup: a person contributes, a reviewer accepts</span></td>
```

`/legend` rows, trimmed: `<th scope="row"><span aria-hidden="true">🟡</span> <span>at risk</span></th><td>The work is at risk of stalling</td>`.

## Manual check procedure for the owner

1. Keyboard, plain table: open `/tableau`; press Tab; the skip link appears first. Tab to an expander (a visible focus ring), press Enter; with HTMX the page does not reload and focus stays on the same expander; the URL changes. Disable script and repeat: the page reloads and `#tog-ID` keeps the position.
2. Keyboard, treegrid: open `/tableau?mode=grid`; Tab enters the grid once; arrows move cell by cell; Right on a closed parent row header opens it, Left closes it, Home and End jump, Ctrl+Home and Ctrl+End reach the ends; Tab leaves the grid. Fix the script if any key strands focus.
3. Screen reader (NVDA with Firefox, VoiceOver with Safari): read a cell in the table; expect "Sensor board, Functional prototype: stalled, blocked" or the column and row headers plus the label, and no emoji names. Expand a row and listen for "N of 6 tasks shown".
4. Theme: switch the system between light and dark and reload; no light frame shows in dark; read every state and the focus ring. Turn on a high-contrast or forced-colours mode and check that the state border patterns stay distinguishable.
5. Motion: set "reduce motion" and check nothing slides.
6. Phone: open at 320 pixels wide (a phone or browser devtools): the task column stays while the table scrolls sideways; `layout=stacked` shows cards with the labels. Zoom to 200 per cent and check nothing clips; tap targets reach 24 CSS pixels.

## Stand-ins and omissions

- State and reason names: the gate definition gives a state and a reason a `key` and a `synopsis` but no `name`; the prototype uses the key with underscores read as spaces ("at risk"). Gates and marks do carry a name and a meaning.
- The tableau uses `kind`, `symbols`, `gate` of each cell only. It omits the status note, date, folded-column counts (`folded`, all zero here), `next_gate`, and the authorisation mark.
- One task detail page and the other views do not exist; the queue links point into the tableau and open the task's ancestors.
- The treegrid sets no `aria-posinset` or `aria-setsize`.
- The pages use their own templates under `prototype/5a2f/templates/`, not `web.Templates`' `base.html`, because that file loads HTMX on every page and has no skip link, theme meta or stylesheet slot. They use `web.Static` for HTMX.

## Findings for the design gate

- The gate definition needs a state `name` and a reason `name` (and, ideally, a short accessible name for every symbol); otherwise each front end invents it.
- Cell `symbols` is a concatenated string. Front ends must tokenize it against the definition, and that works only while no symbol is a prefix of another. A list of `{kind, key}` per cell would be safer and carry the alternative text without a lookup.
- The colour circles alone cannot carry state: the design needs a second visual cue (border pattern here, or a visible word). Decide which.
- Decide the expander's name: a visible word helps voice control; hidden text is shorter.
- Decide whether a manual theme toggle is worth a script and a cookie; CSS alone follows only the system.
- Decide plain table or treegrid after a screen-reader test; this prototype recommends the plain table.
- Decide the stacked breakpoint after a phone test.
