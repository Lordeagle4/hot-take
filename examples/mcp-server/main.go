// Command mcp-server serves the local, read-only catalogue used by the MCP guide.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Lordeagle4/hot-take/internal/examplemcp"
)

func main() {
	fail := flag.Bool("fail-calls", false, "return HTTP 503 for tool calls")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server := &http.Server{Addr: "127.0.0.1:8787", Handler: examplemcp.Handler{FailCalls: *fail}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	log.Print("Example catalogue MCP: http://127.0.0.1:8787/mcp")
	select {
	case err := <-finished:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Fatal(err)
		}
		if err := <-finished; !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}
