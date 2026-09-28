package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mmvalidator/internal/checker"
	"mmvalidator/internal/jobs"
)

func TestRootServesApplicationDirectly(t *testing.T) {
	validator := checker.Mock{}
	server := New(jobs.NewManager(validator, 1000), true, func(string) checker.Validator {
		return validator
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if location := response.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect to %q", location)
	}
	if !strings.Contains(response.Body.String(), "MM Validator") {
		t.Fatal("root response does not contain the application interface")
	}
}

func TestStaticAssetsRemainAvailable(t *testing.T) {
	validator := checker.Mock{}
	server := New(jobs.NewManager(validator, 1000), true, func(string) checker.Validator {
		return validator
	})

	request := httptest.NewRequest(http.MethodGet, "/static/mmvalidator-icon-256.png", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", contentType)
	}
}
