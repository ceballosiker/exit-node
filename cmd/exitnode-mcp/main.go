package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "exitnode-mcp",
		Version: "0.1.0",
	}, nil)

	registerTools(server)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "exitnode-mcp:", err)
		os.Exit(1)
	}
}
