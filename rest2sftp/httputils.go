package rest2sftp

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"net/http"
)

// RespondWithJSON sends client HTTP response in JSON format
func RespondWithJSON(w http.ResponseWriter, httpStatusCode int, data interface{}) {
	resp, err := json.Marshal(data)
	if err != nil {
		log.WithError(err).WithField("data", data).Error("failed to marshal data")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	w.Write(resp)
	return
}

// RespondNoContent sends client HTTP response without content
func RespondNoContent(w http.ResponseWriter, httpStatusCode int) {
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(httpStatusCode)
	return
}
