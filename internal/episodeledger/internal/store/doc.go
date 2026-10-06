// Package store owns every SQL statement on the episode pipeline tables:
// scheduler_items, episodes, episode_attempts and episode_rejections. It also
// reads the runtime owner lease, which belongs to control. It selects and
// writes data and classifies database errors; every decision lives in domain
// and app. Its unit of work is opaque and is always the caller's transaction.
package store
