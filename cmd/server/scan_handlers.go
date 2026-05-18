package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

type ScannerInfo struct {
	Name        string `json:"name"`
	Device      string `json:"device"`
	Description string `json:"description,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Model       string `json:"model,omitempty"`
	Type        string `json:"type,omitempty"`
}

func listScannersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	scanners, err := listSanesDevices()
	if err != nil {
		log.Printf("[scan] failed to list scanners: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list scanners: "+err.Error())
		return
	}

	json.NewEncoder(w).Encode(scanners)
}

func listSanesDevices() ([]ScannerInfo, error) {
	// Use scanimage -L to list available scanners
	cmd := exec.Command("scanimage", "-L")
	output, err := cmd.Output()
	if err != nil {
		// If scanimage command fails, return empty list
		log.Printf("[scan] scanimage -L failed: %v", err)
		return []ScannerInfo{}, nil
	}

	return parseScanimageOutput(string(output)), nil
}

func parseScanimageOutput(output string) []ScannerInfo {
	var scanners []ScannerInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse scanimage output format: `device `name' is a vendor model type`
		// Example: `device `hpaio:/usb/HP_LaserJet_Pro_MFP_M127-M128?serial=XXXXXX' is a Hewlett-Packard HP_LaserJet_Pro_MFP_M127-M128`
		if strings.HasPrefix(line, "device `") && strings.Contains(line, "' is a ") {
			// Extract device name
			start := strings.Index(line, "`") + 1
			end := strings.Index(line, "'")
			if start < end {
				device := line[start:end]

				// Extract description after "is a"
				isIdx := strings.Index(line, "' is a ")
				if isIdx != -1 {
					description := line[isIdx+7:]

					// Parse vendor and model from description
					parts := strings.SplitN(description, " ", 2)
					vendor := ""
					model := ""
					if len(parts) >= 1 {
						vendor = parts[0]
					}
					if len(parts) >= 2 {
						model = parts[1]
					}

					scanner := ScannerInfo{
						Name:        device,
						Device:      device,
						Description: description,
						Vendor:      vendor,
						Model:       model,
						Type:        "local",
					}
					scanners = append(scanners, scanner)
				}
			}
		}
	}

	return scanners
}

type scanRequest struct {
	ScannerDevice string `json:"scannerDevice"`
	Resolution    int    `json:"resolution"`
	ColorMode     string `json:"colorMode"`
	PaperSize     string `json:"paperSize"`
	ScanArea      string `json:"scanArea,omitempty"`
}

type scanResponse struct {
	JobID  int64  `json:"jobId"`
	Status string `json:"status"`
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScannerDevice == "" {
		writeJSONError(w, http.StatusBadRequest, "missing scannerDevice field")
		return
	}

	// Set defaults
	if req.Resolution == 0 {
		req.Resolution = 300
	}
	if req.ColorMode == "" {
		req.ColorMode = "color"
	}
	if req.PaperSize == "" {
		req.PaperSize = "A4"
	}

	// Get user from session
	session, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Create scan job in database
	job := &store.ScanJob{
		UserID:        int64(session.UserID),
		ScannerDevice: req.ScannerDevice,
		Filename:      fmt.Sprintf("scan_%s", time.Now().Format("20060102_150405")),
		StoredPath:    "", // Will be set after scan completes
		Status:        "scanning",
		Resolution:    req.Resolution,
		ColorMode:     req.ColorMode,
		PaperSize:     req.PaperSize,
		ScanArea:      req.ScanArea,
	}

	if err := appStore.CreateScanJob(r.Context(), job); err != nil {
		log.Printf("[scan] failed to create scan job: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to create scan job")
		return
	}

	// Start scan in background
	go executeScanJob(job, req)

	json.NewEncoder(w).Encode(scanResponse{
		JobID:  job.ID,
		Status: "scanning",
	})
}

func scanStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Get scan job ID from URL
	vars := mux.Vars(r)
	jobIDStr := vars["id"]
	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	// Get user from session
	session, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get scan job from database
	job, err := appStore.GetScanJobByUserID(r.Context(), int64(session.UserID), jobID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "scan job not found")
		return
	}

	// Return job status
	response := map[string]interface{}{
		"jobId":      job.ID,
		"status":     job.Status,
		"filename":   job.Filename,
		"createdAt":  job.CreatedAt,
		"resolution": job.Resolution,
		"colorMode":  job.ColorMode,
		"paperSize":  job.PaperSize,
	}

	if job.Status == "completed" && job.StoredPath != "" {
		response["fileUrl"] = fmt.Sprintf("/api/scan/%d/file", job.ID)
	}

	if job.Status == "failed" && job.ErrorMessage != "" {
		response["errorMessage"] = job.ErrorMessage
	}

	if job.CompletedAt != nil {
		response["completedAt"] = job.CompletedAt
	}

	json.NewEncoder(w).Encode(response)
}

