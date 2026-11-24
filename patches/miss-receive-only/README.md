# MISS receive-only audio

`#backchannel=0` keeps receive audio and omits the speaker path on Xiaomi MISS sources, including after reconnect. The default still negotiates supported talkback. The Xiaomi guide tells users to set the option consistently on sources sharing a camera.

This group requires [miss-shared-session](../miss-shared-session/README.md) for the producer and its shared-audio methods. Its producer and user-guide changes apply with or without the channel reconnect fix. [Option and tests](https://github.com/Ricky-Hao/go2rtc/commit/ef312f0840647f2e3160b7ee43183af6b5e98c07) · [PR merge](https://github.com/Ricky-Hao/go2rtc/commit/48ff711437d05852d9e1cce1aa788ed2330c2fc5)

The focused contract checks parsing and the receive-only media list. Before retiring the group, check that the upstream option preserves incoming audio and default negotiation and remains effective across a reconnect. [Checks](tests.json)
