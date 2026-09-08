$ErrorActionPreference = 'Stop'
$architecture = $Env:PROCESSOR_ARCHITECTURE.ToLower()
if ($architecture -notin @('amd64', 'arm64')) {
    throw "Unsupported architecture: $architecture"
}
$binary = "g8e-windows-$architecture.exe"
$temporaryBinary = New-TemporaryFile
$temporaryChecksum = New-TemporaryFile
try {
    Invoke-WebRequest -Uri "https://g8e.ai/$binary" -OutFile $temporaryBinary -UseBasicParsing
    Invoke-WebRequest -Uri "https://g8e.ai/$binary.sha256" -OutFile $temporaryChecksum -UseBasicParsing
    $expected = (Get-Content $temporaryChecksum -Raw).Trim().Split(' ')[0].ToLower()
    $actual = (Get-FileHash $temporaryBinary -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw 'Checksum verification failed' }
    Copy-Item $temporaryBinary ./g8e.exe
    Write-Host 'Installed .\g8e.exe'
} finally {
    Remove-Item $temporaryBinary, $temporaryChecksum -Force -ErrorAction SilentlyContinue
}
