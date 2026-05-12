package pubsub

import (
"testing"
"log/slog"

"github.com/g8e-ai/g8e/components/g8eo/config"
"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

type pubsubFixture struct {
Cfg    *config.Config
Logger *slog.Logger
DB     *MockOperatorPubSubClient
Svc    *PubSubCommandService
}

func newPubsubFixture(t *testing.T) *pubsubFixture {
cfg := testutil.NewTestConfig(t)
logger := testutil.NewTestLogger()
db := NewMockOperatorPubSubClient()

svc, err := NewPubSubCommandService(CommandServiceConfig{
Config:       cfg,
Logger:       logger,
PubSubClient: db,
})
if err != nil {
t.Fatalf("failed to create PubSubCommandService: %v", err)
}

return &pubsubFixture{
Cfg:    cfg,
Logger: logger,
DB:     db,
Svc:    svc,
}
}
