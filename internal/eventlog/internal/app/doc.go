// Package app sequences the event-log use cases: batch append with evidence
// admission in one unit of work, quarantine with bounded retries and hash
// conflict rejection, release and atomic redrive, gap recording and read
// streaming. It reaches SQL only through the store layer's units of work.
package app
