package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockRow implements pgx.Row with a configurable scan function.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error { return r.scanFn(dest...) }

func errRow(err error) pgx.Row {
	return &mockRow{scanFn: func(dest ...any) error { return err }}
}

// mockRows implements pgx.Rows for Query results.
type mockRows struct {
	scanFns []func(dest ...any) error
	pos     int
	err     error
}

func (r *mockRows) Close()                                       {}
func (r *mockRows) Err() error                                   { return r.err }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                             { return nil }

func (r *mockRows) Next() bool {
	return r.pos < len(r.scanFns)
}

func (r *mockRows) Scan(dest ...any) error {
	if r.pos >= len(r.scanFns) {
		return fmt.Errorf("no more rows")
	}
	fn := r.scanFns[r.pos]
	r.pos++
	return fn(dest...)
}

// mockDBTX implements db.DBTX by matching SQL query strings to handlers.
type mockDBTX struct {
	queryRowFns map[string]func(args ...interface{}) pgx.Row
	queryFns    map[string]func(args ...interface{}) (pgx.Rows, error)
	execFns     map[string]func(args ...interface{}) (pgconn.CommandTag, error)
}

func newMockDBTX() *mockDBTX {
	return &mockDBTX{
		queryRowFns: make(map[string]func(args ...interface{}) pgx.Row),
		queryFns:    make(map[string]func(args ...interface{}) (pgx.Rows, error)),
		execFns:     make(map[string]func(args ...interface{}) (pgconn.CommandTag, error)),
	}
}

func (m *mockDBTX) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	for pattern, fn := range m.queryRowFns {
		if strings.Contains(sql, pattern) {
			return fn(args...)
		}
	}
	return errRow(fmt.Errorf("no mock for QueryRow: %s", sql[:min(60, len(sql))]))
}

func (m *mockDBTX) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	for pattern, fn := range m.queryFns {
		if strings.Contains(sql, pattern) {
			return fn(args...)
		}
	}
	return nil, fmt.Errorf("no mock for Query: %s", sql[:min(60, len(sql))])
}

func (m *mockDBTX) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	for pattern, fn := range m.execFns {
		if strings.Contains(sql, pattern) {
			return fn(args...)
		}
	}
	return pgconn.NewCommandTag(""), fmt.Errorf("no mock for Exec: %s", sql[:min(60, len(sql))])
}
