package cli

import (
	"fmt"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func ValidateEngineReuse(existing state.State, requested config.Engine) error {
	if existing.Engine != requested {
		return fmt.Errorf("refusing automatic migration from %s to %s; automatic migration is disabled; install the other engine as a separate new installation", existing.Engine, requested)
	}
	return nil
}
