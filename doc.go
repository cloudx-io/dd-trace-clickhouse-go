// Package ddtrace adds Datadog spans to ClickHouse native API calls. Start the
// tracer in your application, then wrap a connection:
//
//	conn, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"localhost:9000"}})
//	if err != nil {
//	    return err
//	}
//	conn = ddtrace.Wrap(conn)
//	defer conn.Close()
//	err = conn.Ping(ctx)
//
// Pass a context with an active span to make the ClickHouse span its child.
// The wrapper returns driver errors unchanged. A span covers its driver method
// call, not later row scans or reads. This package does not instrument
// database/sql connections.
package ddtrace
