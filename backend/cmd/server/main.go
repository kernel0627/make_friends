package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"make_friends/backend/internal/api"
	"make_friends/backend/internal/db"
)

const (
	// readHeaderTimeout is the Slowloris guard: a peer that opens a connection
	// and dribbles headers cannot hold it open indefinitely.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	// writeTimeout must outlast the slowest handler. Smart-draft calls DeepSeek
	// with an 8s client timeout, so this leaves room around it.
	writeTimeout    = 60 * time.Second
	idleTimeout     = 120 * time.Second
	shutdownTimeout = 15 * time.Second
)

func main() {
	database, err := db.OpenSQLite()
	if err != nil {
		log.Fatalf("init db failed: %v", err)
	}

	serverRuntime := api.NewServer(database)
	router := api.NewRouterWithServer(serverRuntime)
	addr := envOrDefault("BACKEND_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	serverRuntime.StartModerationWorkers(workerCtx)
	serverRuntime.StartAutoApproveLoop()
	go func() {
		log.Printf("backend listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// The websocket handler blocks for the life of each connection, so bound
	// how long shutdown waits rather than draining forever.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("server run failed: %v", err)
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
		cancelWorkers()
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Printf("backend stopped")
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
