package client

import "time"

type Config struct {
	BaseURL string
	Timeout time.Duration

	RetryAttempts uint
	RetryDelay    time.Duration

	CBRatioThreshold float64
	CBMinRequests    uint32
	CBOpenWindow     time.Duration
}
