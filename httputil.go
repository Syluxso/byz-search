package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type httpError struct {
	status int
	detail string
}

func (e *httpError) Error() string { return e.detail }

func errBadRequest(detail string) error { return &httpError{status: 400, detail: detail} }
func errUnauthorized(detail string) error {
	return &httpError{status: 401, detail: detail}
}
func errForbidden(detail string) error  { return &httpError{status: 403, detail: detail} }
func errBadGateway(detail string) error { return &httpError{status: 502, detail: detail} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	writeJSON(w, status, problem{Title: title, Detail: detail, Status: status})
}

func writeErr(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		title := http.StatusText(he.status)
		if title == "" {
			title = "Error"
		}
		writeProblem(w, he.status, title, he.detail)
		return
	}
	log.Printf("error: %v", err)
	writeProblem(w, http.StatusBadGateway, "Bad Gateway", err.Error())
}
