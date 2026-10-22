# Maintaining this fork

The fork is based on upstream quic-go `v0.64.1` at commit
`ed90516797ec8ed7314c2b163797eca58fd5a2b8`. Its module identity is
`github.com/oninescom/quic-go`; upstream references in source comments and
documentation URLs remain pointed at the upstream repository.

## Sync a later upstream release

1. Add the upstream remote if needed, fetch its tags, and inspect the new
   release notes and diff from the current upstream base.
2. Merge the selected upstream tag into the fork branch in a review branch.
   Resolve conflicts in the fork's small congestion-control integration
   separately from upstream changes.
3. Run the path rewrite script from the repository root. It rewrites tracked
   text files, preserves UTF-8 BOMs and line endings, skips
   binary/undecodable files, and leaves `http(s)://` source links unchanged.

   ```powershell
   pwsh -File scripts/rewrite-module-path.ps1 -WhatIf
   pwsh -File scripts/rewrite-module-path.ps1
   pwsh -File scripts/rewrite-module-path.ps1 -Check
   ```

4. Review upstream changes separately from the module-path rewrite. Confirm
   that changes to congestion, ACK/loss recovery, pacing, TLS, DATAGRAM, and
   migration code are upstream changes unless explicitly part of the fork
   integration. Keep the BBR implementation in VTP Core.
5. Run `git diff --check`, `go mod tidy`, the fork suite, focused adapter and
   migration tests, race tests, and supported cross-builds. Record the exact
   upstream tag and commit in the sync review.

The script only changes the module path; it does not fetch upstream, resolve
merge conflicts, or apply the congestion-control integration. This keeps each
upgrade review small and auditable without a patch-stack framework.

## Source and license

Keep the upstream `LICENSE`, copyright notices, source headers, and upstream
issue/PR links intact. Add a VTP notice only to VTP-authored files; do not
replace upstream ownership notices.
