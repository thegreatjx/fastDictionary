param([string]$ImagePath)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.BitmapDecoder, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null
[Windows.Storage.StorageFile, Windows.Foundation, ContentType = WindowsRuntime] | Out-Null

$asTaskGeneric = [System.WindowsRuntimeSystemExtensions].GetMethods() | ? { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' } | Select -First 1

function Await-Async($asyncOp, $type) {
  $method = $asTaskGeneric.MakeGenericMethod($type)
  $task = $method.Invoke($null, @($asyncOp))
  $task.Wait()
  return $task.Result
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

  $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
  if (-not $engine) {
    Write-Error "No OCR engine available"
    exit 1
  }
  $ocrOp = $engine.RecognizeAsync($bitmap)
  $result = Await-Async $ocrOp ([Windows.Media.Ocr.OcrResult])

  Write-Output "EXTRACTED_TEXT_START"
  Write-Output $result.Text
  Write-Output "EXTRACTED_TEXT_END"
}
