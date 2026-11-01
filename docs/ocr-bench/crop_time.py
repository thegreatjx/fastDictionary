import time
from indra_ocr import load
m = load()
for n in ["crop_line","crop_block","crop_wide"]:
    ts=[]
    for _ in range(3):
        t0=time.time(); pages=list(m.convert(f"{n}.png")); ts.append((time.time()-t0)*1000)
    pg=pages[0][1] if isinstance(pages[0],tuple) else pages[0]
    print(f"\n{n}  median={sorted(ts)[1]:.0f}ms")
    print(" ", pg.markdown.replace("\n"," | ")[:200])
