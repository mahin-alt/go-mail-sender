package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"sendmail/internal/settings"
)

// maxSettingsBody caps settings JSON bodies (they are tiny).
const maxSettingsBody = 64 << 10

type settingsResponse struct {
	Items []settings.Item `json:"items"`
}

// decodeJSON reads a size-limited JSON body, rejecting unknown fields.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return false
	}
	return true
}

// writeSettingsError maps service errors to HTTP statuses: bad input is a 400,
// anything else is logged and reported as a generic 500.
func writeSettingsError(w http.ResponseWriter, err error, what string) {
	var in *settings.InputError
	if errors.As(err, &in) {
		writeError(w, http.StatusBadRequest, in.Msg)
		return
	}
	log.Printf("%s failed: %v", what, err)
	writeError(w, http.StatusInternalServerError, "failed to "+what)
}

// GetSettings handles GET /api/v1/settings: every setting with its effective
// value, source (env or override), how it applies, and whether a restart is
// pending. Secret values are never included.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings are not configured")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{Items: h.settings.List()})
}

// UpdateSettings handles PUT /api/v1/settings with
// {"values": {"KEY": "value"}, "reset": ["KEY"]}. All-or-nothing; SMTP changes
// must pass a live connection check before they are saved.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings are not configured")
		return
	}
	var ch settings.Change
	if !decodeJSON(w, r, &ch) {
		return
	}
	items, err := h.settings.Update(r.Context(), ch)
	if err != nil {
		writeSettingsError(w, err, "update settings")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{Items: items})
}

// TestSMTP handles POST /api/v1/settings/smtp/test: sends a real test email over
// a fresh connection built from the (possibly unsaved) values in the body.
func (h *Handler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings are not configured")
		return
	}
	var req settings.TestRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	to, err := h.settings.SendTest(r.Context(), req)
	if err != nil {
		var in *settings.InputError
		if errors.As(err, &in) {
			writeError(w, http.StatusBadRequest, in.Msg)
			return
		}
		// Could not talk to the SMTP server: surface the reason, since this
		// endpoint exists to diagnose exactly that.
		writeError(w, http.StatusBadGateway, "test email failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "test email sent to " + to})
}
