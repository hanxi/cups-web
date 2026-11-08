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
	errDeleteDefaultAdmin = errors.New("default admin cannot be deleted")
	errProtectedRole      = errors.New("protected admin role cannot change")
	errAdminRename        = errors.New("admin username cannot change")
	errDeleteGuest        = errors.New("guest user cannot be deleted")
	errGuestModify        = errors.New("guest user is managed automatically")
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
	RetentionDays     *int64  `json:"retentionDays"`
	SaveHistory       *bool   `json:"saveHistory"`
	MaxPagesPerJob    *int64  `json:"maxPagesPerJob"`
	MaxUploadBytes    *int64  `json:"maxUploadBytes"`
	CustomCSS         *string `json:"customCss"`
	GuestMode         *bool   `json:"guestMode"`
	AllowFontFallback *bool   `json:"allowFontFallback"`
}

// customCSSMaxLen 是自定义 CSS 的字节上限（32 KiB）。
// 管理员错手贴入巨量数据（比如把整个 tailwind.css 复制进来）会拖累每次登录页
// 的公开设置接口，也会撑大 SQLite 单行，这里给一个宽松但不无限的上限。
const customCSSMaxLen = 32 * 1024

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
		writeJSONError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, resp)
}

func adminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload adminUserPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" || payload.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "username and password required")
		return
	}
	// guest 是访客模式（Issue #55）保留账号名，禁止管理员手工新建同名普通账号，
	// 否则会与 SessionHandler 的惰性创建路径分裂成两份语义不同的 guest。
	if strings.EqualFold(payload.Username, guestUsername) {
		writeJSONError(w, http.StatusBadRequest, "guest 是访客模式保留账号")
		return
	}
	role := normalizeRole(payload.Role)
	if role == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid role")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
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
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, mapAdminUser(created))
}

func adminUpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var payload adminUserPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" {
		writeJSONError(w, http.StatusBadRequest, "username required")
		return
	}
	role := normalizeRole(payload.Role)
	if role == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid role")
		return
	}

	var pwdHash *string
	if strings.TrimSpace(payload.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
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
		// guest 是访客模式（Issue #55）用的托管账号，不能改用户名/角色/密码——
		// 让管理员误升为 admin 就等于给公网开了后台，改密码则打破「访客登录不走口令」的
		// 前提。要停用直接关掉访客模式开关。
		if current.Username == guestUsername {
			return errGuestModify
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
			writeJSONError(w, http.StatusBadRequest, "admin role cannot change")
			return
		}
		if errors.Is(err, errGuestModify) {
			writeJSONError(w, http.StatusBadRequest, "guest 账号由访客模式自动管理，不能编辑")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "user not found")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "failed to update user")
		}
		return
	}
	writeJSON(w, mapAdminUser(updated))
}

func adminDeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	sess, _ := auth.GetSession(r)
	if sess.UserID == id {
		writeJSONError(w, http.StatusBadRequest, "cannot delete current user")
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
		if user.Username == guestUsername {
			return errDeleteGuest
		}
		return store.DeleteUser(r.Context(), tx, id)
	})
	if err != nil {
		if errors.Is(err, errDeleteDefaultAdmin) {
			writeJSONError(w, http.StatusBadRequest, "admin cannot be deleted")
			return
		}
		if errors.Is(err, errDeleteGuest) {
			writeJSONError(w, http.StatusBadRequest, "guest 账号由访客模式自动管理，请通过设置关闭访客模式")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "user not found")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete user")
		}
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func adminGetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var retention int64
	var saveHistory int64
	var maxPages int64
	var maxBytes int64
	var customCSS string
	var guestMode int64
	var allowFontFallback int64
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
		mp, err := store.GetSettingInt(r.Context(), tx, store.SettingMaxPagesPerJob, 0)
		if err != nil {
			return err
		}
		maxPages = mp
		mb, err := store.GetSettingInt(r.Context(), tx, store.SettingMaxUploadBytes, 0)
		if err != nil {
			return err
		}
		maxBytes = mb
		css, err := store.GetSettingString(r.Context(), tx, store.SettingCustomCSS, "")
		if err != nil {
			return err
		}
		customCSS = css
		gm, err := store.GetSettingInt(r.Context(), tx, store.SettingGuestMode, 0)
		if err != nil {
			return err
		}
		guestMode = gm
		aff, err := store.GetSettingInt(r.Context(), tx, store.SettingAllowFontFallback, 0)
		if err != nil {
			return err
		}
		allowFontFallback = aff
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, map[string]interface{}{
		"retentionDays":     retention,
		"saveHistory":       saveHistory != 0,
		"maxPagesPerJob":    maxPages,
		"maxUploadBytes":    maxBytes,
		"customCss":         customCSS,
		"guestMode":         guestMode != 0,
		"allowFontFallback": allowFontFallback != 0,
	})
}

// publicSettingsHandler 返回登录前也需要展示的少量前端可读设置：
//   - customCss：登录页 / 主界面都会应用（Issue #57）。
//   - guestMode：前端据此决定登录页是否自动登入访客（Issue #55）。
//
// 未登录用户也会命中这些设置，所以必须走公开接口而不是 /api/admin/settings。
func publicSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var customCSS string
	var guestMode int64
	_ = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		if v, err := store.GetSettingString(r.Context(), tx, store.SettingCustomCSS, ""); err == nil {
			customCSS = v
		}
		if v, err := store.GetSettingInt(r.Context(), tx, store.SettingGuestMode, 0); err == nil {
			guestMode = v
		}
		return nil
	})
	writeJSON(w, map[string]interface{}{
		"customCss": customCSS,
		"guestMode": guestMode != 0,
	})
}

func adminUpdateSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var payload settingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		if payload.RetentionDays != nil {
			if *payload.RetentionDays < 0 {
				return errors.New("invalid retentionDays")
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingRetentionDays, *payload.RetentionDays); err != nil {
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
		if payload.MaxPagesPerJob != nil {
			if *payload.MaxPagesPerJob < 0 {
				return errors.New("invalid maxPagesPerJob")
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingMaxPagesPerJob, *payload.MaxPagesPerJob); err != nil {
				return err
			}
		}
		if payload.MaxUploadBytes != nil {
			if *payload.MaxUploadBytes < 0 {
				return errors.New("invalid maxUploadBytes")
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingMaxUploadBytes, *payload.MaxUploadBytes); err != nil {
				return err
			}
		}
		if payload.CustomCSS != nil {
			css := *payload.CustomCSS
			if len(css) > customCSSMaxLen {
				return errors.New("customCss too long")
			}
			if err := store.SetSettingString(r.Context(), tx, store.SettingCustomCSS, css); err != nil {
				return err
			}
		}
		if payload.GuestMode != nil {
			var v int64
			if *payload.GuestMode {
				v = 1
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingGuestMode, v); err != nil {
				return err
			}
		}
		if payload.AllowFontFallback != nil {
			var v int64
			if *payload.AllowFontFallback {
				v = 1
			}
			if err := store.SetSettingInt(r.Context(), tx, store.SettingAllowFontFallback, v); err != nil {
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
		writeJSONError(w, http.StatusInternalServerError, "cleanup failed: "+err.Error())
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
		Protected:   user.Username == "admin" || user.Username == guestUsername,
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
