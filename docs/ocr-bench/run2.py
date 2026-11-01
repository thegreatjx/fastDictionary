import time
from indra_ocr import load
m = load()
for name in ["plaid_c150", "stock_gs200", "numbers_gs150"]:
    t0 = time.time()
    try:
        md = m.image_ocr(f"{name}.jpg")
        el = (time.time()-t0)*1000
        print(f"\n=== {name} ({el:.0f}ms) ===")
        print(md if isinstance(md,str) else str(md))
    except Exception as e:
        print(f"\n=== {name} FAIL {type(e).__name__}: {e}")
