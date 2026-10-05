// Package store holds every SQL statement and transaction of a replay
// session against its isolated database: the episode worklist, version and
// snapshot digests, shadow comparison persistence, spec deployment and
// episode materialization through the owning modules' APIs. Methods are named
// after domain actions and decide nothing.
package store
