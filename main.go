package main

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ip-api/proxy/batch"
	"github.com/ip-api/proxy/cache"
	"github.com/ip-api/proxy/fetcher"
	"github.com/ip-api/proxy/handlers"
	"github.com/ip-api/proxy/util"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05.000",
	}).With().Str("part", "main").Logger()

	client, err := fetcher.NewIPApi(logger.With().Str("part", "fetcher").Logger())
	if err != nil {
		logger.Fatal().Err(err).Msg("could not create fetcher")
	}

	cacheSize := 1024 * 1024 * 1024 // 1GB
	if v := os.Getenv("CACHE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			logger.Fatal().Err(err).Msg("invalid cache size")
		} else {
			cacheSize = n
		}
	}

	cache := cache.New(cacheSize)
	batches := batch.New(logger.With().Str("part", "batch").Logger(), cache, client)

	go batches.ProcessLoop()

	h := handlers.Handler{
		Logger:  logger.With().Str("part", "handler").Logger(),
		Batches: batches,
		Client:  client,
	}

	s := &fasthttp.Server{
		Handler:               h.Index,
		ReadTimeout:           time.Minute,
		WriteTimeout:          time.Minute,
		IdleTimeout:           time.Hour,
		ReadBufferSize:        4096 * 2,
		WriteBufferSize:       4096 * 2,
		MaxRequestBodySize:    1 * 1024 * 1024,
		Logger:                util.FasthttpLogger{Logger: logger.With().Str("part", "fasthttp").Logger()},
		NoDefaultServerHeader: true,
		NoDefaultContentType:  true,
	}

	addr := os.Getenv("LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	logger.Info().Msgf("listening on %q", addr)

	go func() {
		if err := s.ListenAndServe(addr); err != nil {
			logger.Fatal().Err(err).Msg("failed to ListenAndServe")
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	<-ch

	go func() {
		if err := s.Shutdown(); err != nil {
			logger.Error().Err(err).Msg("failed to shutdown server")
		}
	}()

	time.Sleep(time.Second * 10)
}
