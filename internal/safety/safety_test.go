package safety

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionCodeDoesNotDependOnResearchAutomationOrSecrets(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, forbidden := range []string{
				"playwright",
				"orca",
				"COUPANG_EMAIL",
				"COUPANG_PASSWORD",
				"COUPANG_PHONE",
				"COUPANG_OTP",
			} {
				if strings.Contains(strings.ToLower(string(data)), strings.ToLower(forbidden)) {
					t.Errorf("%s contains forbidden production dependency or secret name %q", path, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
