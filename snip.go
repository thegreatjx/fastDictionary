package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

// snipOCRScale is how much the crop is enlarged before Windows.Media.Ocr reads
// it. 2x is measured as the best value: small type becomes legible, and larger
// factors start losing accuracy again (the engine's own detection struggles
// with very large glyphs).
const snipOCRScale = 2

// maxOCRSide is OcrEngine.MaxImageDimension. A crop scaled past it makes the
// engine throw instead of returning text, so an enormous selection stays at 1x
// rather than failing outright.
const maxOCRSide = 10000

var (
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetDesktopWindow       = user32.NewProc("GetDesktopWindow")
	procOpenInputDesktop       = user32.NewProc("OpenInputDesktop")
	procSetThreadDesktop       = user32.NewProc("SetThreadDesktop")
	procCloseDesktop           = user32.NewProc("CloseDesktop")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procCreateCompatibleDC     = gdi32Dll.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32Dll.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32Dll.NewProc("SelectObject")
	procBitBlt                 = gdi32Dll.NewProc("BitBlt")
	procGetDIBits              = gdi32Dll.NewProc("GetDIBits")
	procDeleteDC               = gdi32Dll.NewProc("DeleteDC")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procShowWindow             = user32.NewProc("ShowWindow")
	procUpdateWindow           = user32.NewProc("UpdateWindow")
	procSetCapture             = user32.NewProc("SetCapture")
	procReleaseCapture         = user32.NewProc("ReleaseCapture")
	procBeginPaint             = user32.NewProc("BeginPaint")
	procEndPaint               = user32.NewProc("EndPaint")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procFillRect               = user32.NewProc("FillRect")
	procFrameRect              = user32.NewProc("FrameRect")
	procCreateSolidBrush       = gdi32Dll.NewProc("CreateSolidBrush")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")

	msimg32Dll     = syscall.NewLazyDLL("msimg32.dll")
	procAlphaBlend = msimg32Dll.NewProc("AlphaBlend")
)

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type paintStruct struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     winRect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msgW struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       winPoint
	LPrivate uint32
}

// captureScreenArea takes a snapshot of a screen rectangle using GDI BitBlt
func captureScreenArea(x, y, w, h int32) (*image.RGBA, error) {
	hInputDesk, _, _ := procOpenInputDesktop.Call(0, 0, 0x01FF)
	if hInputDesk != 0 {
		procSetThreadDesktop.Call(hInputDesk)
		defer procCloseDesktop.Call(hInputDesk)
	}

	hwndDesk, _, _ := procGetDesktopWindow.Call()
	hdcScreen, _, _ := procGetDC.Call(hwndDesk)
	if hdcScreen == 0 {
		hdcScreen, _, _ = procGetDC.Call(0)
	}
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(hwndDesk, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(hdcMem)

	hbm, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, uintptr(w), uintptr(h))
	if hbm == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(hbm)

	oldBmp, _, _ := procSelectObject.Call(hdcMem, hbm)

	const srccopy = 0x00CC0020
	rBlt, _, errBlt := procBitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen, uintptr(x), uintptr(y), srccopy)
	if rBlt == 0 {
		procSelectObject.Call(hdcMem, oldBmp)
		return nil, fmt.Errorf("BitBlt failed: %v", errBlt)
	}

	// Must deselect bitmap before calling GetDIBits
	procSelectObject.Call(hdcMem, oldBmp)

	bih := bitmapInfoHeader{
		BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		BiWidth:       w,
		BiHeight:      -h, // top-down
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: 0, // BI_RGB
	}

	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	procGetDIBits.Call(
		hdcMem, hbm, 0, uintptr(h),
		uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(unsafe.Pointer(&bih)),
		0,
	)

	// BGRA -> RGBA swap
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
		img.Pix[i+3] = 255
	}

	return img, nil
}

type snipSession struct {
	vx, vy, vw, vh int32
	hdcMem         uintptr // holds the full-screen frozen snapshot
	hbm            uintptr
	oldBmp         uintptr
	hdcDark        uintptr // 1x1 black DC for AlphaBlend
	hbmDark        uintptr
	oldDarkBmp     uintptr
	selecting      bool
	startX, startY int32
	curX, curY     int32
	cropDone       bool
	cropRect       winRect
	onSuccess      func(cropRect winRect, imgPath string)
	onError        func(err error)
}

var (
	currentSnip   *snipSession
	snipMu        sync.Mutex
	snipClassOnce sync.Once
	snipAtom      uintptr
)

