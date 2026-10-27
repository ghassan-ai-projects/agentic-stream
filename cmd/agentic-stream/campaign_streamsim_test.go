package main

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	campaignDirVariable    = "CAMPAIGN_DIR"
	campaignArmVariable    = "CAMPAIGN_ARM"
	campaignRunsVariable   = "CAMPAIGN_RUNS"
	campaignOutVariable    = "CAMPAIGN_OUT"
	campaignReplayVariable = "CAMPAIGN_REPLAY"
	campaignTamozVariable  = "CAMPAIGN_TAMOZ_SOCKET"
	campaignApprovalToken  = "campaign-approval-token"
	campaignRelay          = "campaign-relay"
	campaignApprover       = "campaign-approver"
)

var pumpStationCampaignData = filepath.Join("..", "..", "examples", "pump-station", "testdata", "campaign")

type campaignSetup struct {
	campaign campaign.Campaign
	arm      campaign.Arm
	runs     int
	out      string
	db       string
	sockets  string
	playbook string
}

func TestCampaignDrivesTheConsumerThroughItsRuns(t *testing.T) {
	binary, repository := streamsimNode(t)
	setup := campaignUnderTest(t)
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", campaignApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", campaignRelay)
	serveCampaignWorker(t, setup)
	driver := campaign.Driver{Campaign: setup.campaign, Arm: setup.arm, Simulator: campaign.Simulator{Binary: binary, Repository: repository},
		Dir: setup.out, Policies: []string{pumpStationPolicyDigest(t)}, Serve: setup.serve(t)}
	report, err := driver.Drive(t.Context(), setup.runs)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != setup.runs {
		t.Fatalf("%d runs graded, want %d", len(report.Runs), setup.runs)
	}
	t.Logf("arm %s: %d runs graded; the campaign score is in %s", setup.arm.Name, len(report.Runs), setup.out)
}

func campaignUnderTest(t *testing.T) campaignSetup {
	t.Helper()
	dir := os.Getenv(campaignDirVariable)
	if dir == "" {
		t.Skipf("%s is not set: the campaign driver needs a campaign folder from streamsim suite campaign", campaignDirVariable)
	}
	loaded, err := campaign.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	arm, err := campaign.ArmNamed(cmp.Or(os.Getenv(campaignArmVariable), "B"))
	if err != nil {
		t.Fatal(err)
	}
	setup := campaignSetup{campaign: loaded, arm: arm, runs: campaignRuns(t), out: campaignOutput(t), sockets: privateSocketDir(t)}
	setup.db = filepath.Join(setup.out, "runtime.db")
	seedMigratedDatabase(t, setup.db)
	applyCampaignPrincipals(t, setup.db)
	setup.playbook = campaignPlaybook(t, arm, setup.out)
	return setup
}

func campaignRuns(t *testing.T) int {
	t.Helper()
	runs, err := strconv.Atoi(cmp.Or(os.Getenv(campaignRunsVariable), "1"))
	if err != nil || runs < 1 {
		t.Fatalf("%s=%q: want a positive number of runs", campaignRunsVariable, os.Getenv(campaignRunsVariable))
	}
	return runs
}

func campaignOutput(t *testing.T) string {
	t.Helper()
	out := os.Getenv(campaignOutVariable)
	if out == "" {
		return t.TempDir()
	}
	parent, err := os.OpenRoot(filepath.Dir(out))
	if err != nil {
		t.Fatalf("%s=%s: %v", campaignOutVariable, out, err)
	}
	defer func() { _ = parent.Close() }()
	if err := parent.Mkdir(filepath.Base(out), 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		t.Fatalf("%s=%s: %v", campaignOutVariable, out, err)
	}
	return out
}

func applyCampaignPrincipals(t *testing.T, db string) {
	t.Helper()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"principals", "apply", "--db", db, "--file", filepath.Join(pumpStationCampaignData, "principals.yaml")})
	cmd.SetOut(io.Discard)
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("apply the campaign principals: %v", err)
	}
}

func campaignPlaybook(t *testing.T, arm campaign.Arm, out string) string {
	t.Helper()
	source := map[bool]string{true: "static.playbook.yaml", false: "learning.playbook.yaml"}[arm.Static]
	if !arm.Static && !arm.Learning {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(pumpStationCampaignData, source))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("site-a.playbook.yaml", data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "site-a.playbook.yaml")
}

func serveCampaignWorker(t *testing.T, setup campaignSetup) {
	t.Helper()
	recording := campaign.Record(filepath.Join(setup.out, "responses.jsonl"))
	if replay := os.Getenv(campaignReplayVariable); replay != "" {
		var err error
		if recording, err = campaign.Replay(replay); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := worker.ListenEvidenceSocket(filepath.Join(setup.sockets, "worker.sock"))
	if err != nil {
		t.Fatalf("listen worker socket: %v", err)
	}
	server := grpc.NewServer()
	worker := &campaign.Worker{Recording: recording, Abstain: setup.arm.Static, Upstream: campaignTamoz(t, setup.arm)}
	runtimev1.RegisterEpisodeWorkerServer(server, worker.Server(setup.arm.Learning))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

func campaignTamoz(t *testing.T, arm campaign.Arm) runtimev1.EpisodeWorkerClient {
	t.Helper()
	socket := os.Getenv(campaignTamozVariable)
	if socket == "" || arm.Static {
		return nil
	}
	conn, err := workerfake.DialEvidenceSocket(t.Context(), socket)
	if err != nil {
		t.Fatalf("%s=%s: %v", campaignTamozVariable, socket, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return runtimev1.NewEpisodeWorkerClient(conn)
}

func (s campaignSetup) serve(t *testing.T) func(context.Context, campaign.Run) (campaign.Consumer, error) {
	return func(_ context.Context, run campaign.Run) (campaign.Consumer, error) {
		address := freeLoopbackAddress(t)
		serving := plantWireServe{dir: s.sockets, socket: filepath.Join(s.sockets, fmt.Sprintf("host-%d.sock", run.Index)), db: s.db}
		stop := servePlantWire(t, serving, address, pumpStationSpec, filepath.Join(pumpStationPlantWire, kindsOf(run.Domain)),
			filepath.Join(pumpStationPlantWire, "pump-station.site.json"), s.serveFlags()...)
		desk := &campaign.Desk{Approver: s.arm.Approver, Relay: campaign.Relay{BaseURL: "http://" + address, Token: campaignApprovalToken,
			Approver: campaignApprover, Key: ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), Client: http.DefaultClient}}
		return campaign.Consumer{DB: openReadOnly(t, s.db), HostSocket: serving.socket, Desk: desk, Stop: func() error { stop(); return nil }}, nil
	}
}

func (s campaignSetup) serveFlags() []string {
	flags := []string{"--worker-socket", filepath.Join(s.sockets, "worker.sock"), "--worker-name", "tamoz"}
	if s.playbook != "" {
		flags = append(flags, "--playbook", s.playbook)
	}
	return flags
}

func kindsOf(domain string) string {
	if domain == "pump-station-probe-b" {
		return "kinds-probe-b"
	}
	return "kinds"
}

func pumpStationPolicyDigest(t *testing.T) string {
	t.Helper()
	digest, err := policy.DigestForVersion(compilePumpStation(t).Digest)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
