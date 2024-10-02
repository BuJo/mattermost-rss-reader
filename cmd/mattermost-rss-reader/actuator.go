package main

import (
	"encoding/json"
	"net/http"
)

type healthState struct {
	Status string `json:"status"`
}

func healthHandler() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		if r.RequestURI == "/actuator/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(healthState{"UP"})

			return
		}
	}
}
