# Legacy Docker job guards

The generated application carries the original fork workflow for source-tree fidelity. Three Docker jobs in its `build.yml` have explicit `false` conditions. The active workflow of the patch checkout builds binaries from the generated tree. See the workflow mapping in the [extraction record](../../docs/extraction.md).

This group applies directly to the manifest base and preserves the single workflow delta from [`f7f939b`](https://github.com/Ricky-Hao/go2rtc/commit/f7f939b25301b36495905539691c79da30143892).

The contract reads the generated workflow and checks the three guards. Retire the group on a base that keeps the legacy Docker jobs inactive, or after an explicit maintenance decision to stop preserving that source snapshot file. [Checks](tests.json)
