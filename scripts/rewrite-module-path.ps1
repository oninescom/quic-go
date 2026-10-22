[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$Root = (Split-Path -Parent $PSScriptRoot),
    [switch]$Check
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$rootPath = (Resolve-Path -LiteralPath $Root).Path
$gitRoot = & git -C $rootPath rev-parse --show-toplevel
if ($LASTEXITCODE -ne 0) {
    throw "Not a Git worktree: $rootPath"
}
$gitRoot = (Resolve-Path -LiteralPath $gitRoot.Trim()).Path
if (-not [string]::Equals($gitRoot, $rootPath, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Root must be the Git worktree root. Root: $rootPath; Git root: $gitRoot"
}

$upstreamPath = 'github.com/quic-go/quic-go'
$forkPath = 'github.com/oninescom/quic-go'
$scriptFullPath = [System.IO.Path]::GetFullPath($MyInvocation.MyCommand.Path)
$paths = & git -C $rootPath ls-files --cached
if ($LASTEXITCODE -ne 0) {
    throw 'Could not enumerate repository files with Git.'
}

$utf8Strict = [System.Text.UTF8Encoding]::new($false, $true)
$utf8Write = [System.Text.UTF8Encoding]::new($false)
$pattern = [System.Text.RegularExpressions.Regex]::new([System.Text.RegularExpressions.Regex]::Escape($upstreamPath))
$stalePaths = [System.Collections.Generic.List[string]]::new()
$rewrittenCount = 0

foreach ($relativePath in $paths) {
    if ([string]::IsNullOrWhiteSpace($relativePath)) {
        continue
    }

    $filePath = Join-Path $rootPath $relativePath
    if (-not (Test-Path -LiteralPath $filePath -PathType Leaf)) {
        continue
    }
    if ([string]::Equals([System.IO.Path]::GetFullPath($filePath), $scriptFullPath, [StringComparison]::OrdinalIgnoreCase)) {
        continue
    }

    $bytes = [System.IO.File]::ReadAllBytes($filePath)
    if ($bytes -contains [byte]0) {
        continue
    }

    $hasUtf8Bom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    $offset = if ($hasUtf8Bom) { 3 } else { 0 }
    try {
        $content = $utf8Strict.GetString($bytes, $offset, $bytes.Length - $offset)
    } catch [System.Text.DecoderFallbackException] {
        continue
    }

    $rewritten = $pattern.Replace($content, [System.Text.RegularExpressions.MatchEvaluator]{
        param($match)

        $lineStart = 0
        if ($match.Index -gt 0) {
            $previousNewline = $content.LastIndexOf("`n", $match.Index - 1)
            if ($previousNewline -ge 0) {
                $lineStart = $previousNewline + 1
            }
        }
        $prefix = $content.Substring($lineStart, $match.Index - $lineStart)
        if ($prefix.EndsWith('https://', [StringComparison]::OrdinalIgnoreCase) -or
            $prefix.EndsWith('http://', [StringComparison]::OrdinalIgnoreCase)) {
            return $match.Value
        }
        return $forkPath
    })

    if ([string]::Equals($content, $rewritten, [StringComparison]::Ordinal)) {
        continue
    }

    if ($Check) {
        $stalePaths.Add($relativePath)
        continue
    }

    if ($PSCmdlet.ShouldProcess($relativePath, "Rewrite $upstreamPath to $forkPath")) {
        $rewrittenBytes = $utf8Write.GetBytes($rewritten)
        if ($hasUtf8Bom) {
            $withBom = [byte[]]::new($rewrittenBytes.Length + 3)
            $withBom[0] = 0xEF
            $withBom[1] = 0xBB
            $withBom[2] = 0xBF
            [System.Array]::Copy($rewrittenBytes, 0, $withBom, 3, $rewrittenBytes.Length)
            $rewrittenBytes = $withBom
        }
        [System.IO.File]::WriteAllBytes($filePath, $rewrittenBytes)
        Write-Output "Rewrote $relativePath"
        $rewrittenCount++
    }
}

if ($Check) {
    if ($stalePaths.Count -gt 0) {
        $stalePaths | ForEach-Object { Write-Error "Upstream module path remains in $_" }
        exit 1
    }
    Write-Output 'No upstream module paths remain outside external URLs.'
} elseif ($rewrittenCount -eq 0) {
    Write-Output 'No files needed rewriting.'
} else {
    Write-Output "Rewrote $rewrittenCount file(s)."
}
