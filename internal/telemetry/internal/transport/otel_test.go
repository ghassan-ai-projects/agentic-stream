package transport

import (
	"context"
	"errors"
	"testing"
)

func TestConfigureInstallsAProviderAndHelpersTolerateInvalidInput(t *testing.T) {
	provider, err := Configure(t.Context(), "svc", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provider.Shutdown(context.Background()) }()
	_, span := StartSpan(t.Context(), "op")
	if !AddLinkFromW3C(span, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "") {
		t.Fatal("valid traceparent was not linked")
	}
	if AddLinkFromW3C(span, "garbage", "") {
		t.Fatal("invalid traceparent was linked")
	}
	RecordError(span, errors.New("boom"))
	span.End()
}
