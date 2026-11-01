import time
from indra_ocr import load
m = load()
print(f"{'image':<18}{'total':>8}{'orient':>9}{'det':>8}{'rec':>9}{'order':>8}")
for n in ["plaid_c150","stock_gs200","numbers_gs150"]:
    pages = list(m.convert(f"{n}.jpg"))
    t = pages[0][1].timings if isinstance(pages[0], tuple) else pages[0].timings
    print(f"{n:<18}{t['total_ms']:>8.0f}{t['orient_ms']:>9.0f}{t['det_ms']:>8.0f}{t['rec_ms']:>9.0f}{t['order_ms']:>8.0f}")
