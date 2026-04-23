package main

import (
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/learning"
)

func configuredLearningRuntime(_ config.Config) *learning.Runtime {
	return learning.NewRuntime(config.LearningSignalLogPath(), learning.Config{})
}
