package admin

import (
	"testing"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate/testutil"
	"github.com/dgate-io/dgate-api/internal/config/configtest"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"go.uber.org/zap"
)

func TestAdminRoutes_configureRoutes(t *testing.T) {
	mux := chi.NewMux()
	cs := testutil.NewMockChangeState()
	cs.On("ResourceManager").Return(resources.NewManager())
	cs.On("DocumentManager").Return(nil)
	conf := configtest.NewTestAdminConfig()
	if err := configureRoutes(
		mux, "test",
		zap.NewNop(),
		cs, conf,
	); err != nil {
		t.Fatal(err)
	}
}
