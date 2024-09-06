// Copyright (C) 2024 Creditor Corp. Group.
// See LICENSE for copying information.

package common

import (
	"encoding/json"
	"net/http"

	"github.com/zeebo/errs"

	"mempool/internal/logger"
)

// ErrResponse describes response values for error case.
type ErrResponse struct {
	Status  int
	Code    string
	Message string
	Reason  string
}

// ErrResponseCode describes response values for error case with error code.
type ErrResponseCode struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

// NewErrResponse returns ErrResponse from status code and error.
func NewErrResponse(status int, err error) *ErrResponse {
	return &ErrResponse{
		Status:  status,
		Code:    http.StatusText(status),
		Message: err.Error(),
	}
}

// ToErrResponseCode returns ErrResponse sa ErrResponseCode.
func (e *ErrResponse) ToErrResponseCode() ErrResponseCode {
	return ErrResponseCode{
		Code:    e.Code,
		Message: e.Message,
		Reason:  e.Reason,
	}
}

// Serve replies to request with specific code and error.
func (e *ErrResponse) Serve(log logger.Logger, errorClass errs.Class, w http.ResponseWriter) {
	w.WriteHeader(e.Status)
	if err := json.NewEncoder(w).Encode(e.ToErrResponseCode()); err != nil {
		log.Error("failed to write json error response", errorClass.Wrap(err))
	}
}
