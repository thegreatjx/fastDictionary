import time
from rapidocr import RapidOCR
eng = RapidOCR()
for n in ["plaid_c150","crop_line","stock_gs200"]:
    t0=time.time(); res = eng(f"{n}.jpg" if n!="crop_line" else "crop_line.png"); el=(time.time()-t0)*1000
    txts = res.txts if hasattr(res,"txts") and res.txts else []
    print(f"\n=== {n} ({el:.0f}ms) ===")
    print(" | ".join(txts[:20]) if txts else "(nothing)")
