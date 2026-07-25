package engine

import (
	"fmt"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

type Engine interface {
	Name() config.Engine
	Image() string
	Container() string
	InstallCommand(state.State) string
	UpdateCommand(state.State) string
	StatusCommand(state.State) string
}

func Select(kind config.Engine) (Engine, error) {
	switch kind {
	case config.Legacy:
		return legacyEngine{}, nil
	case config.Upstream:
		return upstreamEngine{}, nil
	default:
		return nil, fmt.Errorf("unsupported engine %q", kind)
	}
}
