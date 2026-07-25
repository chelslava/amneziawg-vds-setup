package engine

import (
	"fmt"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	legacyengine "github.com/chelslava/amneziawg-vds-setup/v2/internal/engine/legacy"
	upstreamengine "github.com/chelslava/amneziawg-vds-setup/v2/internal/engine/upstream"
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
		return legacyengine.Engine{}, nil
	case config.Upstream:
		return upstreamengine.Engine{}, nil
	default:
		return nil, fmt.Errorf("unsupported engine %q", kind)
	}
}
