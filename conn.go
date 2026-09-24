package ddtrace

import (
	"context"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	ddtraceext "github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	ddtracer "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/DataDog/dd-trace-go/v2/instrumentation"
)

const (
	// ComponentName is the component tag on spans created by this package.
	ComponentName = "ClickHouse/clickhouse-go.v2"

	// DBSystemName is the db.system tag on spans created by this package.
	DBSystemName = "clickhouse"

	// OperationName is the Datadog operation name for traced ClickHouse calls.
	OperationName = "clickhouse.query"
)

// Wrap traces calls on a ClickHouse connection:
//
//	conn = ddtrace.Wrap(conn, ddtrace.WithPeerService("analytics"))
//	err := conn.Ping(ctx)
//
// Use the wrapped connection in place of conn, and pass request contexts to
// its methods to attach spans to a parent. Options set span metadata without
// changing the underlying connection. Spans inherit the application's service
// name unless [WithService] is set. Wrap(nil) returns nil.
func Wrap(conn clickhouse.Conn, opts ...Option) clickhouse.Conn {
	if conn == nil {
		return nil
	}
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &tracedConn{Conn: conn, cfg: cfg}
}

// tracedConn delegates to a ClickHouse connection and traces selected calls.
type tracedConn struct {
	clickhouse.Conn
	cfg *config
}

// trace records a completed driver call with the supplied context and metadata.
func trace(ctx context.Context, cfg *config, queryType, query string, startTime time.Time, err error) {
	resource := query
	if cfg.resourceName != "" {
		resource = cfg.resourceName
	} else if resource == "" {
		resource = queryType
	}

	opts := []ddtracer.StartSpanOption{
		ddtracer.SpanType(ddtraceext.SpanTypeSQL),
		ddtracer.Tag(ddtraceext.Component, ComponentName),
		ddtracer.Tag(ddtraceext.SpanKind, ddtraceext.SpanKindClient),
		ddtracer.Tag(ddtraceext.DBSystem, DBSystemName),
		ddtracer.Tag("sql.query_type", queryType),
		ddtracer.StartTime(startTime),
		ddtracer.ResourceName(resource),
	}
	if cfg.serviceName != "" {
		opts = append(opts, instrumentation.ServiceNameWithSource(
			cfg.serviceName,
			instrumentation.ServiceSourceWithServiceOption,
		))
	}
	if cfg.peerService != "" {
		opts = append(opts, ddtracer.Tag(ddtraceext.PeerService, cfg.peerService))
	}
	if cfg.host != "" {
		opts = append(opts, ddtracer.Tag(ddtraceext.TargetHost, cfg.host))
	}
	if cfg.port != "" {
		opts = append(opts, ddtracer.Tag(ddtraceext.TargetPort, cfg.port))
	}
	if cfg.database != "" {
		opts = append(opts, ddtracer.Tag(ddtraceext.DBName, cfg.database))
	}
	if cfg.user != "" {
		opts = append(opts, ddtracer.Tag(ddtraceext.DBUser, cfg.user))
	}
	span, _ := ddtracer.StartSpanFromContext(ctx, OperationName, opts...)
	if err != nil && cfg.errCheck != nil && !cfg.errCheck(err) {
		err = nil
	}
	span.Finish(ddtracer.WithError(err))
}

// Select traces the full Select call, including its decoding into dest.
func (c *tracedConn) Select(ctx context.Context, dest any, query string, args ...any) error {
	start := time.Now()
	err := c.Conn.Select(ctx, dest, query, args...)
	trace(ctx, c.cfg, "Query", query, start, err)
	return err
}

// Query traces the call that opens rows; later iteration and scans are untraced.
func (c *tracedConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.Conn.Query(ctx, query, args...)
	trace(ctx, c.cfg, "Query", query, start, err)
	return rows, err
}

// QueryRow reports an error already available from Row.Err when the call returns.
// A later Scan call is outside the span.
func (c *tracedConn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	start := time.Now()
	row := c.Conn.QueryRow(ctx, query, args...)
	trace(ctx, c.cfg, "Query", query, start, row.Err())
	return row
}

// PrepareBatch traces preparation and returns a batch whose Send, Flush, and
// Abort calls are traced separately. The batch uses ctx for those later spans.
func (c *tracedConn) PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	start := time.Now()
	batch, err := c.Conn.PrepareBatch(ctx, query, opts...)
	trace(ctx, c.cfg, "Prepare", query, start, err)
	if err != nil {
		return nil, err
	}
	return &tracedBatch{Batch: batch, cfg: c.cfg, query: query, ctx: ctx}, nil
}

// Exec traces the driver call and returns its error unchanged.
func (c *tracedConn) Exec(ctx context.Context, query string, args ...any) error {
	start := time.Now()
	err := c.Conn.Exec(ctx, query, args...)
	trace(ctx, c.cfg, "Exec", query, start, err)
	return err
}

// AsyncInsert traces the driver call as an Exec operation. The wait argument is
// passed through unchanged; with wait false, the span does not await insertion.
func (c *tracedConn) AsyncInsert(ctx context.Context, query string, wait bool, args ...any) error {
	start := time.Now()
	err := c.Conn.AsyncInsert(ctx, query, wait, args...)
	trace(ctx, c.cfg, "Exec", query, start, err)
	return err
}

// QueryFormat traces the call that opens the reader; later reads are untraced.
func (c *tracedConn) QueryFormat(ctx context.Context, format, query string, args ...any) (io.ReadCloser, error) {
	start := time.Now()
	rc, err := c.Conn.QueryFormat(ctx, format, query, args...)
	trace(ctx, c.cfg, "Query", query, start, err)
	return rc, err
}

// InsertFormat traces the driver call, including its reads from data.
func (c *tracedConn) InsertFormat(ctx context.Context, format, query string, data io.Reader) error {
	start := time.Now()
	err := c.Conn.InsertFormat(ctx, format, query, data)
	trace(ctx, c.cfg, "Exec", query, start, err)
	return err
}

// Ping traces a connectivity check with "Ping" as its default resource.
func (c *tracedConn) Ping(ctx context.Context) error {
	start := time.Now()
	err := c.Conn.Ping(ctx)
	trace(ctx, c.cfg, "Ping", "", start, err)
	return err
}
