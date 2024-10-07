package delete

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"golang.org/x/exp/slog"
	"golang.org/x/net/context"
	"net/http"
	resp "url-shortener/internal/lib/api/response"
)

type DeleteUser interface {
	DeleteUserByNickname(ctx context.Context, log *slog.Logger, nickname string) error
}

func New(log *slog.Logger, deleteUser DeleteUser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.user.delete.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		// Получаем никнейм из параметров URL
		nickname := chi.URLParam(r, "nickname")

		// Получаем никнейм пользователя из контекста (из токена авторизации)
		authNickname, ok := r.Context().Value("nickname").(string)
		if !ok {
			log.Error("failed to get authorized user nickname from context")
			render.JSON(w, r, resp.Error("unauthorized request"))
			return
		}

		// Проверяем, что переданный в запросе nickname совпадает с ником из токена авторизации
		if nickname != authNickname {
			log.Error("unauthorized attempt to delete another user's account", slog.String("authNickname", authNickname), slog.String("nickname", nickname))
			render.JSON(w, r, resp.Error("unauthorized action"))
			return
		}

		if nickname == "" {
			log.Error("nickname is empty")
			render.JSON(w, r, resp.Error("empty request"))
			return
		}

		// Удаляем пользователя
		errDeleteUser := deleteUser.DeleteUserByNickname(r.Context(), log, nickname)
		if errDeleteUser != nil {
			log.Error(errDeleteUser.Error(), "error", errDeleteUser)
			render.JSON(w, r, resp.Error(errDeleteUser.Error()))
			return
		}

		log.Info("user deleted successfully", slog.String("nickname", nickname))
		render.JSON(w, r, resp.OK())
	}
}
