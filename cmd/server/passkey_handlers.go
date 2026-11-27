package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/store"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gorilla/mux"
)

// Passkey（WebAuthn）登录：只是「建立 session 的另一种方式」，校验通过后走与密码登录
// 完全相同的 auth.SetSession + CSRF cookie 路径，下游鉴权无需改动。
//
// 路由仅在 PASSKEY_ENABLED=true 时注册（见 registerPasskeyRoutes）。

const (
	passkeySessionHeader = "X-Passkey-Session"
	maxPasskeysPerUser   = 10
	passkeyAuthFailedMsg = "passkey authentication failed"
)

var (
	passkeyCfg        passkeyConfig
	passkeyWA         *webauthn.WebAuthn
	passkeyChallenges = newPasskeyChallengeStore()
)

// initPasskey 读取环境变量并在开启时构造 WebAuthn 实例；配置错误应让启动失败。
func initPasskey(getenv func(string) string) error {
	cfg, err := parsePasskeyConfig(getenv)
	if err != nil {
		return err
	}
	passkeyCfg = cfg
	passkeyWA = nil
	if !cfg.Enabled {
		return nil
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPName,
		RPOrigins:     cfg.Origins,
	})
	if err != nil {
		return err
	}
	passkeyWA = wa
	return nil
}

func registerPasskeyRoutes(api, protected *mux.Router) {
	if !passkeyCfg.Enabled {
		return
	}
	api.HandleFunc("/passkey/login/begin", passkeyLoginBeginHandler).Methods("POST")
	api.HandleFunc("/passkey/login/finish", passkeyLoginFinishHandler).Methods("POST")
	protected.HandleFunc("/passkeys", passkeyListHandler).Methods("GET")
	protected.HandleFunc("/passkeys/register/begin", passkeyRegisterBeginHandler).Methods("POST")
	protected.HandleFunc("/passkeys/register/finish", passkeyRegisterFinishHandler).Methods("POST")
	protected.HandleFunc("/passkeys/{id:[0-9]+}", passkeyDeleteHandler).Methods("DELETE")
}

// AuthConfigHandler handles GET /api/auth/config（公开）。
func AuthConfigHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"passkeyEnabled": passkeyCfg.Enabled})
}

// passkeyUser 把 store.User 适配为 webauthn.User，不改动 store 层结构体。
type passkeyUser struct {
	user  store.User
	creds []webauthn.Credential
}

func passkeyUserHandle(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

func (u *passkeyUser) WebAuthnID() []byte                         { return passkeyUserHandle(u.user.ID) }
func (u *passkeyUser) WebAuthnName() string                       { return u.user.Username }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func decodeCredentials(rows []store.WebAuthnCredential) []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(rows))
	for _, row := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(row.CredentialJSON), &c); err != nil {
			log.Printf("[passkey] bad credential json id=%d: %v", row.ID, err)
			continue
		}
		out = append(out, c)
	}
	return out
}

func loadPasskeyUser(ctx context.Context, tx *sql.Tx, userID int64) (*passkeyUser, []store.WebAuthnCredential, error) {
	u, err := store.GetUserByID(ctx, tx, userID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := store.ListWebAuthnCredentials(ctx, tx, userID)
	if err != nil {
		return nil, nil, err
	}
	return &passkeyUser{user: u, creds: decodeCredentials(rows)}, rows, nil
}

func credIDString(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

// rejectNonBrowserOrGuest 供绑定类接口使用：拒绝 API Key 通道（防止密钥自繁殖）和 guest。
func rejectNonBrowserOrGuest(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	if auth.IsAPIKeyAuth(r.Context()) {
		writeJSONError(w, http.StatusForbidden, "API 密钥不能管理 passkey，请使用浏览器登录后再操作")
		return auth.Session{}, false
	}
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return auth.Session{}, false
	}
	if strings.EqualFold(sess.Username, guestUsername) {
		writeJSONError(w, http.StatusForbidden, "guest 不支持 passkey")
		return auth.Session{}, false
	}
	return sess, true
}

type passkeyView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
}

func passkeyListHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := rejectNonBrowserOrGuest(w, r)
	if !ok {
		return
	}
	var rows []store.WebAuthnCredential
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var err error
		rows, err = store.ListWebAuthnCredentials(r.Context(), tx, sess.UserID)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list passkeys")
		return
	}
	out := make([]passkeyView, 0, len(rows))
	for _, row := range rows {
		out = append(out, passkeyView{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt})
	}
	writeJSON(w, out)
}

func passkeyRegisterBeginHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := rejectNonBrowserOrGuest(w, r)
	if !ok {
		return
	}
	var pu *passkeyUser
	var count int
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var rows []store.WebAuthnCredential
		var err error
		pu, rows, err = loadPasskeyUser(r.Context(), tx, sess.UserID)
		count = len(rows)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start registration")
		return
	}
	if count >= maxPasskeysPerUser {
		writeJSONError(w, http.StatusConflict, "passkey limit reached")
		return
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(pu.creds))
	for _, c := range pu.creds {
		exclusions = append(exclusions, c.Descriptor())
	}
	// ResidentKey=required：可发现凭证，登录时无需输入用户名。
	creation, session, err := passkeyWA.BeginRegistration(pu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		log.Printf("[passkey] begin registration: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to start registration")
		return
	}
	key, ok := passkeyChallenges.put(passkeyChallenge{kind: "register", userID: sess.UserID, session: *session})
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "too many pending requests")
		return
	}
	writeJSON(w, map[string]any{"sessionKey": key, "options": creation})
}

func passkeyRegisterFinishHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := rejectNonBrowserOrGuest(w, r)
	if !ok {
		return
	}
	ch, ok := passkeyChallenges.take(r.Header.Get(passkeySessionHeader), "register")
	if !ok || ch.userID != sess.UserID {
		writeJSONError(w, http.StatusBadRequest, "passkey session expired")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > 64 {
		writeJSONError(w, http.StatusBadRequest, "name too long")
		return
	}
	var pu *passkeyUser
	if err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var err error
		pu, _, err = loadPasskeyUser(r.Context(), tx, sess.UserID)
		return err
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to finish registration")
		return
	}
	cred, err := passkeyWA.FinishRegistration(pu, ch.session, r)
	if err != nil {
		log.Printf("[passkey] finish registration user=%q: %v", sess.Username, err)
		writeJSONError(w, http.StatusBadRequest, "passkey registration failed")
		return
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to finish registration")
		return
	}
	var created store.WebAuthnCredential
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		n, err := store.CountWebAuthnCredentialsByUser(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		if n >= maxPasskeysPerUser {
			return errPasskeyLimit
		}
		created, err = store.CreateWebAuthnCredential(r.Context(), tx, store.WebAuthnCredential{
			UserID: sess.UserID, CredentialID: credIDString(cred.ID), Name: name, CredentialJSON: string(raw),
		})
		return err
	})
	if err != nil {
		if errors.Is(err, errPasskeyLimit) {
			writeJSONError(w, http.StatusConflict, "passkey limit reached")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to save passkey")
		return
	}
	writeJSON(w, passkeyView{ID: created.ID, Name: created.Name, CreatedAt: created.CreatedAt})
}

var errPasskeyLimit = errors.New("passkey limit reached")

func passkeyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := rejectNonBrowserOrGuest(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.DeleteWebAuthnCredential(r.Context(), tx, id, sess.UserID)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "passkey not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete passkey")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func passkeyLoginBeginHandler(w http.ResponseWriter, r *http.Request) {
	key := clientIP(r) + "|passkey"
	if ok, _ := loginAllowed(key); !ok {
		writeJSONError(w, http.StatusTooManyRequests, "too many attempts, please try again later")
		return
	}
	// begin 不需要凭据，匿名可调；按 IP 限制挑战发放频率，防止占满全局挑战池（拒绝服务）。
	if !passkeyBeginAllowed(clientIP(r)) {
		writeJSONError(w, http.StatusTooManyRequests, "too many attempts, please try again later")
		return
	}
	// 可发现凭证：不传用户名，由浏览器弹出账号列表。
	assertion, session, err := passkeyWA.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		log.Printf("[passkey] begin login: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "login failed")
		return
	}
	ck, ok := passkeyChallenges.put(passkeyChallenge{kind: "login", session: *session})
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "too many pending requests")
		return
	}
	writeJSON(w, map[string]any{"sessionKey": ck, "options": assertion})
}

