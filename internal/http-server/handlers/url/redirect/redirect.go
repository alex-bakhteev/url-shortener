package redirect

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

// URLGetter is an interface for getting url by alias.
//
//go:generate go run github.com/vektra/mockery/v2@v2.28.2 --name=URLGetter
type URLGetter interface {
	GetURL(ctx context.Context, log *slog.Logger, alias string, userID int64) (string, error)
	GetUserByNickname(ctx context.Context, log *slog.Logger, nickname string) (int64, string, error)
}

func New(log *slog.Logger, urlGetter URLGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.redirect.New"

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

		userID, _, errGetUser := urlGetter.GetUserByNickname(r.Context(), log, nickname)
		if errGetUser != nil {
			log.Error("failed to get user by nickname", sl.Err(errGetUser))
			render.JSON(w, r, resp.Error(errGetUser.Error()))
			return
		}

		resURL, errGetURL := urlGetter.GetURL(r.Context(), log, alias, userID)
		if errGetURL != nil {
			log.Error("failed to get url", sl.Err(errGetURL))
			render.JSON(w, r, resp.Error(errGetURL.Error()))
			return
		}

		log.Info("got url", slog.String("url", resURL))

		// redirect to found url
		http.Redirect(w, r, resURL, http.StatusFound)
	}
}
