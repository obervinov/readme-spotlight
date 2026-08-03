package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The RS_* variables only supply each flag's default value, so any flag baked
// into the image's CMD silently wins over the environment. That is how a
// deployment configured with RS_DATABASE_DSN=postgres://… kept writing to a
// container-local SQLite file and lost every config change with the next pod.
//
// The image cannot be built in unit tests, so guard the invariant at the source:
// CMD carries no flags, and the container defaults come from ENV instead.
func TestDockerfileCMDCarriesNoFlags(t *testing.T) {
	raw, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	content := string(raw)

	cmd := regexp.MustCompile(`(?m)^CMD .*$`).FindString(content)
	if cmd == "" {
		t.Fatal("no CMD instruction found in the Dockerfile")
	}
	if strings.Contains(cmd, "--") {
		t.Fatalf("CMD passes flags, which override the RS_* environment: %s", cmd)
	}

	for _, env := range []string{"RS_ADDR", "RS_DATABASE_DSN"} {
		if !strings.Contains(content, env+"=") {
			t.Errorf("Dockerfile does not set a default %s; the container would fall back to the binary's relative path", env)
		}
	}
}
