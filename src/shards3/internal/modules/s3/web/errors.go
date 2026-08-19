package web

import (
	"encoding/xml"
	"net/http"
)

type errorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId,omitempty"`
}

func writeS3Error(w http.ResponseWriter, status int, code, message, resource string) {
	response := errorResponse{
		Code:      code,
		Message:   message,
		Resource:  resource,
		RequestID: "shards3-request",
	}

	payload, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		http.Error(w, message, status)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}
