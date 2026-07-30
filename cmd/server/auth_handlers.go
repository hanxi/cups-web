package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"cups-web/internal/auth"
	"cups-web/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// dummyBcryptHash 是一个合法的 bcrypt 哈希，仅用于在用户名不存在时执行一次
// 等价的密码比较，抹平「用户存在与否」的响应时序差异，防止用户名枚举。
const dummyBcryptHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSONStatus(w, status, map[string]string{"error": msg})
}

func randomToken() string {
	// crypto/rand.Text 返回密码学安全的随机字符串（Go 1.24+）。
	return rand.Text()
}

// LoginHandler handles POST /api/login
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "отсутствуют учетные данные")
		return
	}

	// 暴力破解防护：同一 IP+用户名连续失败过多则临时锁定。
	key := loginKey(r, req.Username)
	if ok, _ := loginAllowed(key); !ok {
		log.Printf("[login] rate limited: key=%q", key)
		writeJSONError(w, http.StatusTooManyRequests, "слишком много попыток, пожалуйста, попробуйте позже")
		return
	}

	var user store.User
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		found, err := store.GetUserByUsername(r.Context(), tx, req.Username)
		if err != nil {
			return err
		}
		user = found
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 用户不存在也执行一次等价 bcrypt 比较，抹平时序差异防用户枚举。
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(req.Password))
			registerLoginFailure(key)
			writeJSONError(w, http.StatusUnauthorized, "неверные учетные данные")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "ошибка входа")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		registerLoginFailure(key)
		writeJSONError(w, http.StatusUnauthorized, "неверные учетные данные")
		return
	}
	clearLoginFailures(key)

	sess := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}
	if err := auth.SetSession(w, sess); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "ошибка сессии")
		return
	}
	// set csrf token cookie (readable by JS)
	token := randomToken()
	csrfCookie := &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   auth.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	}
	http.SetCookie(w, csrfCookie)
	writeJSON(w, map[string]bool{"ok": true})
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w)
	writeJSON(w, map[string]bool{"ok": true})
}

// SessionHandler handles GET /api/session and returns session info if present
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		return
	}
	writeJSON(w, sess)
}

func CSRFHandler(w http.ResponseWriter, r *http.Request) {
	// Not used: CSRF token is set on login; provide endpoint if needed
	token := randomToken()
	csrfCookie := &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   auth.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	}
	http.SetCookie(w, csrfCookie)
	writeJSON(w, map[string]string{"csrfToken": token})
}
