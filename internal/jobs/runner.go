package jobs

import (
	"context"
)

func (s *Service) ReconcileOnce(ctx context.Context) {
	s.runOnce(ctx)
}