// beginSnipSessionLocked 声明这次框选会话的全局单例。调用方必须持有 snipMu。
// 已有会话在进行时返回 false，调用方应放弃本次启动，避免两个消息循环共用
// 一个 currentSnip、各自置 nil 并乱序触发回调。
func beginSnipSessionLocked(s *snipSession) bool {
	if currentSnip != nil {
		return false
	}
	currentSnip = s
	return true
}

// endSnipSessionLocked 只在全局单例仍属于 s 时才清空。调用方必须持有 snipMu。
// 身份判断避免一次迟到的清理把后来启动的新会话误清掉。
func endSnipSessionLocked(s *snipSession) {
	if currentSnip == s {
		currentSnip = nil
	}
}

func registerSnipClass() {
	snipClassOnce.Do(func() {
		className, _ := syscall.UTF16PtrFromString("FastDictSnipOverlay")
		cursor, _, _ := procLoadCursorW.Call(0, uintptr(32515)) // IDC_CROSS = 32515
		wndProc := syscall.NewCallback(snipWndProc)

		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			Style:         3, // CS_HREDRAW | CS_VREDRAW
			LpfnWndProc:   wndProc,
			HCursor:       cursor,
			LpszClassName: className,
		}
		atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		snipAtom = atom
	})
}

func minI32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func maxI32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func snipWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	snipMu.Lock()
	s := currentSnip
	snipMu.Unlock()

	switch msg {
	case 0x0201: // WM_LBUTTONDOWN
		if s != nil {
			s.selecting = true
			s.startX = int32(int16(lParam & 0xFFFF))
			s.startY = int32(int16((lParam >> 16) & 0xFFFF))
			s.curX = s.startX
			s.curY = s.startY
			procSetCapture.Call(hwnd)
			procInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}

	case 0x0200: // WM_MOUSEMOVE
		if s != nil && s.selecting {
			s.curX = int32(int16(lParam & 0xFFFF))
			s.curY = int32(int16((lParam >> 16) & 0xFFFF))
			procInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}

	case 0x0202: // WM_LBUTTONUP
		if s != nil && s.selecting {
			s.selecting = false
			procReleaseCapture.Call()
			x1 := minI32(s.startX, s.curX)
			x2 := maxI32(s.startX, s.curX)
			y1 := minI32(s.startY, s.curY)
			y2 := maxI32(s.startY, s.curY)
			w := x2 - x1
			h := y2 - y1
			if w >= 10 && h >= 10 {
				s.cropDone = true
				s.cropRect = winRect{
					Left:   s.vx + x1,
					Top:    s.vy + y1,
					Right:  s.vx + x2,
					Bottom: s.vy + y2,
				}
			}
			procDestroyWindow.Call(hwnd)
			return 0
		}

	case 0x0204, 0x0205: // WM_RBUTTONDOWN, WM_RBUTTONUP (Right click cancels)
		if s != nil && s.selecting {
			s.selecting = false
			procReleaseCapture.Call()
		}
		procDestroyWindow.Call(hwnd)
		return 0

	case 0x0100: // WM_KEYDOWN
		if wParam == 0x1B { // VK_ESCAPE
			if s != nil && s.selecting {
				s.selecting = false
				procReleaseCapture.Call()
			}
			procDestroyWindow.Call(hwnd)
			return 0
		}

	case 0x000F: // WM_PAINT
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 && s != nil {
			const srccopy = 0x00CC0020
			// 1. Blit base frozen screen snapshot
			procBitBlt.Call(hdc, 0, 0, uintptr(s.vw), uintptr(s.vh), s.hdcMem, 0, 0, srccopy)

			blendDim := func(x, y, w, h int32, alpha byte) {
				if w <= 0 || h <= 0 {
					return
				}
				blendVal := uint32(0) | (uint32(alpha) << 16)
				procAlphaBlend.Call(
					hdc, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
					s.hdcDark, 0, 0, 1, 1,
					uintptr(blendVal),
				)
			}

			if s.selecting {
				x1 := minI32(s.startX, s.curX)
				x2 := maxI32(s.startX, s.curX)
				y1 := minI32(s.startY, s.curY)
				y2 := maxI32(s.startY, s.curY)

				// Dim 4 rectangles outside selection (~35% dark = alpha 90)
				blendDim(0, 0, s.vw, y1, 90)
				blendDim(0, y2, s.vw, s.vh-y2, 90)
				blendDim(0, y1, x1, y2-y1, 90)
				blendDim(x2, y1, s.vw-x2, y2-y1, 90)

				// Draw 2px selection border (#2563EB Royal Blue, BGR: 0x00EB6325)
				blueBrush, _, _ := procCreateSolidBrush.Call(0x00EB6325)
				rSel := winRect{x1, y1, x2, y2}
				procFrameRect.Call(hdc, uintptr(unsafe.Pointer(&rSel)), blueBrush)
				rSelInner := winRect{x1 + 1, y1 + 1, x2 - 1, y2 - 1}
				procFrameRect.Call(hdc, uintptr(unsafe.Pointer(&rSelInner)), blueBrush)
				procDeleteObject.Call(blueBrush)
			} else {
				// Gentle dim across screen (~14% dark = alpha 35) to indicate snipping overlay active
				blendDim(0, 0, s.vw, s.vh, 35)
			}

			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}

	case 0x0002: // WM_DESTROY
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// snipCacheDir is where the cropped screenshot lands before OCR reads it.
// mygo's cache path is preferred; %APPDATA% is the fallback for the rare
// machine where the framework path is unavailable, and an empty APPDATA
// (a service account) must not turn into \Dictionary\cache silently.
func snipCacheDir() (string, error) {
	if dir, err := mygo.App.Path(mygo.PathCache); err == nil && dir != "" {
		return dir, nil
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, appName, "cache"), nil
	}
	return "", fmt.Errorf("no cache directory available")
}

