// Package app orders the ingress use cases: replay a JSON Lines trace or a
// streams-simulator trace into the event log from the last checkpoint, and serve
// a live Unix-socket source. It admits lines through domain rules and the event
// schema, quarantines refusals, and decides through domain and persists through
// store and transport; it holds no SQL and no socket code.
package app
