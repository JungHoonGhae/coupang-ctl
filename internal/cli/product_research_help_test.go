package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestProductResearchHelpWithoutBrowserConfiguration(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-invalid-state")
	for _, command := range []string{"search", "inspect", "recommend", "report"} {
		for _, option := range []string{"--help", "-h"} {
			t.Run(command+option, func(t *testing.T) {
				var out, diagnostic bytes.Buffer
				if err := Run(context.Background(), []string{"products", command, option}, &out, &diagnostic, "test"); err != nil {
					t.Fatal(err)
				}
				var got struct {
					SchemaVersion int    `json:"schema_version"`
					Usage         string `json:"usage"`
					Options       []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
					} `json:"options"`
				}
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.SchemaVersion != 1 || got.Usage == "" || len(got.Options) == 0 || diagnostic.Len() != 0 {
					t.Fatal("missing structured help")
				}
				want := "--category-id"
				if command == "inspect" {
					want = "--product-id"
				}
				if command == "report" {
					want = "--input"
				}
				found := false
				for _, o := range got.Options {
					if o.Name == want && o.Description != "" {
						found = true
					}
				}
				if !found {
					t.Fatal("command's registered option missing from help")
				}
			})
		}
	}
}

func TestOrderStatsHelpWithoutBrowserOrLedger(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-invalid-state")
	for _, option := range []string{"--help", "-h"} {
		var out, diagnostic bytes.Buffer
		if err := Run(context.Background(), []string{"orders", "stats", option}, &out, &diagnostic, "test"); err != nil {
			t.Fatal("stats help acquired dependencies or returned an error")
		}
		var got struct {
			Usage   string `json:"usage"`
			Options []struct {
				Name string `json:"name"`
			} `json:"options"`
		}
		if json.Unmarshal(out.Bytes(), &got) != nil || got.Usage == "" || len(got.Options) != 2 || diagnostic.Len() != 0 {
			t.Fatal("stats help did not return its registered date options")
		}
	}
}
