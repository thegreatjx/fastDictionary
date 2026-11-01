import time, sys
from indra_ocr import load

print("loading models (first run downloads ~76MB)...", flush=True)
t0 = time.time()
m = load()
print(f"loaded in {time.time()-t0:.1f}s", flush=True)

for name in ["plaid_c150", "stock_gs200", "numbers_gs150"]:
    t0 = time.time()
    try:
        page = m.run(f"{name}.jpg", mode="ocr")
        el = time.time() - t0
        print(f"\n=== {name} ({el*1000:.0f}ms) ===")
        print(page.markdown if hasattr(page, "markdown") else page)
    except Exception as e:
        print(f"\n=== {name} FAIL {type(e).__name__}: {e}")
