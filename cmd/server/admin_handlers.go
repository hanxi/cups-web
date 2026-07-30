package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"cups-web/internal/auth"
	"cups-web/internal/store"
)

var (
	errDeleteDefaultAdmin = errors.New("администратор по умолчанию не может быть удален")
	errProtectedRole      = errors.New("защищенная роль администратора не может быть изменена")
	errAdminRename        = errors.New("имя администратора не может быть изменено")
)

type adminUserPayload struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	ContactName string `json:"contactName"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
}

type adminUserResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Protected   bool   `json:"protected"`
	ContactName string `json:"contactName"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type settingsPayload struct {
	RetentionDays *int64 `json:"retentionDays"`
	SaveHistory   *bool  `json:"saveHistory"`
}

func adminListUsersHandler(w http.ResponseWriter, r *http.Request) {
	var resp []adminUserResponse
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		users, err := store.ListUsers(r.Context(), tx)
		if err != nil {
			return err
		}
		resp = mapAdminUsers(users)
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось получить список пользователей")
		return
	}
	writeJSON(w, resp)
}

func adminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload adminUserPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверные данные запроса")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" || payload.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "имя пользователя и пароль обязательны")
		return
	}
	role := normalizeRole(payload.Role)
	if role == "" {
		writeJSONError(w, http.StatusBadRequest, "неверная роль")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось хешировать пароль")
		return
	}

	var created store.User
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		user, err := store.CreateUser(r.Context(), tx, store.CreateUserInput{
			Username:     payload.Username,
			PasswordHash: string(hash),
			Role:         role,
			Protected:    false,
			ContactName:  payload.ContactName,
			Phone:        payload.Phone,
			Email:        payload.Email,
		})
		if err != nil {
			return err
		}
		created = user
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось создать пользователя")
		return
	}
	writeJSON(w, mapAdminUser(created))
}

func adminUpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный идентификатор пользователя")
		return
	}
	var payload adminUserPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверные данные запроса")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" {
		writeJSONError(w, http.StatusBadRequest, "имя пользователя обязательно")
		return
	}
	role := normalizeRole(payload.Role)
	if role == "" {
		writeJSONError(w, http.StatusBadRequest, "неверная роль")
		return
	}

	var pwdHash *string
	if strings.TrimSpace(payload.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "не удалось хешировать пароль")
			return
		}
		h := string(hash)
		pwdHash = &h
	}

	var updated store.User
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		current, err := store.GetUserByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if current.Username == "admin" && payload.Username != "admin" {
			return errAdminRename
		}
		if current.Username == "admin" && role != store.RoleAdmin {
			return errProtectedRole
		}
		if current.Username == "admin" {
			role = store.RoleAdmin
		}

		user, err := store.UpdateUser(r.Context(), tx, store.UpdateUserInput{
			ID:           id,
			Username:     payload.Username,
			PasswordHash: pwdHash,
			Role:         role,
			ContactName:  payload.ContactName,
			Phone:        payload.Phone,
			Email:        payload.Email,
		})
		if err != nil {
			return err
		}
		updated = user
		return nil
	})
	if err != nil {
		if errors.Is(err, errAdminRename) {
			writeJSONError(w, http.StatusBadRequest, errAdminRename.Error())
			return
		}
		if errors.Is(err, errProtectedRole) {
			writeJSONError(w, http.StatusBadRequest, "роль администратора не может быть изменена")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "пользователь не найден")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "не удалось обновить пользователя")
		}
		return
	}
	writeJSON(w, mapAdminUser(updated))
}

func adminDeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный идентификатор пользователя")
		return
	}
	sess, _ := auth.GetSession(r)
	if sess.UserID == id {
		writeJSONError(w, http.StatusBadRequest, "невозможно удалить текущего пользователя")
		return
	}
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if user.Username == "admin" {
			return errDeleteDefaultAdmin
		}
		return store.DeleteUser(r.Context(), tx, id)
	})
	if err != nil {
		if errors.Is(err, errDeleteDefaultAdmin) {
			writeJSONError(w, http.StatusBadRequest, "администратор не может быть удален")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "пользователь не найден")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "не удалось удалить пользователя")
		}
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func adminGetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var retention int64
	var saveHistory int64
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		val, err := store.GetSettingInt(r.Context(), tx, store.SettingRetentionDays, 0)
		if err != nil {
			return err
		}
		retention = val
		sh, err := store.GetSettingInt(r.Context(), tx, store.SettingSaveHistory, 1)
		if err != nil {
			return err
		}
		saveHistory = sh
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить настройки")
		return
	}
	writeJSON(w, map[string]interface{}{
		"retentionDays": retention,
		"saveHistory":   saveHistory != 0,
	})
}

func adminUpdateSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var payload settingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверные данные запроса")
		return
	}
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		if payload.RetentionDays != nil {
			if *payload.RetentionDays < 0 {
				return errors.New("неверное значение срока хранения")
			}
			if err := store.SettingInt(r.Context(), tx, store.SettingRetentionDays, *payload.RetentionDays); err != nil {
				return err
			}
		}
		if payload.SaveHistory != nil {
			var v int64
			if *payload.SaveHistory {
				v = 1
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingSaveHistory, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}


func adminCleanupHandler(w http.ResponseWriter, r *http.Request) {
	count, err := cleanupAllPrints(r.Context(), appStore, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "ошибка очистки: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "deleted": count})
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "":
		return store.RoleUser
	case store.RoleUser:
		return store.RoleUser
	case store.RoleAdmin:
		return store.RoleAdmin
	default:
		return ""
	}
}

func parseIDParam(r *http.Request) (int64, error) {
	idStr := mux.Vars(r)["id"]
	return strconv.ParseInt(idStr, 10, 64)
}

func mapAdminUsers(users []store.User) []adminUserResponse {
	resp := make([]adminUserResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, mapAdminUser(user))
	}
	return resp
}

func mapAdminUser(user store.User) adminUserResponse {
	return adminUserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Role:        user.Role,
		Protected:   user.Username == "admin",
		ContactName: user.ContactName,
		Phone:       user.Phone,
		Email:       user.Email,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
