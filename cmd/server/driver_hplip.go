package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const hplipPluginCommand = "/opt/cups-drivers/libexec/hplip-plugin"

type hplipPluginStatus struct {
	Version   string `json:"version"`
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	Saved     bool   `json:"saved"`
}

func getHPLIPPluginStatus(ctx context.Context) *hplipPluginStatus {
	if _, err := os.Stat(hplipPluginCommand); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, hplipPluginCommand, "status").Output()
	if err != nil {
		log.Printf("[hplip-plugin] Could not read status: %v", err)
		return nil
	}
	var status hplipPluginStatus
	if err := json.Unmarshal(out, &status); err != nil {
		log.Printf("[hplip-plugin] Invalid status response: %v", err)
		return nil
	}
	return &status
}

func adminPrepareHPLIPPluginHandler(w http.ResponseWriter, r *http.Request) {
	status := getHPLIPPluginStatus(r.Context())
	if status == nil || !status.Supported {
		writeJSONError(w, http.StatusBadRequest, "HPLIP plug-in is unavailable on this installation")
		return
	}
	job, busyID := startDriverJob("prepare", "hplip-plugin", func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error) {
		fmt.Fprintln(logBuf, "Downloading and verifying the matching HPLIP plug-in")
		cmd := exec.CommandContext(ctx, hplipPluginCommand, "prepare")
		cmd.Stderr = logBuf
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("could not prepare HPLIP plug-in: %w", err)
		}
		var result map[string]any
		if err := json.Unmarshal(out, &result); err != nil {
			return nil, fmt.Errorf("could not read HPLIP plug-in license: %w", err)
		}
		return result, nil
	})
	if job == nil {
		writeDriverJobBusy(w, busyID)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"jobId": job.id})
}

func adminInstallHPLIPPluginHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AcceptedLicense bool   `json:"acceptedLicense"`
		Digest          string `json:"digest"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if decoded, err := hex.DecodeString(payload.Digest); err != nil || len(decoded) != 32 || !payload.AcceptedLicense {
		writeJSONError(w, http.StatusBadRequest, "Review and accept the HPLIP plug-in license before installation")
		return
	}
	status := getHPLIPPluginStatus(r.Context())
	if status == nil || !status.Supported {
		writeJSONError(w, http.StatusBadRequest, "HPLIP plug-in is unavailable on this installation")
		return
	}
	job, busyID := startDriverJob("install", "hplip-plugin", func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error) {
		if err := runDriverCommand(ctx, logBuf, hplipPluginCommand, "install", payload.Digest); err != nil {
			return nil, fmt.Errorf("HPLIP plug-in installation failed: %w", err)
		}
		invalidatePPDModels()
		return map[string]any{"name": "hplip-plugin"}, nil
	})
	if job == nil {
		writeDriverJobBusy(w, busyID)
		return
	}
	log.Printf("[hplip-plugin] Installation requested (job=%s, user=%s)", job.id, sessionUsername(r))
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"jobId": job.id})
}
