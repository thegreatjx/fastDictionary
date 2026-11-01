param([string]$ImagePath, [string]$Language = "auto")

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.BitmapDecoder, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null
[Windows.Storage.StorageFile, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null
[Windows.Globalization.Language, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null

$asTaskGeneric = [System.WindowsRuntimeSystemExtensions].GetMethods() | ? { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' } | Select -First 1

function Await-Async($asyncOp, $type) {
  $method = $asTaskGeneric.MakeGenericMethod($type)
  $task = $method.Invoke($null, @($asyncOp))
  $task.Wait()
  return $task.Result
}

# Pick the recognizer for the requested language.
#
# TryCreateFromUserProfileLanguages() takes whichever OCR language the user's
# Windows language list happens to put first, so an English screenshot on a
# Chinese Windows is read by the Chinese model and comes back with wrong
# glyphs. Asking for the language explicitly is the fix; "auto" keeps the old
# behaviour for anyone who has not chosen.
#
# The tag is a BCP-47 language tag, and the engine only accepts a tag that
# matches an installed OCR recognizer exactly enough for the OS to resolve it,
# so a missing pack falls back to the profile list rather than failing.
function New-OcrEngine([string]$tag) {
  if (-not $tag -or $tag -eq "auto") {
    return [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
  }
  try {
    $lang = [Windows.Globalization.Language]::new($tag)
    $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang)
    if ($engine) { return $engine }
  } catch {
    # Unknown tag or no pack installed: fall through to the profile list.
  }
  return [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
}

if ($ImagePath -and (Test-Path $ImagePath)) {
  $abs = (Resolve-Path $ImagePath).Path
  $fileOp = [Windows.Storage.StorageFile]::GetFileFromPathAsync($abs)
  $file = Await-Async $fileOp ([Windows.Storage.StorageFile])

  $streamOp = $file.OpenAsync([Windows.Storage.FileAccessMode]::Read)
  $stream = Await-Async $streamOp ([Windows.Storage.Streams.IRandomAccessStream])

  $decoderOp = [Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)
  $decoder = Await-Async $decoderOp ([Windows.Graphics.Imaging.BitmapDecoder])

  $bitmapOp = $decoder.GetSoftwareBitmapAsync()
  $bitmap = Await-Async $bitmapOp ([Windows.Graphics.Imaging.SoftwareBitmap])

  $engine = New-OcrEngine $Language
  if (-not $engine) {
    throw "No OCR engine is available. Install an OCR language in Windows Settings > Time & language > Language & region."
  }
  $ocrOp = $engine.RecognizeAsync($bitmap)
  $result = Await-Async $ocrOp ([Windows.Media.Ocr.OcrResult])

  Write-Output "EXTRACTED_TEXT_START"
  Write-Output $result.Text
  Write-Output "EXTRACTED_TEXT_END"
  Write-Output "OCR_LANGUAGE: $($engine.RecognizerLanguage.LanguageTag)"
}
