# tableaud

The Tableaux graphical front end: a local daemon that serves every view as HTML with role-specific progressive disclosure, and a static export.

The name plays on *Tableaux*: it joins *tableau* to the *d* of a daemon, as in httpd.

## Place in the family

| Project                                            | Role                                                     |
|----------------------------------------------------|----------------------------------------------------------|
| [tableaux](https://github.com/nbyoung/tableaux)    | The language: method, syntax, schemas, corpus, mockups   |
| [tablo](https://github.com/nbyoung/tablo)          | The backend: library and plumbing command                |
| [tabloio](https://github.com/nbyoung/tabloio)      | The command line: textual output and Git input           |
| [tablotui](https://github.com/nbyoung/tablotui)    | The terminal user interface                              |
| [tableaud](https://github.com/nbyoung/tableaud)    | The local daemon: HTML views with progressive disclosure |

The split follows Git's own: `tablo` is plumbing that reads a repository and
emits data, and the three front ends are porcelain that presents it. A front end
never reads a task file itself.

## What it does

- **Serve** every view as HTML from the checked-out repository, on localhost by default.
- **Disclose** progressively by role: the owner expands top-down through the task hierarchy; a contributor opens at the parents of their tasks in a gate window around their next gates.
- **Remember** which columns a viewer hides or shows; the server computes only the default window and imposes no column policy.
- **Link** every state of expansion, so a page shares as a URL.
- **Export** a self-contained static bundle for hosting where no daemon runs.

```
tableaud serve                    # http://localhost:8642/
tableaud serve --as ben@example.org --at main
tableaud export --out public/     # a static bundle for CI to publish
```

## Plan

The project's plan is the Tableaux project in [`.tableaux/`](.tableaux/). The
[Tableaux tooling plan](https://github.com/nbyoung/tableaux/blob/main/PLAN.md)
in the `tableaux` repository pins this project as the submodule
`subprojects/tableaud` and tracks its root task through a recursive junction, states the review policy every task here inherits, and
proposes Go as the implementation language.

## Layout

The module is `github.com/nbyoung/tableaud`. It uses the standard library alone
for the server, the templates and the embedding, and it shares one skeleton
shape with the other three subprojects.

```
cmd/tableaud/            # the command; main.go prints the version until the server lands
internal/web/            # what the daemon serves
  doc.go                 # the package description
  embed.go               # go:embed of templates/ and static/
  templates/             # html/template files; base.html is the layout every view extends
  static/htmx/           # the vendored HTMX script, its licence and a README
  web_test.go            # the tests for the templates and the embedded assets
scripts/vendor-htmx.sh   # fetches the pinned HTMX release
.github/workflows/       # ci.yml on push and pull request, release.yml on a tag
.goreleaser.yaml         # the release matrix
.golangci.yml            # the lint configuration
.tableaux/               # the plan
```

## Build and test

A person who builds from source needs the Go release the `go` directive in
`go.mod` names, or newer; the toolchain fetches a newer one on demand. No user
needs Go: see [Release](#release).

```
go build ./cmd/tableaud          # the binary in the working directory
go test ./...                    # the tests
gofmt -l . && go vet ./...       # what CI checks before the tests
```

CI (`.github/workflows/ci.yml`) runs on every push to `main` and every pull
request: a `gofmt` check, `go vet`, `golangci-lint` through its official
action with the standard linters, and `go test ./...`. A merge to `main`
therefore always passes those four.

## Release

A tag `v<major>.<minor>.<patch>` on `main` triggers
`.github/workflows/release.yml`, which runs GoReleaser with
`.goreleaser.yaml`. GoReleaser cross-compiles `cmd/tableaud` with
`CGO_ENABLED=0` for Linux, macOS and Windows on amd64 and arm64, so each binary
depends on nothing on the host but `git`, and publishes one archive per target
and a `checksums.txt` to the GitHub release for the tag. The linker stamps the
version, commit and date, and `tableaud version` prints them. A source build
reports the module version instead.

```
git tag v0.1.0 && git push origin v0.1.0
```

## Dependencies

`tableaud` depends on `tablo` for every view's data and on nothing else outside
the standard library. Go modules state a minimum, not a range, so the range is
a policy:

- `go.mod` requires the lowest `tablo` release whose view data `tableaud`
  renders, and CI builds and tests against exactly that minimum.
- While `tablo` is at major version 0, `tableaud` accepts any patch of that
  minor and raises the minimum with each `tablo` minor it adopts. From
  `tablo` v1, the range is the major version, as Go's import compatibility
  rule intends.
- `go.mod` carries no `require` for `tablo` until `tablo` tags a release,
  since a requirement that resolves to nothing breaks every `go` command.
  Adding the requirement is the first step of the server task.
- `go.mod` never carries a `replace`. Day-to-day work across the subprojects
  uses a personal `go.work` in the directory above the checkouts, which points
  at `./tablo` and the others and stays uncommitted, as the tooling plan's next
  steps describe; the `tableaux` repository commits its own `go.work` against
  the pinned submodules.

## Vendored assets

HTMX is the one script the daemon serves. It is a single file, so the project
vendors it rather than adding a package manager or a build step:

- `internal/web/static/htmx/htmx.min.js` is the minified script of the release
  that `scripts/vendor-htmx.sh` pins, and `LICENSE` beside it is that
  release's licence (Zero-Clause BSD).
- `internal/web/embed.go` embeds the whole `static/` directory, so the script
  and its licence ship inside the binary and the server mounts them under
  `/static/`.
- To refresh, edit `HTMX_VERSION` in `scripts/vendor-htmx.sh`, run it, and
  commit the two files with the SHA-256 the script prints.
- `TestStaticHoldsHTMX` fails until the script has run, so a checkout without
  the vendored files does not pass the tests.

## Licence

[MIT](LICENSE).
