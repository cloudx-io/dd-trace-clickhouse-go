package ddtrace_test

import (
	"context"
	"log"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	ddtrace "github.com/cloudx-io/dd-trace-clickhouse-go"
)

func ExampleWrap() {
	if err := tracer.Start(); err != nil {
		log.Print(err)
		return
	}
	defer tracer.Stop()

	conn, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"localhost:9000"}})
	if err != nil {
		log.Print(err)
		return
	}
	conn = ddtrace.Wrap(conn, ddtrace.WithService("analytics-api"))
	defer conn.Close()

	var version string
	if err := conn.QueryRow(context.Background(), "SELECT version()").Scan(&version); err != nil {
		log.Print(err)
	}
}
