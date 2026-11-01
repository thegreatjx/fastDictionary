#!/usr/bin/env python3
"""Split settings.go into shell / pages / vocabulary by top-level declaration.

Same contract as split_popup.py: each bucket gets explicit imports, then gofmt
and go build prove the split compiles.
"""
import re
from pathlib import Path

ROOT = Path(r"D:\AI_workspace\translate\fastpot")
SRC = ROOT / "settings.go"

PAGES = {
    "pageGeneral", "pageSelection", "pageTranslate", "pageDict", "pageAI",
    "pageHotkey", "pageAbout", "heading",
}
VOCAB = {"pageCustomActions", "pageVocabulary"}

DECL = re.compile(
    r"^(?:func\s+(?:\([^)]*\)\s*)?(?P<func>\w+)|type\s+(?P<type>\w+)|"
    r"(?P<const>const)\s*\(|(?P<var>var)\s+\w+)"
)

lines = SRC.read_text(encoding="utf-8").splitlines(keepends=True)

import_end = 0
for i, ln in enumerate(lines):
    if ln.startswith("import ("):
        for j in range(i, len(lines)):
            if lines[j].strip() == ")":
                import_end = j + 1
                break
        break

starts = []
for i in range(import_end, len(lines)):
    m = DECL.match(lines[i])
    if not m:
        continue
    name = m.group("func") or m.group("type") or m.group("const") or m.group("var")
    s = i
    while s - 1 >= import_end and lines[s - 1].lstrip().startswith("//"):
        s -= 1
    starts.append((s, i, name))

buckets = {"settings.go": [], "settings_pages.go": [], "settings_vocab.go": []}
for idx, (s, i, name) in enumerate(starts):
    end = starts[idx + 1][0] if idx + 1 < len(starts) else len(lines)
    chunk = "".join(lines[s:end])
    if name in PAGES:
        target = "settings_pages.go"
    elif name in VOCAB:
        target = "settings_vocab.go"
    else:
        target = "settings.go"
    buckets[target].append(chunk)

# Imports are chosen per bucket from what the code actually references.
IMPORTS = {
    "settings.go": ['"fmt"', '"log"', '"strings"', '"time"',
                    '"github.com/egoist/mygo"', '"github.com/egoist/mygo/ui"'],
    "settings_pages.go": ['"fmt"', '"log"', '"strings"', '"time"',
                          '"github.com/egoist/mygo"', '"github.com/egoist/mygo/ui"'],
    "settings_vocab.go": ['"fmt"', '"strings"', '"github.com/egoist/mygo/ui"'],
}

for fname, chunks in buckets.items():
    body = "".join(chunks)
    out = "package main\n\nimport (\n"
    out += "".join("\t%s\n" % imp for imp in IMPORTS[fname])
    out += ")\n\n" + body
    (ROOT / fname).write_text(out, encoding="utf-8")
    print("%s: %d bytes, %d chunks" % (fname, len(out), len(chunks)))
