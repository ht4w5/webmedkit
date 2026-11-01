# 08: code comments

**What to build:** Two comment fixes, no behaviour change. (a)
`bandage/internal/domain/report.go:8-9` "TrackerReading is the tracker
values at trigger time, recorded as report evidence" -> the reading is
the report's tracker reading, not its sample (and "evidence" is
avoided). (b) `bandage/internal/infra/cache/memory/memory.go:117`
"restart the bucket below" -> "restart accumulation below" (bare
"bucket" never names a tracker). Per the spec's suppression list, do
NOT touch `firewall.Hit`, `Stats`, `track.Statistics`, `--nft-hook`, or
test names.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent
