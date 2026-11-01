# Live probe for the typing guard.
#
# It shows its OWN scratch window (a WinForms form with a text box) over the
# desktop, types into it with real key events and drags across the text with
# real mouse events. Every synthetic event lands inside the scratch form, which
# is closed at the end; no other window is touched.
#
# Two details make or break this probe:
#   - movement must go through mouse_event(MOUSEEVENTF_MOVE), not SetCursorPos:
#     SetCursorPos warps the cursor without producing low-level hook events, so
#     a drag built on it looks like a press-release with no movement at all;
#   - the two drags end at different points, otherwise the "same spot"
#     cooldown in the gesture gate swallows the second one and the probe
#     proves nothing.
#
# Expected in %APPDATA%\fastDictionary\logs\fastDictionary.log:
#   1. a drag right after typing   -> "gesture dropped (typing: ...)"
#   2. a drag long after typing    -> "gesture at (...) ... kind=drag"
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$sig = @'
using System;
using System.Runtime.InteropServices;
public class Probe {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, int dx, int dy, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint f, IntPtr e);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT p);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [StructLayout(LayoutKind.Sequential)] public struct POINT { public int X; public int Y; }
  public const uint MOVE = 0x0001, LEFTDOWN = 0x0002, LEFTUP = 0x0004;

  public static void TypeAscii(string s) {
    foreach (char c in s) {
      byte vk = (byte)char.ToUpperInvariant(c);
      keybd_event(vk, 0, 0, IntPtr.Zero);
      System.Threading.Thread.Sleep(20);
      keybd_event(vk, 0, 2, IntPtr.Zero);
      System.Threading.Thread.Sleep(40);
    }
  }

  public static void Drag(int x, int y, int dx, int stepMs) {
    SetCursorPos(x, y);
    System.Threading.Thread.Sleep(150);
    mouse_event(LEFTDOWN, 0, 0, 0, IntPtr.Zero);
    System.Threading.Thread.Sleep(140);
    int steps = 12;
    int per = dx / steps;
    for (int i = 0; i < steps; i++) {
      mouse_event(MOVE, per, 0, 0, IntPtr.Zero);
      System.Threading.Thread.Sleep(stepMs);
    }
    System.Threading.Thread.Sleep(100);
    mouse_event(LEFTUP, 0, 0, 0, IntPtr.Zero);
  }
}
'@
Add-Type -TypeDefinition $sig

# The hook reports physical pixels. A DPI-unaware PowerShell would hand the
# form logical pixels, and every synthetic event would land somewhere else.
[Probe]::SetProcessDPIAware() | Out-Null

$p = New-Object Probe+POINT
[Probe]::GetCursorPos([ref]$p) | Out-Null
$script:origX = $p.X; $script:origY = $p.Y

$form = New-Object System.Windows.Forms.Form
$form.Text = 'fastDictionary typing-guard probe'
$form.StartPosition = 'Manual'
$form.Location = New-Object System.Drawing.Point(220, 220)
$form.Width = 760
$form.Height = 420
$form.TopMost = $true

$box = New-Object System.Windows.Forms.TextBox
$box.Multiline = $true
$box.ScrollBars = 'Vertical'
$box.Dock = 'Fill'
$box.Font = New-Object System.Drawing.Font('Consolas', 16)
$box.Text = 'the quick brown fox jumps over the lazy dog'
$form.Controls.Add($box)

$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 1000
$timer.Add_Tick({
    $timer.Stop()
    try {
        [Probe]::SetForegroundWindow($form.Handle) | Out-Null
        $form.Activate()
        $box.Focus()
        [System.Windows.Forms.Application]::DoEvents()
        Start-Sleep -Milliseconds 300

        # A point well inside the text box, in screen coordinates.
        $loc = $box.PointToScreen((New-Object System.Drawing.Point(40, 60)))
        Write-Host "probe form hwnd=$($form.Handle) drag point=$($loc.X),$($loc.Y)"

        Write-Host 'CASE 1: type then drag right away (inside the typing grace)'
        [Probe]::TypeAscii('abc')
        [Probe]::Drag($loc.X, $loc.Y, 168, 14)
        Start-Sleep -Milliseconds 1500

        Write-Host 'CASE 2: drag after a long pause (past the typing grace)'
        [Probe]::Drag($loc.X + 40, $loc.Y + 40, 168, 14)
        Start-Sleep -Milliseconds 1500
    } catch {
        Write-Host "probe error: $_"
    } finally {
        [Probe]::SetCursorPos($script:origX, $script:origY)
        $form.Close()
    }
})
$timer.Start()

[System.Windows.Forms.Application]::Run($form)
Write-Host 'probe finished'
