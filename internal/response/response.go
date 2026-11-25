package response

import "github.com/labstack/echo/v4"

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(c echo.Context, data any) error {
	return c.JSON(200, data)
}

func Created(c echo.Context, data any) error {
	return c.JSON(201, data)
}

func Error(c echo.Context, status int, code, message string) error {
	return c.JSON(status, ErrorBody{
		Code:    code,
		Message: message,
	})
}
