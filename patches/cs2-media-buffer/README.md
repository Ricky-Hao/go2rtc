# CS2 media buffer

CS2 keeps more incoming 0 and 2 channel frames queued so video or audio is less likely to stall on a burst. This is a transport change and applies directly to the manifest base.

The payload is the final `conn.go` delta from the fork. The buffer change came from [`b231f78`](https://github.com/Ricky-Hao/go2rtc/commit/b231f78c095119e50183dfed5fa793ee629664fc).

`tests.json` builds `./pkg/xiaomi/miss/cs2`. The source snapshot has no CS2 package tests. The full stack's regression tests live in [miss-shared-session](../miss-shared-session/README.md).

Review the queue sizes and delivery behavior under paired media bursts before dropping this delta on a new upstream base. A successful compile establishes compatibility only.
