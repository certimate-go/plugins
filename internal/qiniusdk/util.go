package qiniusdk

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qiniu/go-sdk/v7/client"
)

const qiniuHost = "https://api.qiniu.com"

func urlf(pathf string, pathargs ...any) string {
	path := fmt.Sprintf(pathf, pathargs...)
	path = strings.TrimPrefix(path, "/")
	return qiniuHost + "/" + path
}

// formatSdkError enriches a qiniu go-sdk error with the HTTP status code, the
// request id (X-Reqid, useful to quote when filing a Qiniu support ticket) and
// the full server-side detail, so callers can see exactly what the API rejected
// instead of just the bare error message. Falls back to err.Error() for errors
// that are not a qiniu *client.ErrorInfo.
func formatSdkError(err error) string {
	if err == nil {
		return ""
	}
	var ei *client.ErrorInfo
	if errors.As(err, &ei) {
		return fmt.Sprintf("http_code=%d reqid=%s detail=%s", ei.HttpCode(), ei.Reqid, ei.ErrorDetail())
	}
	return err.Error()
}
