#!/bin/sh
# corpus.sh: the tableaux repository and its built corpus, for the corpus job.
#
#   sh .github/corpus.sh fetch <directory>
#
# fetch  clones the repository that corpus.lock.yaml names into <directory>,
#        checks out the locked commit and its submodules, points the clone's
#        own tag corpus/tooling at the locked commit of that tag, and builds the
#        corpus under <directory>/corpus/build with the clone's corpus/build.sh.
#        It fails when the clone lacks either commit. CI runs it; it needs the
#        network, sh and git.
#
# The tests then read TABLEAUX_CORPUS=<directory>/corpus/build and
# TABLEAUX_DIR=<directory>.

set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
LOCK="$HERE/corpus.lock.yaml"

# field <key>: the value of a top-level scalar of the lock.
field() { sed -n "s/^$1: *\([^ #]*\).*/\1/p" "$LOCK"; }

# need <directory> <commit>: fail unless the clone holds the commit.
need() {
  if ! git -C "$1" cat-file -e "$2^{commit}" 2> /dev/null; then
    echo "corpus.sh: $(field repository) holds no commit $2" >&2
    exit 1
  fi
}

fetch() {
  dir=$1
  commit=$(field commit)
  tooling=$(field tooling)
  git clone -q --no-checkout "$(field repository)" "$dir"
  need "$dir" "$commit"
  need "$dir" "$tooling"
  git -C "$dir" checkout -q --detach "$commit"
  git -C "$dir" submodule -q update --init
  git -C "$dir" tag -f corpus/tooling "$tooling" > /dev/null
  sh "$dir/corpus/build.sh" > /dev/null
  echo "corpus.sh: $dir holds $(field repository) at $commit and its built corpus"
}

case "${1:-}" in
  fetch) [ $# -eq 2 ] || { echo "usage: corpus.sh fetch <directory>" >&2; exit 2; }
         fetch "$2" ;;
  *)     echo "usage: corpus.sh fetch <directory>" >&2; exit 2 ;;
esac
