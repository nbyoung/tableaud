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

## Licence

[MIT](LICENSE).
