package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
)

func TestProbe_EvaluateFailClosed(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	tmp := t.TempDir()
	cat := compliance.KSICatalog{Version: "1.0", Source: "test", KSIs: []compliance.KSI{{
		ID: "KSI-CMT-01", Title: "T", Category: compliance.KSICategoryCMT,
		ControlRefs: []string{"CM-3"}, ApplicableClasses: []compliance.CertificationClass{compliance.ClassC},
		ValidationCycle: compliance.ValidationCycleMachine,
	}}}
	data, _ := json.Marshal(cat)
	path := filepath.Join(tmp, "cat.json")
	os.WriteFile(path, data, 0644)
	loaded, err := loadKSICatalog(path)
	if err != nil {
		t.Fatalf("catalog load: %v", err)
	}
	rs := evaluateKSIs(context.Background(), fileSvc, loaded, compliance.ClassC)
	t.Logf("resultSet is nil: %v", rs == nil)
	if rs != nil {
		t.Logf("results: %d, satisfied: %d, notsat: %d", len(rs.Results), rs.SatisfiedCount(), rs.NotSatisfiedCount())
	}
}
