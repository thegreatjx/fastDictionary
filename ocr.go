package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/ocr.ps1
var embeddedOcrScript []byte

// ensureOCRScript writes the WinRT OCR runner to the app cache directory if not present
func ensureOCRScript() (string, error) {
	cacheDir := filepath.Join(os.Getenv("APPDATA"), "Dictionary", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(cacheDir, "winrt_ocr.ps1")
	if err := os.WriteFile(scriptPath, embeddedOcrScript, 0o644); err != nil {
		return "", err
	}
	return scriptPath, nil
}

// RecognizeImageFile extracts text from an image file using Windows Media OCR
func RecognizeImageFile(ctx context.Context, imgPath string) (string, error) {
	scriptPath, err := ensureOCRScript()
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-ImagePath", imgPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ocr execution failed: %v", err)
	}

	raw := string(out)
	start := strings.Index(raw, "EXTRACTED_TEXT_START")
	end := strings.Index(raw, "EXTRACTED_TEXT_END")
	if start >= 0 && end > start {
		extracted := raw[start+len("EXTRACTED_TEXT_START") : end]
		// OCR keeps the image's own line breaks; CleanSelectionText would
		// fold them away (it is for a selection, which is one logical run
		// of text).
		extracted = CleanOcrText(extracted)
		return extracted, nil
	}

	return "", fmt.Errorf("no text detected in image")
}

// TriggerScreenshotOCR launches the native instant screen snipping overlay and translates in-place
func (a *app) TriggerScreenshotOCR() {
	a.hideSelIcon()
	if a.win != nil && !a.pinned {
		a.win.Hide()
	}

	StartNativeSnip(func(cropRect winRect, imgPath string) {
		imgBytes, _ := os.ReadFile(imgPath)
		var bmp *ui.Bitmap
		if len(imgBytes) > 0 {
			bmp, _ = ui.DecodeBitmap(imgBytes)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		text, err := RecognizeImageFile(ctx, imgPath)
		if err != nil || strings.TrimSpace(text) == "" {
			text = "（未在截图中识别到清晰文本，请重新框选）"
		}

		mygo.RunOnMain(func() {
			a.openPinnedScreenshot(text, bmp, cropRect)
		})
	}, func(err error) {
		// The crop could not be saved: tell the user instead of the overlay
		// closing with nothing happening.
		log.Printf("fastpot: screenshot capture failed: %v", err)
		mygo.RunOnMain(func() {
			a.openPinnedScreenshot("截图保存失败："+err.Error(), nil, winRect{})
		})
	})
}

// openPinnedScreenshot places the translation window pinned right beside the original snip without moving
func (a *app) openPinnedScreenshot(text string, bmp *ui.Bitmap, cropRect winRect) {
	cfg := a.store.Get()
	win := a.win
	if win == nil {
		return
	}

	pt := mygo.Point{X: int(cropRect.Right), Y: int(cropRect.Top)}
	d := mygo.Screen.DisplayNearestPoint(pt)
	w := cfg.PopupWidth
	if w < 440 {
		w = 440
	}

	// 1. Position right beside the original snipped text! (右侧并排对照)
	x := int(cropRect.Right) + 14
	y := int(cropRect.Top)

	// If right edge exceeds screen, position on the left side of original text!
	if float32(x)+w+20 > float32(d.WorkArea.X+d.WorkArea.Width) {
		x = int(cropRect.Left) - int(w) - 14
	}
	// If left side also exceeds, position directly beneath the original text!
	if x < d.WorkArea.X {
		x = int(cropRect.Left)
		y = int(cropRect.Bottom) + 14
	}
	if y+420 > d.WorkArea.Y+d.WorkArea.Height {
		y = d.WorkArea.Y + d.WorkArea.Height - 420
	}
	if y < d.WorkArea.Y {
		y = d.WorkArea.Y + 10
	}

	a.mu.Lock()
	a.mode = modeTranslate
	a.pinned = true // 钉在画面上不动！
	a.screenshotPinned = true
	a.isScreenshotMode = true
	a.snipBitmap = bmp
	a.inputText = ""
	a.notice = ""
	a.source = text
	a.target = ""
	a.errText = ""
	a.aiText = ""
	a.dict = nil
	a.gdict = nil
	a.busy = false
	a.aiBusy = false
	a.actionSavedMsg = ""
	a.popupX = x
	a.popupY = y
	a.popupW = int(w)
	a.popupH = 380
	a.mu.Unlock()

	win.SetSize(int(w), 380)
	win.SetPosition(x, y)
	win.SetAlwaysOnTop(true)
	win.Show()
	win.Focus()

	if strings.TrimSpace(text) != "" && !strings.HasPrefix(text, "（未在截图") {
		a.translate(text)
	} else {
		win.Update(func() {})
	}
}
