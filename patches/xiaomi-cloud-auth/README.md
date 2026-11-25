# Xiaomi cloud authentication refresh

A Xiaomi cloud session cached by account can become unauthorized. A request that receives HTTP 401 retries once using a refreshed session, reusing a refresh already performed by another request. Other errors propagate. Updating the stored user token evicts the cached session.

This group applies directly to the manifest base and follows Xiaomi MISS in the full order. The groups share `internal/xiaomi/xiaomi.go`. MISS keeps its credential type in `miss_cred_cache.go`, so the cloud hunks have the same surrounding lines on the base and on the MISS tree. The payload includes the tests and the change that exposes the response status. [Xiaomi MISS](../xiaomi-miss/README.md) · [Cloud fix](https://github.com/Ricky-Hao/go2rtc/commit/83a263bdb4678ae5bf205c97b2145b0599280eeb) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/95a92fc9a36c42799d1f305701d46b4429ec48ed)

Each account owns its in-flight login. Updating its token lets new requests use the replacement immediately. Completion of the previous login can only wake its own waiters. A failed login leaves the next request free to try again. Other accounts can log in independently.

The focused tests exercise concurrent 401 responses, token changes during login, recovery after refresh failure and the single retry limit. A loopback HTTP server checks 401, 403 and a cloud API error returned with HTTP 200. [Checks](tests.json)
