package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/exp/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	deleteURL "url-shortener/internal/http-server/handlers/url/delete"
	"url-shortener/internal/http-server/handlers/url/redirect"
	"url-shortener/internal/http-server/handlers/url/save"
	deleteUser "url-shortener/internal/http-server/handlers/user/delete"
	"url-shortener/internal/http-server/handlers/user/login"
	"url-shortener/internal/storage/mongodb"
	"url-shortener/internal/storage/multiStorage"

	"url-shortener/internal/config"
	"url-shortener/internal/http-server/handlers/user/register"
	"url-shortener/internal/http-server/middleware/auth"
	mwLogger "url-shortener/internal/http-server/middleware/logger"
	"url-shortener/internal/lib/logger/handlers/slogpretty"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/storage/sqlite"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad()
	log := setupLogger(cfg.Env)
	log.Info(
		"starting url-shortener",
		slog.String("env", cfg.Env),
	)
	log.Debug("debug messages are enabled")

	// Инициализация MongoDB
	mongoDB, err := mongodb.NewClient(context.Background(), cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.AuthDB, cfg.URI)
	if err != nil {
		log.Error("failed to init MongoDB", sl.Err(err))
		os.Exit(1)
	}

	// Инициализация SQLite
	sqliteDB, err := sqlite.New(cfg.StoragePath)
	if err != nil {
		log.Error("failed to init SQLite", sl.Err(err))
		os.Exit(1)
	}

	multiStorage := multiStorage.NewDualStorage(sqliteDB, mongoDB)

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(mwLogger.New(log))
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	router.Route("/", func(r chi.Router) {
		r.Post("/register", register.New(log, multiStorage))
		r.Post("/login", login.New(log, multiStorage))
		r.Post("/url/save", auth.TokenAuthMiddleware(save.New(log, multiStorage)))
		r.Delete("/url/{alias}", auth.TokenAuthMiddleware(deleteURL.New(log, multiStorage)))
		r.Delete("/user/{nickname}", auth.TokenAuthMiddleware(deleteUser.New(log, multiStorage)))
	})
	router.Get("/redirect/{alias}", auth.TokenAuthMiddleware(redirect.New(log, multiStorage)))

	log.Info("starting server", slog.String("address", cfg.Address))

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.HTTPServer.IdleTimeout,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error("failed to start server")
		}
	}()

	log.Info("server started")

	<-done
	log.Info("stopping server")

	// TODO: move timeout to config
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("failed to stop server", sl.Err(err))

		return
	}

	// TODO: close storage

	log.Info("server stopped")
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = setupPrettySlog()
	case envDev:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	default: // If env config is invalid, set prod settings by default due to security
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}

	return log
}

func setupPrettySlog() *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}

	handler := opts.NewPrettyHandler(os.Stdout)

	return slog.New(handler)
}
