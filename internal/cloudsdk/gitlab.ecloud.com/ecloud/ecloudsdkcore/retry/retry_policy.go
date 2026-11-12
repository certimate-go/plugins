package retry

import (
	"math"
	"math/rand"
	"time"
)

type Policy interface {
	computeWaitTime(failedRetryContext Context) int64
}

func NoWait() Policy {
	return &FixedRetryPolicy{
		0,
	}
}

func WaitRetryPolicy(sleepTime int64) Policy {
	return &FixedRetryPolicy{
		sleepTime,
	}
}

type FixedRetryPolicy struct {
	SleepTime int64
}

func (f *FixedRetryPolicy) computeWaitTime(failedRetryContext Context) int64 {
	return f.SleepTime
}

type RandomRetryPolicy struct {
	Minimum int64
	Maximum int64
}

func (r *RandomRetryPolicy) computeWaitTime(failedRetryContext Context) int64 {
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	t := rand.Int63n(int64(math.Abs(float64(r.Maximum - r.Minimum))))
	return t + r.Minimum
}

type IncrementingRetryPolicy struct {
	InitWaitTime int64
	Increment    int64
}

func (i *IncrementingRetryPolicy) computeWaitTime(failedRetryContext Context) int64 {
	res := i.InitWaitTime + (i.Increment * int64(failedRetryContext.getRetryTimes()-1))
	if res > 0 {
		return res
	}
	return 0
}

type ExponentialRetryPolicy struct {
	Multiplier  int64
	MaximumWait int64
}

func (e *ExponentialRetryPolicy) computeWaitTime(failedRetryContext Context) int64 {
	exp := math.Pow(float64(failedRetryContext.getRetryTimes()), 2)
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	t := rand.Int63n(e.Multiplier * int64(exp))
	if t > e.MaximumWait {
		t = e.MaximumWait
	}
	if t >= 0 {
		return t
	}
	return 0
}

type CompositeRetryPolicy struct {
	WaitPolicies []Policy
}

func (c *CompositeRetryPolicy) computeWaitTime(failedRetryContext Context) int64 {
	var waitTime int64 = 0
	for _, policy := range c.WaitPolicies {
		waitTime += policy.computeWaitTime(failedRetryContext)
	}
	return waitTime
}