// StartNativeSnip launches the native instant screen snipping overlay.
// onError is invoked when the crop cannot be saved, so the caller can tell
// the user instead of the overlay just vanishing.
func StartNativeSnip(onSuccess func(cropRect winRect, imgPath string), onError func(err error)) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// 1. Ensure thread desktop is attached to active user input desktop
		hInputDesk, _, _ := procOpenInputDesktop.Call(0, 0, 0x01FF)
		if hInputDesk != 0 {
			procSetThreadDesktop.Call(hInputDesk)
			defer procCloseDesktop.Call(hInputDesk)
		}

		registerSnipClass()

		// 2. Metrics for virtual multi-monitor screen
		vx, _, _ := procGetSystemMetrics.Call(76)
		vy, _, _ := procGetSystemMetrics.Call(77)
		vw, _, _ := procGetSystemMetrics.Call(78)
		vh, _, _ := procGetSystemMetrics.Call(79)
		if vw == 0 || vh == 0 {
			vw, _, _ = procGetSystemMetrics.Call(0)
			vh, _, _ = procGetSystemMetrics.Call(1)
		}

		// 3. Capture screen directly to GDI memory DC
		hwndDesk, _, _ := procGetDesktopWindow.Call()
		hdcScreen, _, _ := procGetDC.Call(hwndDesk)
		if hdcScreen == 0 {
			hdcScreen, _, _ = procGetDC.Call(0)
		}
		if hdcScreen == 0 {
			log.Println("fastDictionary: failed to get screen DC")
			return
		}
		defer procReleaseDC.Call(hwndDesk, hdcScreen)

		hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
		if hdcMem == 0 {
			log.Println("fastDictionary: failed to create compatible DC")
			return
		}
		defer procDeleteDC.Call(hdcMem)

		hbm, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, vw, vh)
		if hbm == 0 {
			log.Println("fastDictionary: failed to create compatible bitmap")
			return
		}
		defer procDeleteObject.Call(hbm)

		oldBmp, _, _ := procSelectObject.Call(hdcMem, hbm)
		defer procSelectObject.Call(hdcMem, oldBmp)

		const srccopy = 0x00CC0020
		rBlt, _, errBlt := procBitBlt.Call(hdcMem, 0, 0, vw, vh, hdcScreen, vx, vy, srccopy)
		if rBlt == 0 {
			log.Printf("fastDictionary: BitBlt screen capture failed: %v", errBlt)
			return
		}

		// 4. Create 1x1 black bitmap memory DC for fast AlphaBlend dimming
		hdcDark, _, _ := procCreateCompatibleDC.Call(hdcScreen)
		hbmDark, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, 1, 1)
		oldDarkBmp, _, _ := procSelectObject.Call(hdcDark, hbmDark)
		defer procDeleteDC.Call(hdcDark)
		defer procDeleteObject.Call(hbmDark)
		defer procSelectObject.Call(hdcDark, oldDarkBmp)

		session := &snipSession{
			vx:         int32(vx),
			vy:         int32(vy),
			vw:         int32(vw),
			vh:         int32(vh),
			hdcMem:     hdcMem,
			hbm:        hbm,
			oldBmp:     oldBmp,
			hdcDark:    hdcDark,
			hbmDark:    hbmDark,
			oldDarkBmp: oldDarkBmp,
			onSuccess:  onSuccess,
			onError:    onError,
		}

		snipMu.Lock()
		if !beginSnipSessionLocked(session) {
			snipMu.Unlock()
			log.Println("fastDictionary: a screen snip is already in progress")
			return
		}
		snipMu.Unlock()
		// 早退（建窗失败、GDI 失败、裁剪写盘失败）也要清掉全局单例；
		// 正常路径在消息循环结束后已置 nil，这里的 defer 兜底为 no-op。
		defer func() {
			snipMu.Lock()
			endSnipSessionLocked(session)
			snipMu.Unlock()
		}()

		// 5. Create full-screen topmost popup overlay window
		className, _ := syscall.UTF16PtrFromString("FastDictSnipOverlay")
		const (
			wsPopup        = 0x80000000
			wsVisible      = 0x10000000
			wsExTopmost    = 0x00000008
			wsExToolWindow = 0x00000080
		)

		hwnd, _, _ := procCreateWindowExW.Call(
			wsExTopmost|wsExToolWindow,
			uintptr(unsafe.Pointer(className)),
			0,
			wsPopup|wsVisible,
			vx, vy, vw, vh,
			0, 0, 0, 0,
		)

		if hwnd == 0 {
			log.Println("fastDictionary: cannot create snip overlay window")
			return
		}

		procShowWindow.Call(hwnd, 5) // SW_SHOW
		procUpdateWindow.Call(hwnd)

		// 6. Message loop
		var msg msgW
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		snipMu.Lock()
		s := currentSnip
		currentSnip = nil
		snipMu.Unlock()

		// 7. Crop the selected region if valid
		if s != nil && s.cropDone {
			cropW := s.cropRect.Right - s.cropRect.Left
			cropH := s.cropRect.Bottom - s.cropRect.Top
			srcX := s.cropRect.Left - s.vx
			srcY := s.cropRect.Top - s.vy

			cropHdc, _, _ := procCreateCompatibleDC.Call(hdcScreen)
			cropHbm, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, uintptr(cropW), uintptr(cropH))
			oldCropBmp, _, _ := procSelectObject.Call(cropHdc, cropHbm)

			procBitBlt.Call(cropHdc, 0, 0, uintptr(cropW), uintptr(cropH), s.hdcMem, uintptr(srcX), uintptr(srcY), srccopy)

			// Deselect before calling GetDIBits
			procSelectObject.Call(cropHdc, oldCropBmp)

			bih := bitmapInfoHeader{
				BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
				BiWidth:       cropW,
				BiHeight:      -cropH, // top-down
				BiPlanes:      1,
				BiBitCount:    32,
				BiCompression: 0,
			}
			cropImg := image.NewRGBA(image.Rect(0, 0, int(cropW), int(cropH)))
			procGetDIBits.Call(
				cropHdc, cropHbm, 0, uintptr(cropH),
				uintptr(unsafe.Pointer(&cropImg.Pix[0])),
				uintptr(unsafe.Pointer(&bih)),
				0,
			)

			procDeleteDC.Call(cropHdc)
			procDeleteObject.Call(cropHbm)

			// BGRA -> RGBA swap
			for i := 0; i < len(cropImg.Pix); i += 4 {
				cropImg.Pix[i], cropImg.Pix[i+2] = cropImg.Pix[i+2], cropImg.Pix[i]
				cropImg.Pix[i+3] = 255
			}

			// Every failure path reports back: a user who finished the crop
			// and saw the overlay close with nothing happening had no way to
			// know the cache dir was unwritable.
			cacheDir, dirErr := snipCacheDir()
			if dirErr != nil {
				log.Printf("fastDictionary: snip cache dir: %v", dirErr)
				if s.onError != nil {
					s.onError(dirErr)
				}
				return
			}
			if err := os.MkdirAll(cacheDir, 0o755); err != nil {
				log.Printf("fastDictionary: snip cache mkdir: %v", err)
				if s.onError != nil {
					s.onError(err)
				}
				return
			}
			outPath := filepath.Join(cacheDir, "snip.png")

			f, err := os.Create(outPath)
			if err != nil {
				log.Printf("fastDictionary: snip save: %v", err)
				if s.onError != nil {
					s.onError(err)
				}
				return
			}
			if err := png.Encode(f, cropImg); err != nil {
				f.Close()
				log.Printf("fastDictionary: snip encode: %v", err)
				if s.onError != nil {
					s.onError(err)
				}
				return
			}
			f.Close()
			if s.onSuccess != nil {
				s.onSuccess(s.cropRect, outPath)
			}
		}
	}()
}
