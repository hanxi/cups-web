package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

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
		writeJSONError(w, http.StatusBadRequest, "missing credentials")
		return
	}
	// guest 是保留账号，只能由 SessionHandler 在开启访客模式时自动登入。
	// 用它的用户名从公开登录接口挤进来会绕过管理员对该模式的开关意图。
	if strings.EqualFold(strings.TrimSpace(req.Username), guestUsername) {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// 暴力破解防护：同一 IP+用户名连续失败过多则临时锁定。
	key := loginKey(r, req.Username)
	if ok, _ := loginAllowed(key); !ok {
		log.Printf("[login] rate limited: key=%q", key)
		writeJSONError(w, http.StatusTooManyRequests, "too many attempts, please try again later")
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
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "login failed")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		registerLoginFailure(key)
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	clearLoginFailures(key)

	sess := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}
	if err := auth.SetSession(w, r, sess); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "session error")
		return
	}
	// set csrf token cookie (readable by JS)
	http.SetCookie(w, auth.NewCSRFCookie(r, randomToken()))
	writeJSON(w, map[string]bool{"ok": true})
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w, r)
	writeJSON(w, map[string]bool{"ok": true})
}

// SessionHandler handles GET /api/session and returns session info if present.
// 若访客模式（Issue #55）已开启且当前没有有效会话，就临时为保留账号 guest 签发一个
// 会话 + CSRF cookie，让访客直接进入打印页；guest 是普通 user 角色，管理页仍拒绝进入。
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err == nil {
		writeJSON(w, sess)
		return
	}
	if guestSess, ok := issueGuestSessionIfEnabled(w, r); ok {
		writeJSON(w, guestSess)
		return
	}
	writeJSONError(w, http.StatusUnauthorized, "unauthorized")
}

// issueGuestSessionIfEnabled 检查 guest_mode 设置：开启时惰性创建/复用 guest 用户
// 并写入 session + csrf cookie；未开启或过程出错时返回 false，让调用方走原路径。
// 惰性创建保证关闭访客模式的部署里没有多余的 guest 账号常驻数据库。
//
// 并发安全：两个首次访问的会话如果几乎同时命中此函数，两侧 GetUserByUsername 都会
// 报 ErrNoRows，随后一侧 CreateUser 拿到 UNIQUE 约束错误——遇到这种情况就再读一次
// 已存在的行，让后到者复用而不是回落到 401。
func issueGuestSessionIfEnabled(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	user, ok := loadOrCreateGuest(r)
	if !ok {
		return auth.Session{}, false
	}
	sess := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}
	if err := auth.SetSession(w, r, sess); err != nil {
		return auth.Session{}, false
	}
	http.SetCookie(w, auth.NewCSRFCookie(r, randomToken()))
	return sess, true
}

func loadOrCreateGuest(r *http.Request) (store.User, bool) {
	var enabled bool
	var user store.User
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		v, err := store.GetSettingInt(r.Context(), tx, store.SettingGuestMode, 0)
		if err != nil {
			return err
		}
		if v == 0 {
			return nil
		}
		enabled = true
		existing, err := store.GetUserByUsername(r.Context(), tx, guestUsername)
		if err == nil {
			user = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(randomToken()), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		created, err := store.CreateUser(r.Context(), tx, store.CreateUserInput{
			Username:     guestUsername,
			PasswordHash: string(hash),
			Role:         store.RoleUser,
			Protected:    true,
		})
		if err != nil {
			return err
		}
		user = created
		return nil
	})
	if !enabled {
		return store.User{}, false
	}
	if err == nil {
		return user, true
	}
	// 并发首访冲突：另一侧刚刚把 guest 插进去，回退到只读事务再读一次。
	var retry store.User
	if err2 := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		u, err := store.GetUserByUsername(r.Context(), tx, guestUsername)
		if err != nil {
			return err
		}
		retry = u
		return nil
	}); err2 == nil {
		return retry, true
	}
	return store.User{}, false
}

// guestUsername 是访客模式下自动登入的保留账号名。写死小写字符串而不是配置项，
// 是为了让「保护 guest 不被删/改名/密码登录」这几处判断都指向同一个真值。
const guestUsername = "guest"

func CSRFHandler(w http.ResponseWriter, r *http.Request) {
	// Not used: CSRF token is set on login; provide endpoint if needed
	token := randomToken()
	http.SetCookie(w, auth.NewCSRFCookie(r, token))
	writeJSON(w, map[string]string{"csrfToken": token})
}
