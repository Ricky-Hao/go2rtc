# Xiaomi MISS shared session

Xiaomi MISS substreams can share one physical connection. This patch classifies packets by channel, manages acquisition and shutdown of the shared session, caches cloud-provided MISS credentials, and routes shared receive audio and talkback. The Xiaomi user guide describes channel selection and audio behavior. The implementation plan is retained as history.

The group applies directly to the manifest base. Classification, credential lookup, session lifecycle and audio routing are kept together because the producer, session and their tests share these contracts. [CS2 buffering](../cs2-media-buffer/README.md) can be applied separately. Reconnect and receive-only audio each build on this group.

The net delta is taken through the fork's rework merge. It includes the initial dual-channel implementation, the credential cache and classifier extraction, lifecycle fixes for concurrent acquire and last-stream close, channel and URL validation, and the later shared-audio negotiation and start changes. [Initial dual stream](https://github.com/Ricky-Hao/go2rtc/commit/b713d792df6f17b8d67d1b231557aefe5df73989) · [Credential cache](https://github.com/Ricky-Hao/go2rtc/commit/ae7c4c3c7de5476862b75532ec3443e686d6252e) · [Classifier](https://github.com/Ricky-Hao/go2rtc/commit/90ed39acd9fcbf57a8e6cef0814a027bac4a2fe3) · [Lifecycle](https://github.com/Ricky-Hao/go2rtc/commit/147722f9944e95fa8a0e412f28fe86efc04e75bf) · [Shared audio](https://github.com/Ricky-Hao/go2rtc/commit/70c79d30d4b20c413b3cbc18049ec7ed7171962a) · [Merge](https://github.com/Ricky-Hao/go2rtc/commit/44b79b4855a383f2ea78c2542c08dde1d10ffd55)

The focused contract runs the existing credential cache, classifier, session manager and producer/talkback tests. The project contract exercises related local media and streaming packages. [Checks](tests.json)

On a new upstream base, compare two-channel routing, single-channel fallback for Dafang-like devices, concurrent lifecycle, credential expiry and shared media negotiation with these tests. Keep any missing behavior, and revalidate both dependent groups before retiring any part.
