#!/usr/bin/env python3
"""Give every summary the id of its details element with "-s" after it.

Run once from the root of the repository: python3 design/5a2f/summary-ids.py
It rewrites the templates under internal/web/templates in place and prints
the count of summaries it names per file. A summary it cannot name stops it.
"""
import pathlib
import re
import sys

FOLD = re.compile(r'(\{\{template "fold" ([^}]+?)\}\})<summary>')
DETAILS = re.compile(r'(<details\b[^>]*\bid="([^"]+)"[^>]*>)<summary>')


def fold(m):
    expr = m.group(2)
    ident = "{{.ID}}" if expr == "." else "{{" + expr + ".ID}}"
    return m.group(1) + '<summary id="' + ident + '-s">'


total = 0
for path in sorted(pathlib.Path("internal/web/templates").rglob("*.html")):
    text = path.read_text(encoding="utf-8")
    text, a = FOLD.subn(fold, text)
    text, b = DETAILS.subn(lambda m: m.group(1) + '<summary id="' + m.group(2) + '-s">', text)
    if "<summary>" in text:
        sys.exit(str(path) + ": a summary stands after neither a fold nor a details with an id")
    path.write_text(text, encoding="utf-8")
    print(path, a + b)
    total += a + b
print("total", total)
