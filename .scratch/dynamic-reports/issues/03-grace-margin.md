# 03: Grace margin on dynamic reports

**What to build:** A rules author adds `grace: <duration>` (Go
duration strings plus `d` for days, like `ttl`) to a dynamic report
action, and the Report's death line widens from `volume` to
`volume − leak × grace`: a brief pause in otherwise sustained abuse no
longer flaps enforcement. `ttl` and `grace` on one report action is a
load error. The validated plan prints `grace=` beside `ttl=`. Absent
`grace` keeps the strict boundary.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A dynamic Report with `grace` stays alive while its value is
      above `volume − leak × grace` and dies at that line (strict).
- [ ] Without `grace`, the strict line is unchanged (dead at
      `value <= volume`).
- [ ] Load refuses a report action carrying both `ttl` and `grace`.
- [ ] `grace` parses the `ttl` quantity language including day
      suffixes; unparseable values fail load naming the key.
- [ ] The plan prints `grace=` on the report line.
- [ ] Anti-flap is verifiable on the daemon seam: drive the value
      below the volume but above the widened line (clock advance),
      assert the Report survives; cross the line, assert death.
