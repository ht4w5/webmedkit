# Size 0 means unknown

An Event's size 0 is the "unknown" marker and nothing else: the Sampling
rule observes only non-empty response sizes and the estimate starts and
stays at 1 or above, so "known empty" is deliberately unrepresentable -
an empty file's size reads as unknown forever (GLOSSARY: Filesize,
Sampling rule). Size Filter predicates follow from that contract:
`size >=` / `size <=` match known sizes only, and `size unknown` selects
the rest (ADRs 0019, 0021).

## Considered Options

- **Represent unknown explicitly** (a -1 sentinel, an optional value, or
  a known flag) so a known size of 0 could exist. Rejected: nothing in
  the domain consumes "known empty" - the replay fraction is undefined
  for empty content - and the change would reinterpret every stored
  `size_bytes: 0` report row still within its TTL while putting the
  replay division and volume math at risk.
