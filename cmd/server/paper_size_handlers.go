package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"cups-web/internal/store"
)

func listPaperSizesHandler(w http.ResponseWriter, r *http.Request) {
	var sizes []store.CustomPaperSize
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var err error
		sizes, err = store.ListCustomPaperSizes(r.Context(), tx)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list paper sizes")
		return
	}
	if sizes == nil {
		sizes = []store.CustomPaperSize{}
	}
	writeJSON(w, sizes)
}

func createPaperSizeHandler(w http.ResponseWriter, r *http.Request) {
	var input store.CreatePaperSizeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Width <= 0 || input.Height <= 0 {
		writeJSONError(w, http.StatusBadRequest, "width and height must be positive")
		return
	}

	var created store.CustomPaperSize
	err := appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		var err error
		created, err = store.CreateCustomPaperSize(r.Context(), tx, input)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create paper size")
		return
	}
	writeJSON(w, created)
}

func updatePaperSizeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var input store.UpdatePaperSizeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Width <= 0 || input.Height <= 0 {
		writeJSONError(w, http.StatusBadRequest, "width and height must be positive")
		return
	}

	var updated store.CustomPaperSize
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		var err error
		updated, err = store.UpdateCustomPaperSize(r.Context(), tx, id, input)
		return err
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "paper size not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update paper size")
		return
	}
	writeJSON(w, updated)
}

func deletePaperSizeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.DeleteCustomPaperSize(r.Context(), tx, id)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "paper size not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete paper size")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
