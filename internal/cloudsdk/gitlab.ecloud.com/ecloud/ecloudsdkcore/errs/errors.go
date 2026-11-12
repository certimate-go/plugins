package errs

import (
	"fmt"
)

type SdkError struct {
	ErrorType    string
	ErrorMessage string
	error        error
}

func NewSdkError(errorType string, msg string, err error) *SdkError {
	return &SdkError{
		ErrorType:    errorType,
		ErrorMessage: msg,
		error:        err,
	}
}

func (err *SdkError) Error() string {
	return fmt.Sprintf("{\"errorType\": \"%s\", \"error-message\": \"%s\"}", err.ErrorType, err.ErrorMessage)
}

func (err *SdkError) DetailError() string {
	return fmt.Sprintf("{\"errorType\": \"%s\", \"error-message\": \"%s\", \"error\": \"%s\"}", err.ErrorType, err.ErrorMessage, err.error)
}

type ServerRequestError struct {
	*SdkError
}

func NewServerRequestError(msg string, err error) *ServerRequestError {
	return &ServerRequestError{
		NewSdkError("ServerRequestError", msg, err),
	}
}

type ServerResponseError struct {
	*SdkError
	Code    int
	Headers map[string][]string
	Body    string
}

func NewServerResponseError(msg string, err error, code int, headers map[string][]string, body string) *ServerResponseError {
	message := msg
	if len(message) == 0 {
		message = fmt.Sprintf("code=%d", code)
	}
	return &ServerResponseError{
		SdkError: NewSdkError("ServerResponseError", message, err),
		Code:     code,
		Headers:  headers,
		Body:     body,
	}
}

type ConnectionTimeOutError struct {
	*SdkError
}

func NewConnectionTimeOutError(msg string, err error) *ConnectionTimeOutError {
	return &ConnectionTimeOutError{
		SdkError: NewSdkError("ConnectionTimeOutError", msg, err),
	}
}

type SignatureError struct {
	*SdkError
}

func NewSignatureError(msg string, err error) *SignatureError {
	return &SignatureError{
		SdkError: NewSdkError("SignatureError", msg, err),
	}
}

type SslHandShakeError struct {
	*SdkError
}

func NewSslHandShakeError(msg string, err error) *SslHandShakeError {
	return &SslHandShakeError{
		SdkError: NewSdkError("SslHandShakeError", msg, err),
	}
}

type InvalidParameterError struct {
	*SdkError
}

func NewInvalidParameterError(msg string, err error) *InvalidParameterError {
	return &InvalidParameterError{
		NewSdkError("InvalidParameterError", msg, err),
	}
}

type CredentialError struct {
	*SdkError
}

func NewCredentialError(msg string, err error) *CredentialError {
	return &CredentialError{
		NewSdkError("CredentialError", msg, err),
	}
}
