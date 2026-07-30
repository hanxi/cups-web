package main

import (
	"database/sql"
	"errors"
	"net/http"

	"cups-web/internal/auth"
	"cups-web/internal/store"
)

type meResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func MeHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		return
	}

	var resp meResponse
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		resp = meResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить профиль")
		}
		return
	}
	writeJSON(w, resp)
}
