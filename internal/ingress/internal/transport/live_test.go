package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
)

func TestReadLiveLineBoundsUnterminatedInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("x", domain.MaxLineBytes+1)))
	line, err := readLiveLine(reader)
	if err == nil || !errors.Is(err, domain.ErrLineTooLarge) {
		t.Fatalf("read oversized live line error = %v", err)
	}
	if got, want := len(line), domain.MaxLineBytes; got != want {
		t.Fatalf("oversized live line prefix length = %d, want %d", got, want)
	}
}

func TestOversizedClientFrameKeepsOnlyTheBoundedPrefix(t *testing.T) {
	s := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), connections: make(map[net.Conn]struct{})}
	prefix := bytes.Repeat([]byte("x"), domain.MaxLineBytes)
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	go func() {
		defer func() { _ = client.Close() }()
		_, _ = client.Write(append(append([]byte(nil), prefix...), 'y'))
	}()
	lines := make(chan domain.LiveLine, 1)
	s.readClient(context.Background(), server, 7, lines)
	item := <-lines
	if !errors.Is(item.ReadErr, domain.ErrLineTooLarge) {
		t.Fatalf("read error = %v, want oversized-line error", item.ReadErr)
	}
	if !bytes.Equal(item.Data, prefix) || item.ConnectionID != 7 {
		t.Fatal("the oversized frame must surface only its bounded prefix")
	}
}
