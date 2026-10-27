# Relation index

`internal/relations.Index` owns direct relation lookup. The TUI adapts provider
snapshots into documents; it no longer constructs a relation graph. Explicit
links are bidirectional. Jira keys use inverted lists instead of a clique;
repository/branch groups connect sessions to PRs. A URL trie finds PR URL
substrings in messages, preserving the previous substring matching behavior
without scanning every PR for every message.

`Sync` compares the input snapshot and configuration before rebuilding indexes.
It copies document/link slices, so in-place source edits are detected too.
Changes, deletions, custom Jira patterns and Jira server/context changes take
effect on the next relation lookup. Unchanged inputs reuse the index. A changed
snapshot currently rebuilds the index as a whole; updates are not incremental
per entity. Synchronization and queries run on the terminal event loop.

`Related` expands exactly one hop, includes origins of requested providers,
removes deferred-group identities, deduplicates and sorts results. It does not
expand through unrelated tasks. Generated Jira references use fresh provider
metadata when available.

## Reproducing the measurement

```sh
go test ./internal/tui -run '^$' -bench '^BenchmarkRelationCalculation$' -benchtime=1x -count=3 -benchmem
go test ./internal/relations -run '^$' -bench '^BenchmarkRelations$' -benchmem
```

The TUI benchmark retains the pre-refactor implementation in a test-only helper
(`legacyRelatedReferenceKinds`, from main at `9ad3ac6`). Each dataset contains N
PRs, N Copilot sessions and N Mattermost messages, with groups of 20 PRs sharing
a Jira key, unique branches and messages containing PR URLs. Both paths request
the same provider kinds from the same PR. Separate differential tests compare
old/new results, including successive source edits.

Measured on 2026-10-27, linux/amd64, AMD EPYC 9V74, Go 1.27.2. Times below are
medians of three single-iteration samples; allocations are representative
samples. They are synthetic measurements, not production latency guarantees.

| PRs / sessions / messages | Previous calculation | Build index + lookup | Reused index + TUI adaptation | Previous / reused |
| --- | ---: | ---: | ---: | ---: |
| 1,000 each | 67.82 ms | 13.16 ms | 4.94 ms | 13.7x |
| 10,000 each | 4,977.52 ms | 149.40 ms | 49.43 ms | 100.7x |

For 10,000 of each entity, allocated bytes per call fall from about 187 MB to
30.5 MB with a reused index (about 84% less); allocations fall from about
2,644,600 to 171,050 (about 94% fewer). A cold index uses about 80 MB of total
allocations per build, including adaptation and lookup, not retained heap.

The reused TUI path still enumerates/sorts references, prepares documents and
checks for changes. It is not constant-time in the dataset size. The domain-only
benchmark isolates indexed lookup from this adapter cost. A separate 10,000
entity shared-key test verifies that no pairwise Jira adjacency is stored.
