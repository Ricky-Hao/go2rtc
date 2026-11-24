# MISS channel reconnect

A dropped channel can restart while its peer remains active. The session reissues the dual-channel media request using the current qualities when both channels are available. It falls back to a single-channel request after the peer leaves and keeps the shared audio settings.

This fix requires [miss-shared-session](../miss-shared-session/README.md) for the shared session state and media start paths. The payload is the net two-file change from the follow-up PR and includes its tests. [Fix](https://github.com/Ricky-Hao/go2rtc/commit/ac5b16ba655787373436c1129ac1c3c907f143d5) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/5f2281d5bc86d292f1ae6a252823d52a41d410da)

The contract runs both existing restart cases, with the peer present and after it has left. Retire this fix once the selected upstream session satisfies both cases through its normal media-start path. [Checks](tests.json)
