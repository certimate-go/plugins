package retry

import (
	"fmt"
	"time"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
)

type Template struct {
	RetryPolicy   Policy
	RetryTimes    int32
	MaxDuringTime int64
}

type RetryError struct {
	*errs.SdkError
	NumberOfFailedAttempts        int32
	MaxDuringTimeOfFailedAttempts int64
	LastFailedRetryContext        Context
}

func NewRetryError(numberOfFailedAttempts int32, maxDuringTimeOfFailedAttempts int64, lastFailedRetryContext Context) *RetryError {
	var msg string
	if numberOfFailedAttempts > 0 {
		msg = fmt.Sprintf("Retrying failed to complete successfully after %v attempts.", numberOfFailedAttempts)
	}
	if maxDuringTimeOfFailedAttempts > 0 {
		msg = fmt.Sprintf("Retrying failed to complete successfully after %v milliseconds.", maxDuringTimeOfFailedAttempts)
	}
	var retryErr error
	if v, err := lastFailedRetryContext.(*ErrorContext); err {
		retryErr = v.Err
	} else {
		retryErr = nil
	}

	return &RetryError{
		SdkError: errs.NewSdkError("RetryError", msg, retryErr),
	}
}

type ReFunc func() (interface{}, error)

func (t *Template) Call(f ReFunc) (interface{}, error) {
	startTime := time.Now().UnixNano()
	for retryTimes := 1; ; retryTimes++ {
		var ctx Context
		res, err := f()
		duration := (time.Now().UnixNano() - startTime) / 1e6
		if err != nil {
			ctx = &ErrorContext{
				Err:        err,
				RetryTimes: int32(retryTimes),
				DelayTime:  duration,
			}
		} else {
			ctx = &ResultContext{
				Result:     res,
				RetryTimes: int32(retryTimes),
				DelayTime:  duration,
			}
		}
		if r, err := ctx.(*ResultContext); err {
			return r.Result, nil
		}
		if int32(retryTimes) == t.RetryTimes {
			return nil, NewRetryError(int32(retryTimes), 0, ctx)
		} else if duration >= t.MaxDuringTime {
			return nil, NewRetryError(int32(retryTimes), duration, ctx)
		} else {
			sleepTime := t.RetryPolicy.computeWaitTime(ctx)
			if sleepTime <= 0 {
				continue
			}
			time.Sleep(time.Duration(sleepTime * 1e6))
		}
	}
}
