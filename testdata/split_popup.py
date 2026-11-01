#!/usr/bin/env python3
"""Split popup.go into view / actions / layout files by top-level declaration.

Reads the current popup.go, groups every top-level declaration (with its doc
comment) into one of four buckets, and writes each bucket as its own file.
Imports are written explicitly per bucket; gofmt + go build confirm the split.
"""
import re
import sys
from pathlib import Path

ROOT = Path(r"D:\AI_workspace\translate\fastpot")
SRC = ROOT / "popup.go"

# Which bucket each top-level declaration belongs to.
VIEW = {
    "renderIconButton", "popupView", "renderNotesBlock", "dictBlock",
    "renderEmptyActionStart", "sameLang", "langPrefix", "renderAiMarkdown",
}
ACTIONS = {
    "translate", "extractNotes", "saveDictToVocab", "saveGrammarToVocab",
    "saveWritingToVocab", "runAction",
}
LAYOUT = {
    "calcPopupPosition", "calcPopupPositionIn", "popupContentHeight",
    "clampPopupHeight", "liftPopupY", "fitHeight", "wrappedLines",
}
# Everything else stays in popup.go (types, app state, snapshot, transitions).

DECL = re.compile(
    r"^(?:func\s+(?:\([^)]*\)\s*)?(?P<func>\w+)|type\s+(?P<type>\w+)|"
    r"(?P<const>const)\s*\(|(?P<var>var)\s+\w+)"
)

lines = SRC.read_text(encoding="utf-8").splitlines(keepends=True)

# Find the end of the import block.
import_end = 0
for i, ln in enumerate(lines):
    if ln.startswith("import ("):
        for j in range(i, len(lines)):
            if lines[j].strip() == ")":
                import_end = j + 1
                break
        break
header = "".join(lines[:import_end])

# Locate every top-level declaration and its doc comment.
starts = []
for i in range(import_end, len(lines)):
    m = DECL.match(lines[i])
    if not m:
        continue
    name = m.group("func") or m.group("type") or m.group("const") or m.group("var")
    # Walk back over the contiguous comment block directly above.
    s = i
    while s - 1 >= import_end and lines[s - 1].lstrip().startswith("//"):
        s -= 1
    starts.append((s, i, name))

buckets = {"popup.go": [], "popup_view.go": [], "popup_actions.go": [], "popup_layout.go": []}
for idx, (s, i, name) in enumerate(starts):
    end = starts[idx + 1][0] if idx + 1 < len(starts) else len(lines)
    chunk = "".join(lines[s:end])
    if name in VIEW:
        target = "popup_view.go"
    elif name in ACTIONS:
        target = "popup_actions.go"
    elif name in LAYOUT:
        target = "popup_layout.go"
    else:
        target = "popup.go"
    buckets[target].append(chunk)

IMPORTS = {
    "popup.go": ['"context"', '"fmt"', '"log"', '"strings"', '"sync"', '"time"',
                 '"github.com/egoist/mygo"', '"github.com/egoist/mygo/ui"'],
    "popup_view.go": ['"strings"', '"github.com/egoist/mygo"', '"github.com/egoist/mygo/ui"',
                      '"github.com/egoist/mygo/ui/markdown"'],
    "popup_actions.go": ['"context"', '"encoding/json"', '"fmt"', '"log"', '"strings"',
                         '"sync"', '"time"', '"github.com/egoist/mygo"'],
    "popup_layout.go": ['"strings"', '"github.com/egoist/mygo"'],
}

for fname, chunks in buckets.items():
    imports = IMPORTS[fname]
    body = "".join(chunks)
    out = "package main\n\nimport (\n"
    out += "".join("\t%s\n" % imp for imp in imports)
    out += ")\n\n" + body
    (ROOT / fname).write_text(out, encoding="utf-8")
    print("%s: %d bytes, %d chunks" % (fname, len(out), len(chunks)))
