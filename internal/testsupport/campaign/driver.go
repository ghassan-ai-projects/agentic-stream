package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const pollEvery = 500 * time.Millisecond

const worldSpanQuery = `SELECT COALESCE(MIN(event_time), ''), COALESCE(MAX(event_time), '') FROM event_log WHERE ingested_at >= ?1`

// Consumer is one started agentic-stream serve: its database, the plant wire
// host socket a session connects to and the approval desk.
type Consumer struct {
	DB         *sql.DB
	HostSocket string
	Desk       *Desk
	Stop       func() error
}

// Driver drives a campaign arm run by run: it starts the consumer, connects
// the run's session, lets the desk answer, stops the session at the run's
// simulated end, writes the verdict, grades the run and restarts the consumer
// for the next run so a written playbook takes effect.
type Driver struct {
	Campaign  Campaign
	Arm       Arm
	Simulator Simulator
	Dir       string
	Policies  []string
	Serve     func(context.Context, Run) (Consumer, error)
}

// Drive runs the first limit runs of the campaign and writes the report.
func (d Driver) Drive(ctx context.Context, limit int) (Report, error) {
	report := Report{Arm: d.Arm.Name}
	for _, run := range d.Campaign.Runs[:min(limit, len(d.Campaign.Runs))] {
		result, err := d.drive(ctx, run)
		if err != nil {
			return report, fmt.Errorf("run %d: %w", run.Index, err)
		}
		report.Add(result)
	}
	return report, report.Write(d.Dir)
}

func (d Driver) drive(ctx context.Context, run Run) (RunResult, error) {
	since := time.Now()
	consumer, err := d.Serve(ctx, run)
	if err != nil {
		return RunResult{}, err
	}
	result, err := d.play(ctx, run, consumer, since)
	if stopErr := consumer.Stop(); err == nil && stopErr != nil {
		err = fmt.Errorf("stop the consumer: %w", stopErr)
	}
	return result, err
}

func (d Driver) play(ctx context.Context, run Run, consumer Consumer, since time.Time) (RunResult, error) {
	runDir := filepath.Join(d.Dir, fmt.Sprintf("run-%04d", run.Index))
	session, err := d.connect(ctx, run, consumer, runDir)
	if err != nil {
		return RunResult{}, err
	}
	if err := d.await(ctx, run, consumer, since); err != nil {
		_ = session.Stop()
		return RunResult{}, err
	}
	if err := session.Stop(); err != nil {
		return RunResult{}, err
	}
	return d.grade(ctx, run, consumer, runDir, since)
}

func (d Driver) connect(ctx context.Context, run Run, consumer Consumer, runDir string) (*Session, error) {
	if err := os.MkdirAll(d.plansDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create the plans folder: %w", err)
	}
	plan, err := d.Campaign.PreparePlan(run, d.plansDir(), consumer.HostSocket, d.Policies)
	if err != nil {
		return nil, err
	}
	return d.Simulator.Connect(plan, runDir)
}

func (d Driver) plansDir() string { return filepath.Join(d.Dir, "plans") }

func (d Driver) await(ctx context.Context, run Run, consumer Consumer, since time.Time) error {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		done, err := d.tick(ctx, run, consumer, since)
		if err != nil || done {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the run's end: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (d Driver) tick(ctx context.Context, run Run, consumer Consumer, since time.Time) (bool, error) {
	start, now, err := worldSpan(ctx, consumer.DB, since)
	if err != nil || now.IsZero() {
		return false, err
	}
	if _, err := consumer.Desk.Act(ctx, consumer.DB, now); err != nil {
		return false, err
	}
	return now.Sub(start) >= run.Duration(), nil
}

func worldSpan(ctx context.Context, db *sql.DB, since time.Time) (time.Time, time.Time, error) {
	var first, last string
	if err := db.QueryRowContext(ctx, worldSpanQuery, kernel.FormatTime(since)).Scan(&first, &last); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("read the run's world time: %w", err)
	}
	if last == "" {
		return time.Time{}, time.Time{}, nil
	}
	return parseWorldSpan(first, last)
}

func parseWorldSpan(first, last string) (time.Time, time.Time, error) {
	start, err := kernel.ParseTime(first)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("read the run's first event time: %w", err)
	}
	now, err := kernel.ParseTime(last)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("read the run's last event time: %w", err)
	}
	return start, now, nil
}

func (d Driver) grade(ctx context.Context, run Run, consumer Consumer, runDir string, since time.Time) (RunResult, error) {
	if err := WriteVerdict(ctx, consumer.DB, runDir, d.Arm.Name, since); err != nil {
		return RunResult{}, err
	}
	grade, err := d.Simulator.Score(runDir)
	if err != nil {
		return RunResult{}, err
	}
	ledger, err := ReadLedger(ctx, consumer.DB, since)
	return RunResult{Index: run.Index, Phase: run.Phase, Ledger: ledger, Grade: grade}, err
}
