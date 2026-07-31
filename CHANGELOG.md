# Changelog

## Unreleased

- Add unified core controller API.
- Add SRE adaptive throttling strategy.
- Add pressure-based strategy with time decay.
- Add custom strategy registration for consumers that need their own algorithm.
- Add sequence and key-based samplers.
- Add permit-based and generic `Do[T]` APIs.
- Add observer callbacks and snapshots without required metrics dependencies.
- Add optional HTTP client adapter.
- Add table-driven tests, examples, benchmarks, and CI workflow.
- Make zero-value config normalize to disabled fail-open instead of implicitly enabled defaults.
- Separate actual local rejects from shadow-mode would-rejects.
- Remove unused timeout policy from core config.
- Include pressure-strategy accepts in snapshots.
- Report panic outcomes with a synthetic panic error.
- Add random sampler and copy permit attrs before delayed reports.
- Add expanded benchmarks for attrs, observers, samplers, and baseline primitives.
- Preserve explicit zero config values where zero is meaningful.
- Count direct `Controller.Report` calls as standalone attempts for built-in strategies.
- Document synchronous observer callbacks and experimental custom strategy support.
- Add simulation tests for SRE and pressure strategy behavior.
