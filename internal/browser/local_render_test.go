package browser

import (
	"strings"
	"testing"
)

func TestLocalPageScreenshotArgumentsStayLocalAndBounded(t *testing.T) {
	arguments := localPageScreenshotArguments(
		"/private/tmp/synthetic share.html",
		"/private/tmp/synthetic share.png",
		"/private/tmp/synthetic profile",
		1080,
		1350,
	)
	joined := strings.Join(arguments, "\n")
	for _, required := range []string{
		"--headless=new", "--window-size=1080,1350", "--screenshot=/private/tmp/synthetic share.png",
		"--user-data-dir=/private/tmp/synthetic profile", "file:///private/tmp/synthetic%20share.html",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("local screenshot arguments omit %q: %s", required, joined)
		}
	}
	for _, forbidden := range []string{"http://", "https://", "--remote-debugging"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("local screenshot arguments contain %q: %s", forbidden, joined)
		}
	}
}
