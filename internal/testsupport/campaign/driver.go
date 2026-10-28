package campaign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const pollEvery = 500 * time.Millisecond

var errSessionEnded = errors.New("the session ended before the run's end")

const worldGapTolerance = time.Hour

const lastEventQuery = `SELECT COALESCE(MAX(event_time), '') FROM event_log`

// LearnedDatabase is the copy of the database an arm's learning ended with,
// kept in the arm's folder so another arm can play the holdout from the same
// state.
const LearnedDatabase = "learned.db"

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
// for the next run so a written playbook takes effect. With Learned set it
// asks after each learn run whether learning has settled. When learning ends it
// keeps a copy of the database as LearnedDatabase. A run whose planned start
// lies more than an hour past the database's last event starts there instead,
// so skipped runs leave no gap in the world's time.
type Driver struct {
	Campaign  Campaign
	Arm       Arm
	Simulator Simulator
	Dir       string
	Policies  []string
	Serve     func(context.Context, Run) (Consumer, error)
	Learned   func(context.Context, *sql.DB) (bool, error)
}

// Drive runs the first limit runs of the campaign and writes the report. Once
// learning has settled the remaining learn runs are skipped.
func (d Driver) Drive(ctx context.Context, limit int) (Report, error) {
	report := Report{Arm: d.Arm.Name}
	settled := false
	for _, run := range d.Campaign.Runs[:min(limit, len(d.Campaign.Runs))] {
		if settled && run.Phase == learnPhase {
			continue
		}
		result, err := d.drive(ctx, run)
		if err != nil {
			return report, fmt.Errorf("run %d: %w", run.Index, err)
		}
		report.Add(result)
		settled = settled || result.LearningSettled
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
	if err == nil && run.Phase == learnPhase {
		result.LearningSettled, err = d.closeLearning(ctx, run, consumer.DB)
	}
	if stopErr := consumer.Stop(); err == nil && stopErr != nil {
		err = fmt.Errorf("stop the consumer: %w", stopErr)
	}
	return result, err
}

func (d Driver) play(ctx context.Context, run Run, consumer Consumer, since time.Time) (RunResult, error) {
	runDir := filepath.Join(d.Dir, fmt.Sprintf("run-%04d", run.Index))
	session, window, err := d.connect(ctx, run, consumer, runDir)
	if err != nil {
		return RunResult{}, err
	}
	if err := d.await(ctx, run, consumer, session, since); err != nil {
		_ = session.Stop()
		return RunResult{}, err
	}
	if err := session.Stop(); err != nil {
		return RunResult{}, err
	}
	return d.grade(ctx, run, consumer, runDir, since, window)
}

func (d Driver) connect(ctx context.Context, run Run, consumer Consumer, runDir string) (*Session, Window, error) {
	window, err := d.windowOf(ctx, run, consumer.DB)
	if err != nil {
		return nil, Window{}, err
	}
	if err := os.MkdirAll(d.plansDir(), 0o700); err != nil {
		return nil, Window{}, fmt.Errorf("create the plans folder: %w", err)
	}
	plan, err := d.Campaign.PreparePlan(run, d.plansDir(), consumer.HostSocket, d.Policies, window.From)
	if err != nil {
		return nil, Window{}, err
	}
	session, err := d.Simulator.Connect(plan, runDir)
	return session, window, err
}

func (d Driver) windowOf(ctx context.Context, run Run, db *sql.DB) (Window, error) {
	planned, err := d.Campaign.Window(run)
	if err != nil {
		return Window{}, err
	}
	last, err := lastEventTime(ctx, db)
	if err != nil || last.IsZero() || !planned.From.After(last.Add(worldGapTolerance)) {
		return planned, err
	}
	from := last.Truncate(time.Second).Add(time.Second)
	return Window{From: from, Until: from.Add(run.Duration())}, nil
}

func lastEventTime(ctx context.Context, db *sql.DB) (time.Time, error) {
	var last string
	if err := db.QueryRowContext(ctx, lastEventQuery).Scan(&last); err != nil {
		return time.Time{}, fmt.Errorf("read the world's last event time: %w", err)
	}
	if last == "" {
		return time.Time{}, nil
	}
	parsed, err := kernel.ParseTime(last)
	if err != nil {
		return time.Time{}, fmt.Errorf("read the world's last event time: %w", err)
	}
	return parsed, nil
}

func (d Driver) closeLearning(ctx context.Context, run Run, db *sql.DB) (bool, error) {
	settled := false
	if d.Learned != nil {
		var err error
		if settled, err = d.Learned(ctx, db); err != nil {
			return false, err
		}
	}
	if !settled && run.Index != d.Campaign.lastLearnRun() {
		return false, nil
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?1", filepath.Join(d.Dir, LearnedDatabase)); err != nil {
		return settled, fmt.Errorf("keep the database learning ended with: %w", err)
	}
	return settled, nil
}

func (d Driver) plansDir() string { return filepath.Join(d.Dir, "plans") }

func (d Driver) await(ctx context.Context, run Run, consumer Consumer, session *Session, since time.Time) error {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		done, err := d.tick(ctx, run, consumer, since)
		if err != nil || done {
			return err
		}
		if err := nextPoll(ctx, ticker, session); err != nil {
			return d.lastTick(ctx, run, consumer, since, err)
		}
	}
}

func nextPoll(ctx context.Context, ticker *time.Ticker, session *Session) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for the run's end: %w", ctx.Err())
	case <-session.Ended():
		return fmt.Errorf("%w: %s", errSessionEnded, session.output.Bytes())
	case <-ticker.C:
		return nil
	}
}

func (d Driver) lastTick(ctx context.Context, run Run, consumer Consumer, since time.Time, cause error) error {
	if ctx.Err() != nil {
		return cause
	}
	done, err := d.tick(ctx, run, consumer, since)
	if err != nil || !done {
		return errors.Join(cause, err)
	}
	return nil
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

func (d Driver) grade(ctx context.Context, run Run, consumer Consumer, runDir string, since time.Time, window Window) (RunResult, error) {
	if err := WriteVerdict(ctx, consumer.DB, runDir, d.Arm.Name, window); err != nil {
		return RunResult{}, err
	}
	grade, err := d.Simulator.Score(runDir)
	if err != nil {
		return RunResult{}, err
	}
	ledger, err := ReadLedger(ctx, consumer.DB, since)
	return RunResult{Index: run.Index, Phase: run.Phase, Ledger: ledger, Grade: grade}, err
}
