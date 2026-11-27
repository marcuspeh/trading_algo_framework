package tearsheet

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/marcuspeh/trading-framework/order"

	_ "modernc.org/sqlite" // the pure-Go SQLite driver
)

// SQLiteStore keeps the events of a strategy's runs in a SQLite file.
//
// It is a single file on disk, so it needs no server and it survives
// the process that wrote it. That is what a live service wants: it
// restarts, loads its history back and carries on.
//
// The store is safe for concurrent use.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a store at the given path.
//
// The file's parent directory is created if it does not exist. The
// schema is created if it is not there yet.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("tearsheet: store path is required")
	}

	// SQLite will not create the file's directory, and says so in a
	// way that reads like a memory problem.
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("tearsheet: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("tearsheet: %w", err)
	}
	// SQLite takes one writer at a time. The store writes rarely - on
	// a day boundary or when a run stops - so a small pool is enough.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// migrate creates the schema.
func (s *SQLiteStore) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS runs (
    key       TEXT PRIMARY KEY,
    strategy  TEXT NOT NULL,
    mode      TEXT NOT NULL,
    start     TEXT NOT NULL,
    end       TEXT NOT NULL,
    starting  REAL NOT NULL,
    currency  TEXT NOT NULL,
    updated   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_strategy ON runs (strategy, updated DESC);

CREATE TABLE IF NOT EXISTS equity (
    run_key   TEXT NOT NULL,
    seq       INTEGER NOT NULL,
    at        TEXT NOT NULL,
    value     REAL NOT NULL,
    open_lots INTEGER NOT NULL,
    PRIMARY KEY (run_key, seq)
);

CREATE TABLE IF NOT EXISTS fills (
    run_key    TEXT NOT NULL,
    seq        INTEGER NOT NULL,
    at         TEXT NOT NULL,
    symbol     TEXT NOT NULL,
    side       TEXT NOT NULL,
    quantity   REAL NOT NULL,
    price      REAL NOT NULL,
    commission REAL NOT NULL,
    PRIMARY KEY (run_key, seq)
);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("tearsheet: schema: %w", err)
	}
	return nil
}

// Save records the run's events, replacing what was stored under the
// key before.
func (s *SQLiteStore) Save(ctx context.Context, key string, run Run, evts Events) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tearsheet: save: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM equity WHERE run_key = ?`, key); err != nil {
		return fmt.Errorf("tearsheet: save: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fills WHERE run_key = ?`, key); err != nil {
		return fmt.Errorf("tearsheet: save: %w", err)
	}

	for i, p := range evts.Equity {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO equity (run_key, seq, at, value, open_lots) VALUES (?, ?, ?, ?, ?)`,
			key, i, p.Time.UTC().Format(time.RFC3339Nano), p.Value, p.Open)
		if err != nil {
			return fmt.Errorf("tearsheet: save equity: %w", err)
		}
	}
	for i, f := range evts.Fills {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO fills (run_key, seq, at, symbol, side, quantity, price, commission) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			key, i, f.Time.UTC().Format(time.RFC3339Nano), f.Symbol, f.Side.String(), f.Quantity, f.Price, f.Commission)
		if err != nil {
			return fmt.Errorf("tearsheet: save fill: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO runs (key, strategy, mode, start, end, starting, currency, updated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		     strategy = excluded.strategy,
		     mode     = excluded.mode,
		     start    = excluded.start,
		     end      = excluded.end,
		     starting = excluded.starting,
		     currency = excluded.currency,
		     updated  = excluded.updated`,
		key, run.Strategy, run.Mode,
		run.Start.UTC().Format(time.RFC3339Nano),
		run.End.UTC().Format(time.RFC3339Nano),
		run.Starting, run.Currency,
		time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("tearsheet: save run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tearsheet: save: %w", err)
	}
	return nil
}

// Load returns the run and the events stored under the key.
//
// A key that was never saved returns a zero Run and no events.
func (s *SQLiteStore) Load(ctx context.Context, key string) (Run, Events, error) {
	var run Run
	var start, end string
	err := s.db.QueryRowContext(ctx,
		`SELECT strategy, mode, start, end, starting, currency FROM runs WHERE key = ?`, key).
		Scan(&run.Strategy, &run.Mode, &start, &end, &run.Starting, &run.Currency)
	switch {
	case err == sql.ErrNoRows:
		return Run{}, Events{}, nil
	case err != nil:
		return Run{}, Events{}, fmt.Errorf("tearsheet: load run: %w", err)
	}
	run.Start, _ = time.Parse(time.RFC3339Nano, start)
	run.End, _ = time.Parse(time.RFC3339Nano, end)

	var evts Events

	rows, err := s.db.QueryContext(ctx,
		`SELECT at, value, open_lots FROM equity WHERE run_key = ? ORDER BY seq`, key)
	if err != nil {
		return Run{}, Events{}, fmt.Errorf("tearsheet: load equity: %w", err)
	}
	for rows.Next() {
		var at string
		var p Point
		if err := rows.Scan(&at, &p.Value, &p.Open); err != nil {
			rows.Close()
			return Run{}, Events{}, fmt.Errorf("tearsheet: load equity: %w", err)
		}
		p.Time, _ = time.Parse(time.RFC3339Nano, at)
		evts.Equity = append(evts.Equity, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Run{}, Events{}, fmt.Errorf("tearsheet: load equity: %w", err)
	}
	rows.Close()

	frows, err := s.db.QueryContext(ctx,
		`SELECT at, symbol, side, quantity, price, commission FROM fills WHERE run_key = ? ORDER BY seq`, key)
	if err != nil {
		return Run{}, Events{}, fmt.Errorf("tearsheet: load fills: %w", err)
	}
	for frows.Next() {
		var at, side string
		var f order.Fill
		if err := frows.Scan(&at, &f.Symbol, &side, &f.Quantity, &f.Price, &f.Commission); err != nil {
			frows.Close()
			return Run{}, Events{}, fmt.Errorf("tearsheet: load fills: %w", err)
		}
		f.Time, _ = time.Parse(time.RFC3339Nano, at)
		if side == order.Sell.String() {
			f.Side = order.Sell
		} else {
			f.Side = order.Buy
		}
		evts.Fills = append(evts.Fills, f)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return Run{}, Events{}, fmt.Errorf("tearsheet: load fills: %w", err)
	}
	frows.Close()

	return run, evts, nil
}

// Runs lists the runs of a strategy, newest first.
func (s *SQLiteStore) Runs(ctx context.Context, strategy string) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT strategy, mode, start, end, starting, currency
		 FROM runs WHERE strategy = ? ORDER BY updated DESC`, strategy)
	if err != nil {
		return nil, fmt.Errorf("tearsheet: runs: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var run Run
		var start, end string
		if err := rows.Scan(&run.Strategy, &run.Mode, &start, &end, &run.Starting, &run.Currency); err != nil {
			return nil, fmt.Errorf("tearsheet: runs: %w", err)
		}
		run.Start, _ = time.Parse(time.RFC3339Nano, start)
		run.End, _ = time.Parse(time.RFC3339Nano, end)
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tearsheet: runs: %w", err)
	}
	return out, nil
}
