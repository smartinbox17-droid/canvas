package kernel

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorReason string

const (
	ReasonOK                  ErrorReason = ""
	ReasonInvalidArgument     ErrorReason = "INVALID_ARGUMENT"
	ReasonUnauthorized        ErrorReason = "UNAUTHORIZED"
	ReasonForbidden           ErrorReason = "FORBIDDEN"
	ReasonNotFound            ErrorReason = "NOT_FOUND"
	ReasonConflict            ErrorReason = "CONFLICT"
	ReasonRevisionConflict    ErrorReason = "REVISION_CONFLICT"
	ReasonRateLimited         ErrorReason = "RATE_LIMITED"
	ReasonInsufficientCredits ErrorReason = "INSUFFICIENT_CREDITS"
	ReasonRouteUnavailable    ErrorReason = "ROUTE_UNAVAILABLE"
	ReasonUpstreamBlocked     ErrorReason = "UPSTREAM_BLOCKED"
	ReasonAssetInUse          ErrorReason = "ASSET_IN_USE"
	ReasonQuotaExceeded       ErrorReason = "QUOTA_EXCEEDED"
	ReasonInternalError       ErrorReason = "INTERNAL_ERROR"
	ReasonServiceNotReady     ErrorReason = "SERVICE_NOT_READY"
)

type AppError struct {
	HTTPStatus int
	Code       int
	Reason     ErrorReason
	Message    string
}

func (e *AppError) Error() string {
	return e.Message
}

func NewError(httpStatus, code int, reason ErrorReason, msg string) *AppError {
	return &AppError{HTTPStatus: httpStatus, Code: code, Reason: reason, Message: msg}
}

type Envelope[T any] struct {
	Code   int         `json:"code"`
	Data   T           `json:"data"`
	Msg    string      `json:"msg"`
	Reason ErrorReason `json:"reason"`
}

func OK[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, Envelope[T]{
		Code:   0,
		Data:   data,
		Msg:    "ok",
		Reason: ReasonOK,
	})
}

func Fail(c *gin.Context, err *AppError) {
	c.JSON(err.HTTPStatus, Envelope[any]{
		Code:   err.Code,
		Data:   nil,
		Msg:    err.Message,
		Reason: err.Reason,
	})
}
