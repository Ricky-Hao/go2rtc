# CS2 media buffer

The pinned official base includes an overflow fix that keeps the CS2 receive worker running when a queue fills. The control queue on channel 0 holds 10 packets and the media queue on channel 2 holds 100. Either queue drops the incoming packet when full and retains the queued packets. The `upstream` entry records the fix and replays zero patches. [Upstream fix](https://github.com/AlexxIT/go2rtc/commit/6fe6e24b92d5a2bdf7345891792471f20382e08e) · [Pinned CS2 transport](https://github.com/AlexxIT/go2rtc/blob/85495f41d961ad166c5631de32df664b46bb8588/pkg/xiaomi/miss/cs2/conn.go)

The old official base returned a queue-full error that stopped the worker. The fork increased the media queue from 100 to 500, kept the control queue at 10, and dropped the oldest packet when either queue filled. [Original fork change](https://github.com/Ricky-Hao/go2rtc/commit/b231f78c095119e50183dfed5fa793ee629664fc)

The focused contract builds the CS2 package and runs the upstream worker over `net.Pipe`. It fills both queues, verifies which packets are retained, and checks that the same worker acknowledges new frames and accepts data after the queues are drained. The test uses a temporary Go overlay in the test process. [Checks](tests.json) · [Related streaming checks](../xiaomi-miss/README.md)

The replacement was accepted for keeping the connection alive on queue saturation. Picture and audio quality during real dual-channel or high-bitrate streaming remain unverified.
