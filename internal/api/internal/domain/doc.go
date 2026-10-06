// Package domain holds the HTTP surface's rules as pure code: the problem
// document, the approval request shape and decoding rules, bearer-token
// matching, the Server-Sent Events frame formats, cursor parsing, duplicate
// suppression, stream timing defaults and the mapping from stream failures to
// problem responses. It performs no I/O and imports no HTTP package.
package domain
