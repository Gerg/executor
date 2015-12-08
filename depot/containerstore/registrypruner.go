package containerstore

import (
	"os"

	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

type registryPruner struct {
	logger     lager.Logger
	config     *ContainerConfig
	clock      clock.Clock
	containers *nodeMap
}

func newRegistryPruner(logger lager.Logger, config *ContainerConfig, clock clock.Clock, containers *nodeMap) *registryPruner {
	return &registryPruner{
		logger:     logger,
		config:     config,
		clock:      clock,
		containers: containers,
	}
}

func (r *registryPruner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	logger := r.logger.Session("registry-pruner")
	ticker := r.clock.NewTicker(r.config.ReservedExpirationTime / 2)

	close(ready)

	defer ticker.Stop()
	for {
		select {
		case <-ticker.C():

			now := r.clock.Now()
			r.containers.CompleteExpired(logger, now)
		case <-signals:
			return nil
		}
	}
	return nil
}