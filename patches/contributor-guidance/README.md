# Application contributor instructions

The generated checkout gets its own `AGENTS.md` with Go formatting, package tests, a binary build and the handoff back to the patch repository. The patch checkout's root rules cover work on payloads and the manifest.

This migration-local addition applies directly to the manifest base. The layout comes from the [LibreChat contributor-guidance group](https://github.com/Ricky-Hao/LibreChat/tree/fff88e3c323a5d11b0e1e3e9a9ec8ef2abb88e44/patches/contributor-guidance). The content follows the go2rtc build and test entry points.

The contract checks the generated guide and its diff whitespace. If a future upstream base introduces its own guidance, preserve those upstream rules and keep only the missing patch-maintenance instructions. [Checks](tests.json)