func passkeyLoginFinishHandler(w http.ResponseWriter, r *http.Request) {
	key := clientIP(r) + "|passkey"
	if ok, _ := loginAllowed(key); !ok {
		writeJSONError(w, http.StatusTooManyRequests, "too many attempts, please try again later")
		return
	}
	fail := func() {
		registerLoginFailure(key)
		writeJSONError(w, http.StatusUnauthorized, passkeyAuthFailedMsg)
	}
	ch, ok := passkeyChallenges.take(r.Header.Get(passkeySessionHeader), "login")
	if !ok {
		fail()
		return
	}
	// FinishPasskeyLogin 会读取并解析 body；限制大小避免滥用。
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail()
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var matched *passkeyUser
	var matchedRow store.WebAuthnCredential
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) != 8 {
			return nil, errors.New("bad user handle")
		}
		uid := int64(binary.BigEndian.Uint64(userHandle))
		var pu *passkeyUser
		var row store.WebAuthnCredential
		err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
			var err error
			row, err = store.GetWebAuthnCredentialByCredentialID(r.Context(), tx, credIDString(rawID))
			if err != nil {
				return err
			}
			if row.UserID != uid {
				return errors.New("credential does not belong to user")
			}
			pu, _, err = loadPasskeyUser(r.Context(), tx, uid)
			return err
		})
		if err != nil {
			return nil, err
		}
		matched, matchedRow = pu, row
		return pu, nil
	}
	_, cred, err := passkeyWA.FinishPasskeyLogin(handler, ch.session, r)
	if err != nil || matched == nil || cred == nil {
		log.Printf("[passkey] finish login failed ip=%s: %v", clientIP(r), err)
		fail()
		return
	}
	// 签名计数回退 / 克隆告警：同步型 passkey 计数恒为 0，库内已按此处理，不会误判。
	if cred.Authenticator.CloneWarning {
		log.Printf("[passkey] clone warning user=%q credential=%d", matched.user.Username, matchedRow.ID)
		fail()
		return
	}
	// guest 是保留账号，只能由访客模式自动登入。
	if strings.EqualFold(matched.user.Username, guestUsername) {
		fail()
		return
	}
	// 先写回签名计数与最近使用时间，再签发会话：凭证已被删除（0 行受影响）或写回失败
	// 都视为登录失败，避免已删除的 passkey 仍能换到 session。
	raw, err := json.Marshal(cred)
	if err == nil {
		err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
			return store.UpdateWebAuthnCredentialUsage(r.Context(), tx, matchedRow.ID, string(raw))
		})
	}
	if err != nil {
		log.Printf("[passkey] update usage user=%q credential=%d: %v", matched.user.Username, matchedRow.ID, err)
		fail()
		return
	}
	clearLoginFailures(key)

	s := auth.Session{UserID: matched.user.ID, Username: matched.user.Username, Role: matched.user.Role}
	if err := auth.SetSession(w, r, s); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "session error")
		return
	}
	http.SetCookie(w, auth.NewCSRFCookie(r, randomToken()))
	writeJSON(w, map[string]bool{"ok": true})
}

// passkeyBeginAllowed：每个 IP 每分钟最多 30 次 begin，进程内计数。
var (
	passkeyBeginMu   sync.Mutex
	passkeyBeginHits = map[string][]time.Time{}
)

func passkeyBeginAllowed(ip string) bool {
	now := time.Now()
	cut := now.Add(-time.Minute)
	passkeyBeginMu.Lock()
	defer passkeyBeginMu.Unlock()
	if len(passkeyBeginHits) > 10000 {
		passkeyBeginHits = map[string][]time.Time{}
	}
	kept := passkeyBeginHits[ip][:0]
	for _, t := range passkeyBeginHits[ip] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 30 {
		passkeyBeginHits[ip] = kept
		return false
	}
	passkeyBeginHits[ip] = append(kept, now)
	return true
}
