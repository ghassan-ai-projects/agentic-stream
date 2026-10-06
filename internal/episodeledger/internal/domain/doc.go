// Package domain holds the episode pipeline vocabulary and rules as pure
// functions: scheduler item and episode statuses, worker identity and fence
// checks over values the store has read, the attempt transition table,
// rejection reasons and ids, admission defaults and the recovery terminal. It
// performs no I/O and reads no clock; time arrives as a parameter.
package domain
