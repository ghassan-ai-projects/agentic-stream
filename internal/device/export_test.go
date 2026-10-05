package device

// NewUDSTransportForTest wraps an already-connected gateway link for tests that
// own the connection. Production code dials with DialUDSTransport.
var NewUDSTransportForTest = newUDSTransport
