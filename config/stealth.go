package config

import (
	"os"
	"strings"
)

const (
	HeaderStrategyRealSimulation = "real_simulation"
	HeaderStrategyRandom         = "random"

	http2ModeAuto    = "auto"
	http2ModeForce   = "force"
	http2ModeDisable = "disable"

	HTTP2ModeAuto    = http2ModeAuto
	HTTP2ModeForce   = http2ModeForce
	HTTP2ModeDisable = http2ModeDisable
)

var (
	stealthModeEnv    = "STEALTH_MODE"
	headerStrategyEnv = "HEADER_STRATEGY"
	http2ModeEnv      = "STEALTH_HTTP2_MODE"
)

func IsStealthModeEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(stealthModeEnv)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ActiveHeaderStrategy() string {
	strategy := strings.ToLower(strings.TrimSpace(os.Getenv(headerStrategyEnv)))
	switch strategy {
	case HeaderStrategyRandom:
		return HeaderStrategyRandom
	case HeaderStrategyRealSimulation:
		return HeaderStrategyRealSimulation
	default:
		if IsStealthModeEnabled() {
			return HeaderStrategyRandom
		}
		return HeaderStrategyRealSimulation
	}
}

func HTTP2Mode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(http2ModeEnv)))
	switch mode {
	case http2ModeForce, http2ModeDisable:
		return mode
	default:
		return http2ModeAuto
	}
}
