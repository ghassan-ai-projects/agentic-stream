package transport

// NewForTest wraps an already-connected gateway link for tests that own the
// connection. Production code uses Dial.
var NewForTest = newUDS
