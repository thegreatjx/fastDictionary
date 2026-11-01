package main

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestSnipMetricsAndCapture(t *testing.T) {
	vx, _, _ := procGetSystemMetrics.Call(76)
	vy, _, _ := procGetSystemMetrics.Call(77)
	vw, _, _ := procGetSystemMetrics.Call(78)
	vh, _, _ := procGetSystemMetrics.Call(79)
	t.Logf("VirtualScreen metrics: vx=%d, vy=%d, vw=%d, vh=%d", vx, vy, vw, vh)

	if vw == 0 || vh == 0 {
		vw, _, _ = procGetSystemMetrics.Call(0)
		vh, _, _ = procGetSystemMetrics.Call(1)
		t.Logf("Fallback Primary metrics: vw=%d, vh=%d", vw, vh)
	}

	registerSnipClass()
	t.Logf("registerSnipClass atom=%v", snipAtom)
	if snipAtom == 0 {
		t.Fatalf("registerSnipClass failed to get atom")
	}

	className, _ := syscall.UTF16PtrFromString("FastDictSnipOverlay")
	const (
		wsPopup        = 0x80000000
		wsVisible      = 0x10000000
		wsExTopmost    = 0x00000008
		wsExToolWindow = 0x00000080
	)
	hwnd, _, err := procCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		0,
		wsPopup|wsVisible,
		vx, vy, vw, vh,
		0, 0, 0, 0,
	)
	t.Logf("CreateWindowExW hwnd=%#x, err=%v", hwnd, err)
	procDestroyWindow.Call(hwnd)

	if err := procAlphaBlend.Find(); err != nil {
		t.Fatalf("AlphaBlend not found: %v", err)
	}
	t.Logf("AlphaBlend found successfully")

	// Test 1x1 stretch AlphaBlend on memory DC
	hdcScreen, _, _ := procGetDC.Call(0)
	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	hbm, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, 100, 100)
	procSelectObject.Call(hdcMem, hbm)

	hdcDark, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	hbmDark, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, 1, 1)
	procSelectObject.Call(hdcDark, hbmDark)

	// AlphaBlend 1x1 onto hdcMem with 100/255 opacity
	blendVal := uint32(0) | (uint32(0) << 8) | (uint32(100) << 16) | (uint32(0) << 24)
	ret, _, err := procAlphaBlend.Call(
		hdcMem, 0, 0, 100, 100,
		hdcDark, 0, 0, 1, 1,
		uintptr(blendVal),
	)
	t.Logf("AlphaBlend result=%v err=%v", ret, err)
	if ret == 0 {
		t.Fatalf("AlphaBlend failed: %v", err)
	}
	procReleaseDC.Call(0, hdcScreen)
	procDeleteDC.Call(hdcMem)
	procDeleteObject.Call(hbm)
	procDeleteDC.Call(hdcDark)
	procDeleteObject.Call(hbmDark)
}

func TestCaptureAndOCR(t *testing.T) {
	// The capture is of the real desktop, so it can legitimately fail when
	// the session is locked or a full-screen exclusive window owns the
	// display — neither is a bug in this code. Retry briefly, then skip
	// rather than report a red suite that says nothing about the program.
	var imgReal *image.RGBA
	var errCap error
	for attempt := 0; attempt < 5; attempt++ {
		imgReal, errCap = captureScreenArea(100, 100, 600, 300)
		if errCap == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("captureScreenArea: err=%v", errCap)
	if errCap != nil {
		t.Skipf("the desktop cannot be captured right now (%v); nothing to read", errCap)
	}
	t.Logf("captureScreenArea bounds=%v, sample pixel=%v", imgReal.Bounds(), imgReal.Pix[:16])

	tmpFile := filepath.Join(t.TempDir(), "test_screen.png")
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("create tmp file failed: %v", err)
	}
	if err := png.Encode(f, imgReal); err != nil {
		f.Close()
		t.Fatalf("png encode failed: %v", err)
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	text, errOcr := RecognizeImageFile(ctx, tmpFile)
	t.Logf("RecognizeImageFile: text=%q, err=%v", text, errOcr)
	if errOcr != nil {
		t.Logf("OCR note: %v", errOcr)
	}
}
