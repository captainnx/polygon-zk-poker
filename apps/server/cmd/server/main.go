package main

import (
	"context"
	"fmt"
	"github.com/polygon-zk-poker/server/internal/httpapi"
	"github.com/polygon-zk-poker/server/internal/identity"
	"github.com/polygon-zk-poker/server/internal/store"
	"github.com/polygon-zk-poker/server/internal/table"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	var handler http.Handler = httpapi.New()
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		setup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		db, e := store.Open(setup, databaseURL)
		cancel()
		if e != nil {
			log.Fatal(fmt.Errorf("database initialization: %w", e))
		}
		defer db.Pool.Close()
		keyPath := os.Getenv("SERVER_KEY_FILE")
		if keyPath == "" {
			keyPath = ".local/server.key"
		}
		key, e := table.LoadKey(keyPath)
		if e != nil {
			log.Fatal(e)
		}
		var cache *redis.Client
		if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
			options, e := redis.ParseURL(redisURL)
			if e != nil {
				log.Fatal(e)
			}
			cache = redis.NewClient(options)
			defer cache.Close()
		}
		service := table.New(db, key, cache)
		handler = httpapi.WithProtocol(service, &identity.Postgres{Pool: db.Pool})
		log.Printf("signed protocol v3 enabled; server=%s", service.Address())
	}
	s := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdown)
	}()
	log.Printf("server listening at %s", address)
	if e := s.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
