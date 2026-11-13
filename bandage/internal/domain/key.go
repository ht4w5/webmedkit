package domain

import "strings"

// Tracker-cache key convention: state is stored as (namespace, key)
// pairs; namespace is the rule ID, key is Target or Target+KeySep.

// KeySep joins a Target and a path in a per-(Target, path) tracker key.
// NUL occurs in neither a Target nor a path, so the join is unambiguous.
const KeySep = "\x00"

// TrackerKey builds the cache key for Target-keyed tracker state (rate,
// bandwidth): the Target string itself.
func TrackerKey(target string) string {
	return target
}

// ReplayTrackerKey builds the cache key for per-(Target, path) tracker
// state (replay): Target, KeySep and the path.
func ReplayTrackerKey(target, path string) string {
	return target + KeySep + path
}

// SplitTrackerKey splits a tracker cache key into its Target and, for
// per-(Target, path) keys, its path (empty for plain Target keys): the
// inverse of TrackerKey/ReplayTrackerKey.
func SplitTrackerKey(key string) (target, path string) {
	if i := strings.IndexByte(key, 0); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}

// WritingKey returns the tracker-cache state behind this report's
// reading - the Tracker record named in its reading, the key the
// writing event fed (spec: Dynamic report liveness): the rule ID
// namespaces the record, keyed by the Target, or for replay by the
// Target and the sample's path. The sample moves with every write
// (ADR 0021), so it always names the writing key.
func (r Report) WritingKey() (ns, key string) {
	if r.Tracker.Track == string(TrackReplay) {
		return r.Rule, ReplayTrackerKey(r.Target, r.Sample.Path)
	}
	return r.Rule, TrackerKey(r.Target)
}
