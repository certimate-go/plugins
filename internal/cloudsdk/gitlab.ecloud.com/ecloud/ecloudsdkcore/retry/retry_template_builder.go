package retry

type TemplateBuilder struct {
	RetryPolicy   Policy
	RetryTimes    int32
	MaxDuringTime int64
}

const (
	MaxRetryTimes              int32 = 10
	MaxDuringTimeInMillisecond int64 = 60 * 1000
)

func NewBuilder() *TemplateBuilder {
	return &TemplateBuilder{}
}

func (t *TemplateBuilder) SetRetryPolicy(retryPolicy Policy) *TemplateBuilder {
	t.RetryPolicy = retryPolicy
	return t
}

func (t *TemplateBuilder) SetRetryTimes(retryTimes int32) *TemplateBuilder {
	t.RetryTimes = retryTimes
	return t
}

func (t *TemplateBuilder) SetMaxDuringTime(maxDuringTime int64) *TemplateBuilder {
	t.MaxDuringTime = maxDuringTime
	return t
}

func (t *TemplateBuilder) Build() *Template {
	retryTimes := t.RetryTimes
	if t.RetryTimes <= 0 {
		retryTimes = MaxRetryTimes
	}
	maxDuringTime := t.MaxDuringTime
	if maxDuringTime <= 0 {
		maxDuringTime = MaxDuringTimeInMillisecond
	}
	return &Template{
		RetryPolicy:   t.RetryPolicy,
		RetryTimes:    retryTimes,
		MaxDuringTime: maxDuringTime,
	}
}
