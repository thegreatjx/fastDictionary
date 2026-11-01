"""Throwaway analysis: which rows of the recorded gesture TSV would the
current selection policy reject?  Used to sanity-check the thresholds
before pinning them in selection_policy_test.go."""
import io
import sys

rows = []
for L in io.open('testdata/gestures_2026-10-23.tsv', encoding='utf-8'):
    L = L.rstrip('\n')
    if not L or L.startswith('#'):
        continue
    p = L.split('\t')
    if len(p) < 3:
        print('BAD', L)
        continue
    rows.append((p[0], int(p[1]), p[2]))

print('rows', len(rows))


def wordlike(s):
    for ch in s:
        o = ord(ch)
        if ch.isalnum():
            continue
        if 0x4E00 <= o <= 0x9FFF or 0x3400 <= o <= 0x4DBF:
            continue
        if 0x3040 <= o <= 0x309F or 0x30A0 <= o <= 0x30FF:
            continue
        if 0xAC00 <= o <= 0xD7AF:
            continue
        return False
    return True


rej = []
for owner, n, head in rows:
    h = head.replace('\\n', '\n').replace('\\r', '\r')
    if len(h) >= n:
        t = h[:n]
    else:
        t = h + 'x' * (n - len(h))
    if n <= 1:
        rej.append((owner, n, head, 'one character'))
    elif n <= 3 and not wordlike(t):
        rej.append((owner, n, head, 'fragment'))

print('rejected', len(rej))
for r in rej:
    print(r)
print('2-3 char rows:', [(o, n, h) for o, n, h in rows if 2 <= n <= 3])
