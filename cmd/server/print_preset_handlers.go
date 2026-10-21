package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/mux"

	"cups-web/internal/auth"
	"cups-web/internal/store"
)

// 打印预置（issue #128）：按用户保存多组打印参数，跨设备同步。
// 「上次设置」由前端存 localStorage，不经过这里。

// printPresetMaxPerUser 每个用户最多保存的预置数量，防止无限写入撑大表。
const printPresetMaxPerUser = 20

// printPresetMaxNameRunes 名称上限按字符计，中文名称与英文同样允许 64 个字。
const printPresetMaxNameRunes = 64

// printPresetMaxSettingsBytes 参数快照上限。正常快照不足 2KB，留足驱动选项标签的余量。
const printPresetMaxSettingsBytes = 16 << 10

var (
	errPrintPresetLimit     = errors.New("print preset limit reached")
	errPrintPresetDuplicate = errors.New("print preset name exists")
)

type printPresetResponse struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Settings  json.RawMessage `json:"settings"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

type printPresetPayload struct {
	Name     *string         `json:"name"`
	Settings json.RawMessage `json:"settings"`
}

// printPresetSession 取当前会话；guest 是多人共享账号，预置会互相覆盖，直接拒绝。
func printPresetSession(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return sess, false
	}
	if strings.EqualFold(sess.Username, guestUsername) {
		writeJSONError(w, http.StatusForbidden, "访客模式不支持打印预置")
		return sess, false
	}
	return sess, true
}

func validatePrintPresetName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "预置名称不能为空"
	}
	if utf8.RuneCountInString(name) > printPresetMaxNameRunes {
		return "", "预置名称过长"
	}
	return name, ""
}

// validatePrintPresetSettings 只要求是一个 JSON 对象且大小受限；具体字段由前端
// 套用时白名单过滤，提交打印时再由 /api/print 校验，这里不重复维护一份字段表。
func validatePrintPresetSettings(raw json.RawMessage) (string, string) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", "预置参数不能为空"
	}
	if len(raw) > printPresetMaxSettingsBytes {
		return "", "预置参数过大"
	}
	var obj map[string]json.RawMessage
	if raw[0] != '{' || json.Unmarshal(raw, &obj) != nil {
		return "", "预置参数格式无效"
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", "预置参数格式无效"
	}
	return compact.String(), ""
}

func printPresetListHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := printPresetSession(w, r)
	if !ok {
		return
	}
	var presets []store.PrintPreset
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var err error
		presets, err = store.ListPrintPresetsByUser(r.Context(), tx, sess.UserID)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list print presets")
		return
	}
	resp := make([]printPresetResponse, 0, len(presets))
	for _, p := range presets {
		resp = append(resp, mapPrintPreset(p))
	}
	writeJSON(w, resp)
}

func printPresetCreateHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := printPresetSession(w, r)
	if !ok {
		return
	}
	var payload printPresetPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, printPresetMaxSettingsBytes+1024)).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if payload.Name == nil {
		writeJSONError(w, http.StatusBadRequest, "预置名称不能为空")
		return
	}
	name, msg := validatePrintPresetName(*payload.Name)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	settings, msg := validatePrintPresetSettings(payload.Settings)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	var created store.PrintPreset
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		if _, err := store.GetPrintPresetByName(r.Context(), tx, sess.UserID, name); err == nil {
			return errPrintPresetDuplicate
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		n, err := store.CountPrintPresetsByUser(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		if n >= printPresetMaxPerUser {
			return errPrintPresetLimit
		}
		created, err = store.CreatePrintPreset(r.Context(), tx, sess.UserID, name, settings)
		return err
	})
	if writePrintPresetTxError(w, err, "failed to create print preset") {
		return
	}
	writeJSON(w, mapPrintPreset(created))
}

// printPresetUpdateHandler 支持只改名、只覆盖参数或两者同时修改。
func printPresetUpdateHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := printPresetSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var payload printPresetPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, printPresetMaxSettingsBytes+1024)).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	var name, settings string
	if payload.Name != nil {
		var msg string
		if name, msg = validatePrintPresetName(*payload.Name); msg != "" {
			writeJSONError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if payload.Settings != nil {
		var msg string
		if settings, msg = validatePrintPresetSettings(payload.Settings); msg != "" {
			writeJSONError(w, http.StatusBadRequest, msg)
			return
		}
	}

	var updated store.PrintPreset
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		current, err := store.GetPrintPreset(r.Context(), tx, id, sess.UserID)
		if err != nil {
			return err
		}
		if payload.Name == nil {
			name = current.Name
		}
		if payload.Settings == nil {
			settings = current.SettingsJSON
		}
		if name != current.Name {
			if other, err := store.GetPrintPresetByName(r.Context(), tx, sess.UserID, name); err == nil && other.ID != id {
				return errPrintPresetDuplicate
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		updated, err = store.UpdatePrintPreset(r.Context(), tx, id, sess.UserID, name, settings)
		return err
	})
	if writePrintPresetTxError(w, err, "failed to update print preset") {
		return
	}
	writeJSON(w, mapPrintPreset(updated))
}

func printPresetDeleteHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := printPresetSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.DeletePrintPreset(r.Context(), tx, id, sess.UserID)
	})
	if writePrintPresetTxError(w, err, "failed to delete print preset") {
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// writePrintPresetTxError 统一映射事务错误，返回 true 表示已写出错误响应。
func writePrintPresetTxError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrPrintPresetNotFound):
		writeJSONError(w, http.StatusNotFound, "预置不存在")
	// 并发同名请求都可能通过事务内的预检查，由 UNIQUE(user_id, name) 兜底。
	case errors.Is(err, errPrintPresetDuplicate), strings.Contains(err.Error(), "UNIQUE constraint failed"):
		writeJSONError(w, http.StatusConflict, "已存在同名预置")
	case errors.Is(err, errPrintPresetLimit):
		writeJSONError(w, http.StatusBadRequest, "已达到预置数量上限")
	default:
		writeJSONError(w, http.StatusInternalServerError, fallback)
	}
	return true
}

func mapPrintPreset(p store.PrintPreset) printPresetResponse {
	return printPresetResponse{
		ID:        p.ID,
		Name:      p.Name,
		Settings:  json.RawMessage(p.SettingsJSON),
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
