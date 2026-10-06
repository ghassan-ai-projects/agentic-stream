// Package domain holds the episode rules that are pure over values: budget
// parsing, snapshot evidence validation, attempt failure classification and
// the decision digest and validation-failure documents. Time arrives as a
// parameter; there is no I/O, no clock read and no SQL.
package domain
