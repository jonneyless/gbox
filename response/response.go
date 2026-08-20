package response

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

var successCode int

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type PageResponse struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Data     any   `json:"data"`
}

func SetSuccessCode(code int) {
	successCode = code
}

// Success 成功响应
func Success(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{
		Code:    successCode,
		Message: "success",
		Data:    data,
	})
}

// 自定义文本响应
func Message(c echo.Context, message string) error {
	return c.JSON(http.StatusOK, Response{
		Code:    successCode,
		Message: message,
	})
}

// Ok 简单的成功响应
func Ok(c echo.Context) error {
	return c.JSON(http.StatusOK, Response{
		Code:    successCode,
		Message: "success",
	})
}

// Error 错误响应
func Error(c echo.Context, code int, message string) error {
	log.Printf("错误响应：%v", message)

	return c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// BadRequest 400错误
func BadRequest(c echo.Context, message string) error {
	if message == "" {
		message = "请求参数错误"
	}
	return Error(c, http.StatusBadRequest, message)
}

// Unauthorized 401错误
func Unauthorized(c echo.Context, message string) error {
	if message == "" {
		message = "未授权访问"
	}
	return Error(c, http.StatusUnauthorized, message)
}

// Forbidden 403错误
func Forbidden(c echo.Context, message string) error {
	if message == "" {
		message = "禁止访问"
	}
	return Error(c, http.StatusForbidden, message)
}

// NotFound 404错误
func NotFound(c echo.Context, message string) error {
	if message == "" {
		message = "资源不存在"
	}
	return Error(c, http.StatusNotFound, message)
}

// InternalServerError 500错误
func InternalServerError(c echo.Context, err error) error {
	message := "服务器内部错误"
	if err != nil {
		message = err.Error()
	}
	return Error(c, http.StatusInternalServerError, message)
}

// Page 分页响应
func Page(c echo.Context, total int64, page int, pageSize int, data any) error {
	return c.JSON(http.StatusOK, Response{
		Code:    successCode,
		Message: "success",
		Data: PageResponse{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
			Data:     data,
		},
	})
}
