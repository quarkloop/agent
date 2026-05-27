package gatewaysvc

import (
	"fmt"
	"strings"
	"sync"

	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

// externalRequestQuota bounds outbound provider operations. Reserving before
// dispatch is required because providers can charge or rate-limit failed calls.
type externalRequestQuota struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func newExternalRequestQuota(limit int64) *externalRequestQuota {
	return &externalRequestQuota{limit: limit}
}

func (q *externalRequestQuota) reserve(provider, model, operation string) error {
	if q == nil || q.limit <= 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.used >= q.limit {
		return serviceerrors.RateLimit(fmt.Sprintf(
			"gateway external request limit reached before provider dispatch: provider=%s model=%s operation=%s requests=%d limit=%d",
			strings.TrimSpace(provider), strings.TrimSpace(model), strings.TrimSpace(operation), q.used, q.limit,
		))
	}
	q.used++
	return nil
}
