package app

import "testing"

func TestSafeStopThatMayHaveBeenSentInvalidatesTheLinkAndIsNeverRetried(t *testing.T) {
	t.Parallel()
	transport := &partialSafeStopTransport{}
	session := &Session{
		transport: transport, catalog: thermalCatalog(t), deviceID: "thermal-01", bootID: "boot-A",
		ownerEpoch: "epoch-1", ownerInstance: "instance-1", opened: true, authority: admittedAuthority(t),
	}
	defer func() { _ = session.Close() }()

	if _, sent, err := session.SafeStop(t.Context(), "fan-01"); err == nil || !sent {
		t.Fatalf("partial safe-stop send: sent=%v err=%v, want a sent failure", sent, err)
	}
	if !transport.closed {
		t.Fatal("partial safe-stop send left the link open")
	}
	if _, sent, err := session.SafeStop(t.Context(), "fan-01"); err == nil || sent {
		t.Fatalf("safe-stop retried on an invalidated link: sent=%v err=%v", sent, err)
	}
	if transport.sends != 1 {
		t.Fatalf("safe-stop sends=%d, want 1", transport.sends)
	}
}
