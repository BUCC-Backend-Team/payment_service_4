package main

import (
	"encoding/json"
	"net/http"
)

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", HandleHealth)
}