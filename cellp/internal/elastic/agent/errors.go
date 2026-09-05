package agent

import (
	"errors"

	"github.com/cellp/cellp/internal/elastic/contract"
)

var (
	errInvalidNode   = errors.New("invalid runtime node")
	errNodeCordoned  = errors.New("runtime node cordoned")
	errNodeNotFound  = errors.New("runtime node not found")
	errReplicaNotFound = errors.New("replica not found")
)

// CommandError carries a contract reason for API mapping.
type CommandError struct {
	Reason  contract.ReasonCode
	Message string
}

func (e *CommandError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Reason)
}

func elasticDisabled() error {
	return &CommandError{Reason: contract.ReasonElasticDisabled, Message: "elastic runtime disabled"}
}
