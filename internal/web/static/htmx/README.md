# Vendored HTMX

This directory holds the one JavaScript file the daemon serves, copied from the
HTMX release the project pins and embedded into the binary by
`internal/web/embed.go`. No build step and no package manager stand between the
source and the binary.

| File           | Content                                                  |
|----------------|----------------------------------------------------------|
| `htmx.min.js`  | The minified HTMX script for the pinned version           |
| `LICENSE`      | The HTMX licence (Zero-Clause BSD) for that version       |
| `README.md`    | This description                                         |

`scripts/vendor-htmx.sh` at the repository root pins the version, fetches both
files from the HTMX repository tag `v<version>`, and prints the SHA-256 of the
script for the commit message. Refresh HTMX by editing the version in the
script, running it, and committing the result; `TestStaticHoldsHTMX` in
`internal/web` checks that both files exist.

This branch has not run the script yet. Implementation runs it and commits
the two files.
