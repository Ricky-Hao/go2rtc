# Xiaomi cloud authentication refresh

A Xiaomi cloud session cached by account can become unauthorized. A request that receives HTTP 401 retries once using a refreshed session, reusing a refresh already performed by another request. Other errors propagate. Updating the stored user token evicts the cached session.

This group applies directly to the manifest base. It shares `internal/xiaomi/xiaomi.go` with [miss-shared-session](../miss-shared-session/README.md). The payload has two diff sections for that file so the declaration and `getCloud` hunks can both use the blank line they share on the base. The payload includes the tests and the change that exposes the response status. [Cloud fix](https://github.com/Ricky-Hao/go2rtc/commit/83a263bdb4678ae5bf205c97b2145b0599280eeb) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/95a92fc9a36c42799d1f305701d46b4429ec48ed)

Five existing tests cover the retry, session reuse, non-401 errors, retry limit and token change. Require those behaviors from the pinned upstream cloud path before retiring the patch. [Checks](tests.json)
