# 02: The sample loses its response-header fields

**What to build:** The report sample's wire form is the whole Event, so
removing the Content-Length and Content-Range total fields from the
Event removes the `content_length` and `content_range_total` keys from
the wire contract - everywhere the contract is pinned: the canonical
report example, the consumer-side sample mirror, the stored-sample
goldens and the bench corpus. The `size_bytes` key survives with its
redefined served-size meaning. Stored documents that still carry the
removed keys keep parsing; unknown keys are tolerated by construction.

**Blocked by:** 01 - Served-size Filesize from the parser (the deleted
enrichment is the fields' last consumer)

**Status:** ready-for-agent

- [ ] The Event carries no Content-Length or Content-Range total
      fields; the sample wire form renders only the remaining keys
- [ ] The canonical report example and its reproduction pins show the
      smaller sample and pass
- [ ] The consumer-side sample mirror matches the contract key for key
- [ ] Stored-sample goldens and the bench corpus reflect the smaller
      sample
- [ ] `size_bytes` survives with the served-size meaning
- [ ] A stored document carrying the removed keys still parses

## Comments
