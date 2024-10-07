package login

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
)

type Request struct {
	Nickname string `json:"nickname" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

type GetUser interface {
	GetUserByNickname(ctx context.Context, log *slog.Logger, nickname string) (int64, string, error)
}

func New(log *slog.Logger, getUser GetUser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.user.login.New"

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

		userID, passwordHash, errGetUser := getUser.GetUserByNickname(r.Context(), log, req.Nickname)
		if errGetUser != nil {
			log.Error("user is not exist", "error", errGetUser)
			render.JSON(w, r, resp.Error("User is not exist"))
			return
		}

		token, errLogin := auth.Login(req.Nickname, req.Password, passwordHash)
		if errLogin != nil {
			log.Error("failed to login", "error", errLogin, userID)
			render.JSON(w, r, resp.Error("Wrong login or password"))
			return
		}

		log.Info("user login successfully")
		response := LoginResponse{
			Status: "success",
			Token:  token,
		}
		render.JSON(w, r, response)
	}
}
