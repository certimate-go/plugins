package retry

type Context interface {
	get() interface{}
	hasResult() bool
	hasError() bool
	getResult() interface{}
	getRetryTimes() int32
	getDelayTime() int64
}

type ResultContext struct {
	Result     interface{}
	RetryTimes int32
	DelayTime  int64
}

func (r *ResultContext) get() interface{} {
	return r.Result
}

func (r *ResultContext) hasResult() bool {
	return true
}

func (r *ResultContext) hasError() bool {
	return false
}

func (r *ResultContext) getResult() interface{} {
	return r.Result
}

func (r *ResultContext) getRetryTimes() int32 {
	return r.RetryTimes
}

func (r *ResultContext) getDelayTime() int64 {
	return r.DelayTime
}

type ErrorContext struct {
	Err        error
	RetryTimes int32
	DelayTime  int64
}

func (e *ErrorContext) get() interface{} {
	return e.Err
}

func (e *ErrorContext) hasResult() bool {
	return false
}

func (e *ErrorContext) hasError() bool {
	return true
}

func (e *ErrorContext) getResult() interface{} {
	return e.Err
}

func (e *ErrorContext) getRetryTimes() int32 {
	return e.RetryTimes
}

func (e *ErrorContext) getDelayTime() int64 {
	return e.DelayTime
}
