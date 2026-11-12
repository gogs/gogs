package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

type userLoginSession struct {
	ID         int64     `json:"id"`
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	// IsCurrent is true for the browser that asks.
	IsCurrent bool `json:"isCurrent"`
}

type userLoginSessions struct {
	Items []userLoginSession `json:"items"`
}

// currentLoginSessionID returns the ID of the login session of the browser, or
// zero when it has none, e.g., when it signed in with an access token.
func currentLoginSessionID(c flamego.Context, sess session.Session, viewer *database.User) (int64, error) {
	token, _ := sess.Get(context.LoginSessionKey).(string)
	ls, err := database.Handle.LoginSessions().GetByToken(c.Request().Context(), viewer.ID, token)
	if err != nil {
		if database.IsErrLoginSessionNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "get login session")
	}
	return ls.ID, nil
}

// getUserLoginSessions lists the browsers the authenticated user is authenticated on.
func getUserLoginSessions(c flamego.Context, sess session.Session, viewer *database.User) (statusCode int, resp any, err error) {
	currentID, err := currentLoginSessionID(c, sess, viewer)
	if err != nil {
		log.Error("getUserLoginSessions: get current login session of user %d: %v", viewer.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	sessions, err := database.Handle.LoginSessions().List(c.Request().Context(), viewer.ID)
	if err != nil {
		log.Error("getUserLoginSessions: list login sessions of user %d: %v", viewer.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list login sessions")
	}
	items := make([]userLoginSession, 0, len(sessions))
	for _, ls := range sessions {
		item := userLoginSession{
			ID:         ls.ID,
			UserAgent:  ls.UserAgent,
			IP:         ls.IP,
			CreatedAt:  ls.Created,
			LastSeenAt: ls.LastSeen,
			IsCurrent:  ls.ID == currentID,
		}
		// The current browser comes first.
		if item.IsCurrent {
			items = append([]userLoginSession{item}, items...)
		} else {
			items = append(items, item)
		}
	}
	return http.StatusOK, &userLoginSessions{Items: items}, nil
}

// deleteUserLoginSession signs one browser of the authenticated user out.
func deleteUserLoginSession(c flamego.Context, viewer *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err = database.Handle.LoginSessions().DeleteByID(c.Request().Context(), viewer.ID, id); err != nil {
		log.Error("deleteUserLoginSession: delete login session %d of user %d: %v", id, viewer.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete login session")
	}
	return http.StatusNoContent, nil, nil
}

// deleteUserLoginSessions signs every other browser of the authenticated user out.
func deleteUserLoginSessions(c flamego.Context, sess session.Session, viewer *database.User) (statusCode int, resp any, err error) {
	currentID, err := currentLoginSessionID(c, sess, viewer)
	if err != nil {
		log.Error("deleteUserLoginSessions: get current login session of user %d: %v", viewer.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	if err = database.Handle.LoginSessions().DeleteAll(c.Request().Context(), viewer.ID, currentID); err != nil {
		log.Error("deleteUserLoginSessions: delete login sessions of user %d: %v", viewer.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete login sessions")
	}
	return http.StatusNoContent, nil, nil
}
