# Xiaomi MISS

Two channels of the same Xiaomi MISS camera can stream over one physical connection. The patch classifies packets by channel, manages the lifecycle of the shared session, caches cloud-provided MISS credentials, and routes shared receive audio and talkback.

A dropped channel can restart while its peer remains active. The session reissues the dual-channel request with the current qualities when both channels are available, or falls back to a single-channel request when the peer has left. Both paths preserve the shared audio settings.

Concurrent sources for the same endpoint share the physical dial. A different camera can dial while the first camera connects or shuts down. Sharing requires compatible credentials, model, transport and per-lens quality. The last successful video-start command determines whether packets need classifying. Each producer times out after ten seconds without video from its lens, even while shared audio continues. Multiple readers can subscribe to either lens.

`#backchannel=0` preserves incoming audio and omits the speaker path, including after reconnect. The default still negotiates supported talkback. The generated `internal/xiaomi/README.md` describes channel selection and how to set the option consistently on sources sharing a camera. The generated `internal/xiaomi/MISS_REFACTOR_IMPLEMENTATION_PLAN.md` preserves the implementation plan.

When video is ready and the receive audio codec is unknown, the probe waits up to one second for audio within its 15-second total budget. It returns as soon as it identifies the codec. On timeout it advertises PCMA at 8 kHz. Opus first observed after fallback requires a newly negotiated receive track. `audio=0` and a cached receive codec both skip the audio wait.

The group applies directly to the manifest base, independently of cloud authentication refresh. The physical connection is closed after a shared read error before the logical readers are notified to reconnect. That ordering carries the upstream camera-slot fix into the shared session. [Upstream close fix](https://github.com/AlexxIT/go2rtc/commit/41ce11e4c0bfedd87824f1464b541845611856cf) · [CS2 overflow replacement](../cs2-media-buffer/README.md) · [Cloud authentication refresh](../xiaomi-cloud-auth/README.md)

## Source history

- The shared-session rework covers dual-channel demux, the credential cache and classifier, concurrent acquisition and last-stream close, channel and URL validation, and shared audio negotiation and start. [Initial dual stream](https://github.com/Ricky-Hao/go2rtc/commit/b713d792df6f17b8d67d1b231557aefe5df73989) · [Credential cache](https://github.com/Ricky-Hao/go2rtc/commit/ae7c4c3c7de5476862b75532ec3443e686d6252e) · [Classifier](https://github.com/Ricky-Hao/go2rtc/commit/90ed39acd9fcbf57a8e6cef0814a027bac4a2fe3) · [Lifecycle](https://github.com/Ricky-Hao/go2rtc/commit/147722f9944e95fa8a0e412f28fe86efc04e75bf) · [Shared audio](https://github.com/Ricky-Hao/go2rtc/commit/70c79d30d4b20c413b3cbc18049ec7ed7171962a) · [Rework merge](https://github.com/Ricky-Hao/go2rtc/commit/44b79b4855a383f2ea78c2542c08dde1d10ffd55)
- PR #1 added dropped-channel reconnect and tests for both peer states. [Fix](https://github.com/Ricky-Hao/go2rtc/commit/ac5b16ba655787373436c1129ac1c3c907f143d5) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/5f2281d5bc86d292f1ae6a252823d52a41d410da)
- PR #3 added the receive-only option, tests and Xiaomi guide changes. [Option and tests](https://github.com/Ricky-Hao/go2rtc/commit/ef312f0840647f2e3160b7ee43183af6b5e98c07) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/48ff711437d05852d9e1cce1aa788ed2330c2fc5) · [Extraction record](../../docs/extraction.md)

## Checks on a new base

The focused contracts run the existing credential cache, classifier, lifecycle, dropped-channel restart, producer and audio tests. The project contract exercises related local media and streaming packages. [Checks](tests.json)

Before replacing the patch, verify two-channel routing, single-channel fallback for Dafang-like devices, concurrent lifecycle, credential expiry and shared media negotiation. Verify restart with and without a peer, and confirm that receive-only audio survives reconnect and that default talkback negotiation still works.
