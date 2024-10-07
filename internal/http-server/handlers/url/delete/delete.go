package delete

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"golang.org/x/exp/slog"
	"golang.org/x/net/context"
	"net/http"
	resp "url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
)

type DeleteURL interface {
	DeleteURL(ctx context.Context, log *slog.Logger, alias string, userID int64) error
	GetUserByNickname(ctx context.Context, log *slog.Logger, nickname string) (int64, string, error)
}

func New(log *slog.Logger, deleteURL DeleteURL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.user.delete.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		alias := chi.URLParam(r, "alias")
		nickname := r.Context().Value("nickname").(string)

		if nickname == "" || alias == "" {
			log.Error("params is empty")
			render.JSON(w, r, resp.Error("empty request"))
			return
		}

		userID, _, errGetUser := deleteURL.GetUserByNickname(r.Context(), log, nickname)
		if errGetUser != nil {
			log.Error("failed to get user by nickname", sl.Err(errGetUser))
			render.JSON(w, r, resp.Error(errGetUser.Error()))
			return
		}

		errDeleteURL := deleteURL.DeleteURL(r.Context(), log, alias, userID)
		if errDeleteURL != nil {
			log.Error(errDeleteURL.Error(), "error", errDeleteURL)
			render.JSON(w, r, resp.Error(errDeleteURL.Error()))
			return
		}

		log.Info("url delete successfully")
		render.JSON(w, r, resp.OK())
	}
}
