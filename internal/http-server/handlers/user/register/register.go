package register

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"golang.org/x/exp/slog"
	"io"
	"net/http"
	"url-shortener/internal/http-server/middleware/auth"
	resp "url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/storage"
)

type Request struct {
	Nickname string `json:"nickname" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type UserSaver interface {
	SaveUser(ctx context.Context, log *slog.Logger, nickname, passwordHash string) error
}

func New(log *slog.Logger, userSaver UserSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.save.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		err := render.DecodeJSON(r.Body, &req)
		if errors.Is(err, io.EOF) {
			// Такую ошибку встретим, если получили запрос с пустым телом.
			// Обработаем её отдельно
			log.Error("request body is empty")

			render.JSON(w, r, resp.Error("empty request"))

			return
		}
		if err != nil {
			log.Error("failed to decode request body", sl.Err(err))

			render.JSON(w, r, resp.Error("failed to decode request"))

			return
		}

		log.Info("request body decoded", slog.Any("request", req))

		if err := validator.New().Struct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)

			log.Error("invalid request", sl.Err(err))

			render.JSON(w, r, resp.ValidationError(validateErr))

			return
		}

		hashedPassword, err := auth.RegisterUser(req.Nickname, req.Password)
		if err != nil {
			log.Error("failed to register user", "error", err)
		}

		errSaveUser := userSaver.SaveUser(r.Context(), log, req.Nickname, hashedPassword)
		if errors.Is(errSaveUser, storage.ErrUserExists) {
			log.Info("user already exists", slog.String("url", req.Nickname))
			render.JSON(w, r, resp.Error("User already exists"))
			return
		}

		log.Info("user registered successfully", slog.String("nickname", req.Nickname), slog.String("hashPassword", hashedPassword))
		render.JSON(w, r, resp.OK())
	}
}
