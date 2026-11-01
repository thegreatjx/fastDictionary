import time
from indra_ocr import load
m = load()
for name in ["plaid_c150", "stock_gs200", "numbers_gs150"]:
    t0 = time.time()
    try:
        pages = list(m.convert(f"{name}.jpg"))
        el = (time.time()-t0)*1000
        print(f"\n=== {name} ({el:.0f}ms, {len(pages)} page) ===")
        for p in pages:
            md = getattr(p, "markdown", None) or getattr(p, "text", None) or str(p)
            print(md)
    except Exception as e:
        print(f"\n=== {name} FAIL {type(e).__name__}: {e}")
