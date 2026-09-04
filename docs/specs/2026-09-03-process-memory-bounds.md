# API process memory bounds (issue #36)

Date: 2026-09-03

Each EC2 instance runs two Go API processes behind nginx. At service start,
`/opt/app/service-env.sh` reads `MemTotal` and exports half of it as
`GOMEMLIMIT` for each process. This follows the actual selected instance size
(`t4g.nano` or the `t4g.micro` Spot fallback) instead of baking in a value for
one type.

The runtime emits the structured log message `ALARM: process memory pressure`
once per minute while Go-managed resident memory (`MemStats.Sys -
MemStats.HeapReleased`) is at least 85% of that limit. In production, when the
existing `CLOUDWATCH_ALARMS_ENABLED=true` cost switch is explicitly enabled,
a log metric filter alarms on the shared operations SNS topic after one sample.

Table memory is now bounded in two independent ways:

- Every actor, including one holding the latency-only table lease, is stopped
  after five continuous minutes with no WebSocket connections. Teardown also
  releases its lease and its table-scoped global equity entries.
- The process-global equity LRU uses an approximate 4 MiB byte budget rather
  than a 20,000-entry count. Actor estimates carry their table ID so teardown
  can remove only that table's entries.

## RSS-versus-tables measurement and instance-size decision

The opt-in load harness now records Linux process RSS once per second and
prints CSV rows of `elapsed,live_tables,local_actors,rss_mib` with its normal
results. Run the existing controlled soak command at increasing
`LOADTEST_TABLES` values and retain the output with the release evidence; this
measurement is deliberately not part of CI or the normal test suite because it
is resource intensive.

The current capacity decision is to keep `t4g.nano` as the primary Spot type
and `t4g.micro` as the existing capacity fallback. Do not add `t4g.small` based
on actor count alone. Revisit only after the new curve is captured on a
prod-like ARM host; move to `t4g.small` if steady-state combined RSS for both
processes plus nginx leaves less than 20% physical-memory headroom or the
memory-pressure alarm fires under the target live-table load.

No soak was run as part of this change: doing so on the development workstation
would violate the repository owner's explicit CPU/memory constraint and would
not produce an instance-size-valid ARM/EC2 measurement.
