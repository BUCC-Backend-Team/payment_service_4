package errors

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Status  int    `json:"-"`
	Code    string `json:"error"`
	Message string `json:"-"`
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) HTTPStatus() int {
	if e.Status > 0 {
		return e.Status
	}
	return http.StatusInternalServerError
}

func New(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *AppError {
	return New(http.StatusBadRequest, "invalid_request", message)
}

func Unauthorized(message string) *AppError {
	return New(http.StatusUnauthorized, "unauthorized", message)
}

func Server(message string, err error) *AppError {
	return New(http.StatusInternalServerError, "internal_error", fmt.Sprintf("%s: %v", message, err))
}
