# Durable reports, volatile trackers

Report rows persist in sqlite; tracker (leaky-bucket) state is memory-only
and restart-cold. The asymmetry is deliberate: reports are the product
(operators query and delete them; they must survive restarts), while
tracker state is disposable - leaky buckets self-heal toward zero and
re-warm quickly. The `TrackerCache` port is the seam where a write-back
persistence layer can be added later without touching the trackers.

Consequences: a restart is a free pass for abusers (all buckets empty,
file-size EWMA re-warms after one observation); accepted for v0.

Amended (dynamic reports, ADR 0031): reports survive restarts only while fixed -
the daemon sweeps every dynamic report at startup, so no Report outlives
the volatile tracker state that justifies it. The free pass extends to
dynamic reports by construction: re-flagging after a restart requires
re-accumulation past volume from an empty bucket, so a burst-then-pause
offender gains amnesty from a restart during the pause and a crash loop
is an amnesty machine. Accepted because restarts are seldom; if
practice shows crash loops matter, the mitigation is seeding trackers
from report snapshots, not more report state.
