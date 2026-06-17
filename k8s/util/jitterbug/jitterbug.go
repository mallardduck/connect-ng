package jitterbug

// jitterbug is a package to implement a lightweight jitter based task system
// The key principal

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type JitterFunction func(nextTrigger, strictDeadline time.Duration) (bool, error)

// JitterChecker is not go-routine safe
type JitterChecker struct {
	config          *Config
	calculator      Calculator
	callable        JitterFunction
	tickChan        <-chan time.Time
	triggerInterval time.Duration
}

// NewJitterChecker will complete initialization of optional Config fields and return a jitter checker
// It is not a go-routine safe
func NewJitterChecker(config *Config, callable JitterFunction) *JitterChecker {
	calculator := NewJitterCalculator(config, nil)
	return NewJitterCheckerFromCalculator(*calculator, callable)
}

// NewJitterCheckerFromCalculator will complete initialization of optional Config fields and return a jitter checker
func NewJitterCheckerFromCalculator(calculator JitterCalculator, callable JitterFunction) *JitterChecker {
	return &JitterChecker{
		config:     calculator.config,
		calculator: &calculator,
		callable:   callable,
	}
}

// Start prepares the first checkin interval and starts the ticker
func (j *JitterChecker) Start() {
	j.calculateCheckinInterval()
	if j.tickChan == nil {
		j.tickChan = time.Tick(j.config.PollingInterval)
	}
}

func (j *JitterChecker) calculateCheckinInterval() {
	j.triggerInterval = j.calculator.CalculateCheckinInterval()
}

func (j *JitterChecker) Run() {
	logger := log.Log.WithName("jitterbug")
	for range j.tickChan {
		logger.V(1).Info("tick")
		j.run()
	}
}

func (j *JitterChecker) run() {
	logger := log.Log.WithName("jitterbug")

	// Apply initial delay if configured
	if j.config.InitialDelay > 0 {
		logger.V(1).Info("initial delay", "duration", j.config.InitialDelay)
		select {
		case <-time.After(j.config.InitialDelay):
			// Proceed
		}
	}
	refresh, err := j.callable(j.triggerInterval, j.config.StrictDeadline)
	if err != nil {
		logger.Error(err, "jitter check failed")
		return
	}

	if refresh {
		j.calculateCheckinInterval()
	}
}