func scanFileHandler(w http.ResponseWriter, r *http.Request) {
	// Get scan job ID from URL
	vars := mux.Vars(r)
	jobIDStr := vars["id"]
	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	// Get user from session
	session, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get scan job from database
	job, err := appStore.GetScanJobByUserID(r.Context(), int64(session.UserID), jobID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "scan job not found")
		return
	}

	// Check if scan is completed
	if job.Status != "completed" {
		writeJSONError(w, http.StatusBadRequest, "scan not completed yet")
		return
	}

	// Check if file exists
	if job.StoredPath == "" {
		writeJSONError(w, http.StatusNotFound, "scan file not found")
		return
	}

	// Serve the file
	http.ServeFile(w, r, job.StoredPath)
}

func scanRecordsHandler(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get scan jobs from database
	jobs, err := appStore.ListScanJobsByUserID(r.Context(), int64(session.UserID))
	if err != nil {
		log.Printf("[scan] failed to list scan jobs: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list scan jobs")
		return
	}

	// Convert to response format
	type scanJobResponse struct {
		ID            int64      `json:"id"`
		ScannerDevice string     `json:"scannerDevice"`
		Filename      string     `json:"filename"`
		Status        string     `json:"status"`
		Resolution    int        `json:"resolution"`
		ColorMode     string     `json:"colorMode"`
		PaperSize     string     `json:"paperSize"`
		CreatedAt     time.Time  `json:"createdAt"`
		CompletedAt   *time.Time `json:"completedAt,omitempty"`
		StoredPath    string     `json:"storedPath,omitempty"`
	}

	var response []scanJobResponse
	for _, job := range jobs {
		response = append(response, scanJobResponse{
			ID:            job.ID,
			ScannerDevice: job.ScannerDevice,
			Filename:      job.Filename,
			Status:        job.Status,
			Resolution:    job.Resolution,
			ColorMode:     job.ColorMode,
			PaperSize:     job.PaperSize,
			CreatedAt:     job.CreatedAt,
			CompletedAt:   job.CompletedAt,
			StoredPath:    job.StoredPath,
		})
	}

	json.NewEncoder(w).Encode(response)
}

func executeScanJob(job *store.ScanJob, req scanRequest) {
	// Create uploads directory for scans
	scanDir := filepath.Join(uploadDir, "scans", time.Now().Format("20060102"))
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		log.Printf("[scan] failed to create scan directory: %v", err)
		appStore.UpdateScanJobStatus(nil, job.ID, "failed", "failed to create scan directory")
		return
	}

	// Generate output filename
	outputPath := filepath.Join(scanDir, fmt.Sprintf("%s.png", job.Filename))

	// Build scanimage command
	args := []string{
		"-d", req.ScannerDevice,
		"--resolution", strconv.Itoa(req.Resolution),
		"--mode", req.ColorMode,
		"--format", "png",
		"--output", outputPath,
	}

	// Add paper size if specified
	if req.PaperSize != "" {
		args = append(args, "-x", paperSizeToScanArea(req.PaperSize))
		args = append(args, "-y", paperSizeToScanArea(req.PaperSize))
	}

	// Execute scan
	cmd := exec.Command("scanimage", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[scan] scanimage failed: %v, output: %s", err, string(output))
		appStore.UpdateScanJobStatus(nil, job.ID, "failed", fmt.Sprintf("scan failed: %v", err))
		return
	}

	// Update job with file path
	if err := appStore.UpdateScanJobFilePath(nil, job.ID, outputPath); err != nil {
		log.Printf("[scan] failed to update scan job file path: %v", err)
		appStore.UpdateScanJobStatus(nil, job.ID, "failed", "failed to update job")
		return
	}

	// Mark as completed
	appStore.UpdateScanJobStatus(nil, job.ID, "completed", "")
	log.Printf("[scan] scan job %d completed: %s", job.ID, outputPath)
}

func paperSizeToScanArea(paperSize string) string {
	switch paperSize {
	case "A4":
		return "210mm"
	case "A3":
		return "297mm"
	case "Letter":
		return "215.9mm"
	case "Legal":
		return "215.9mm"
	default:
		return "210mm"
	}
}