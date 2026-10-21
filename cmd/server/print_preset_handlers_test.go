package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"cups-web/internal/auth"
	"cups-web/internal/store"
)

func printPresetRequest(t *testing.T, method, path, body string, sess auth.Session) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.HandleFunc("/print-presets", printPresetListHandler).Methods("GET")
	r.HandleFunc("/print-presets", printPresetCreateHandler).Methods("POST")
	r.HandleFunc("/print-presets/{id:[0-9]+}", printPresetUpdateHandler).Methods("PUT")
	r.HandleFunc("/print-presets/{id:[0-9]+}", printPresetDeleteHandler).Methods("DELETE")
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestPrintPresetCRUDAndOwnership(t *testing.T) {
	s, user := scheduledTestStore(t)
	previousStore := appStore
	appStore = s
	t.Cleanup(func() { appStore = previousStore })
	owner := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}

	rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"  黑白双面  ","settings":{"isColor":false,"duplex":"two-sided-long-edge"}}`, owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created printPresetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "黑白双面" || string(created.Settings) != `{"isColor":false,"duplex":"two-sided-long-edge"}` {
		t.Fatalf("unexpected preset: %+v %s", created, created.Settings)
	}
	path := "/print-presets/" + strconv.FormatInt(created.ID, 10)

	if rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"黑白双面","settings":{}}`, owner); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate name: %d", rec.Code)
	}

	var other store.User
	if err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
		var err error
		other, err = store.CreateUser(context.Background(), tx, store.CreateUserInput{Username: "other", PasswordHash: "unused", Role: store.RoleUser})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stranger := auth.Session{UserID: other.ID, Username: other.Username, Role: other.Role}
	for _, method := range []string{"PUT", "DELETE"} {
		if rec := printPresetRequest(t, method, path, `{"name":"x"}`, stranger); rec.Code != http.StatusNotFound {
			t.Fatalf("%s by other user: %d", method, rec.Code)
		}
	}
	if rec := printPresetRequest(t, "GET", "/print-presets", "", stranger); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("other user sees presets: %s", rec.Body)
	}

	// 只改名时保留原参数。
	rec = printPresetRequest(t, "PUT", path, `{"name":"Mono"}`, owner)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"isColor":false`) || !strings.Contains(rec.Body.String(), `"name":"Mono"`) {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	// 只覆盖参数时保留名称。
	rec = printPresetRequest(t, "PUT", path, `{"settings":{"copies":2}}`, owner)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Mono"`) || !strings.Contains(rec.Body.String(), `{"copies":2}`) {
		t.Fatalf("overwrite: %d %s", rec.Code, rec.Body)
	}
	if rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"Other","settings":{}}`, owner); rec.Code != http.StatusOK {
		t.Fatalf("second preset: %d", rec.Code)
	}
	if rec := printPresetRequest(t, "PUT", path, `{"name":"Other"}`, owner); rec.Code != http.StatusConflict {
		t.Fatalf("rename onto existing: %d", rec.Code)
	}

	if rec := printPresetRequest(t, "DELETE", path, "", owner); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := printPresetRequest(t, "DELETE", path, "", owner); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", rec.Code)
	}
}

func TestPrintPresetValidationAndLimits(t *testing.T) {
	s, user := scheduledTestStore(t)
	previousStore := appStore
	appStore = s
	t.Cleanup(func() { appStore = previousStore })
	owner := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}

	cases := map[string]string{
		"missing name":    `{"settings":{}}`,
		"blank name":      `{"name":"   ","settings":{}}`,
		"long name":       `{"name":"` + strings.Repeat("长", printPresetMaxNameRunes+1) + `","settings":{}}`,
		"missing setting": `{"name":"a"}`,
		"array settings":  `{"name":"a","settings":[1]}`,
		"string settings": `{"name":"a","settings":"x"}`,
		"huge settings":   `{"name":"a","settings":{"k":"` + strings.Repeat("x", printPresetMaxSettingsBytes) + `"}}`,
	}
	for name, body := range cases {
		if rec := printPresetRequest(t, "POST", "/print-presets", body, owner); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s", name, rec.Code, rec.Body)
		}
	}
	if rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"`+strings.Repeat("长", printPresetMaxNameRunes)+`","settings":{}}`, owner); rec.Code != http.StatusOK {
		t.Fatalf("64-rune name rejected: %d", rec.Code)
	}

	for i := 1; i < printPresetMaxPerUser; i++ {
		if rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"p`+strconv.Itoa(i)+`","settings":{}}`, owner); rec.Code != http.StatusOK {
			t.Fatalf("preset %d: %d", i, rec.Code)
		}
	}
	if rec := printPresetRequest(t, "POST", "/print-presets", `{"name":"overflow","settings":{}}`, owner); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit not enforced: %d", rec.Code)
	}

	guest := auth.Session{UserID: user.ID, Username: guestUsername, Role: store.RoleUser}
	if rec := printPresetRequest(t, "GET", "/print-presets", "", guest); rec.Code != http.StatusForbidden {
		t.Fatalf("guest allowed: %d", rec.Code)
	}
}

// 并发同名创建绕过预检查时，数据库 UNIQUE 约束的报错也要映射为 409 而不是 500。
func TestPrintPresetUniqueConstraintMapsToConflict(t *testing.T) {
	s, user := scheduledTestStore(t)
	err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
		if _, err := store.CreatePrintPreset(context.Background(), tx, user.ID, "dup", "{}"); err != nil {
			return err
		}
		_, err := store.CreatePrintPreset(context.Background(), tx, user.ID, "dup", "{}")
		return err
	})
	if err == nil {
		t.Fatal("expected unique constraint error")
	}
	rec := httptest.NewRecorder()
	if !writePrintPresetTxError(rec, err, "fallback") || rec.Code != http.StatusConflict {
		t.Fatalf("got %d %s for %v", rec.Code, rec.Body, err)
	}
}
