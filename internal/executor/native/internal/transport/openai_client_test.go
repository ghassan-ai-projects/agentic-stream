package transport

import "testing"

func TestProviderWithoutAClientUsesTheBoundedDefault(t *testing.T) {
	t.Parallel()
	client, err := (&OpenAICompatibleProvider{}).boundedHTTPClient()
	if err != nil || client != defaultOpenAIHTTPClient || client.Timeout <= 0 {
		t.Fatalf("boundedHTTPClient() = %+v, %v; want the default client with a positive timeout", client, err)
	}
}
