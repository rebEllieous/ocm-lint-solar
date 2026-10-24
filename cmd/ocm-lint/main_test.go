package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
)

const digest = `,"digest":{"hashAlgorithm":"SHA-256","normalisationAlgorithm":"genericBlobDigest/v1","value":"00"}`

func withDigest(res string) string { return res[:len(res)-1] + digest + "}" }

func TestExitCodes(t *testing.T) {
	clean := fixture.Descriptor(t, fixture.Component("acme.org/clean", "1.0.0",
		withDigest(fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1")), ""))
	warnOnly := fixture.Descriptor(t, fixture.Component("acme.org/warn", "1.0.0",
		fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1"), "")) // no digest: warning
	broken := fixture.Descriptor(t, fixture.Component("acme.org/broken", "1.0.0", "",
		fixture.Ref("dep", "acme.org/gone", "1.0.0"))) // unresolvable: error
	ctf := fixture.CTF(t, clean, warnOnly, broken)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"clean", []string{ctf + "//acme.org/clean:1.0.0"}, 0},
		{"warnings only", []string{ctf + "//acme.org/warn:1.0.0"}, 0},
		{"error finding", []string{ctf + "//acme.org/broken:1.0.0"}, 1},
		{"air-gap pack finds external image", []string{"--rule", "air-gap", ctf + "//acme.org/clean:1.0.0"}, 1},
		{"allow-host clears it", []string{"--rule", "air-gap", "--allow-host", "ghcr.io", ctf + "//acme.org/clean:1.0.0"}, 0},
		{"unknown rule", []string{"--rule", "nope", ctf + "//acme.org/clean:1.0.0"}, 2},
		{"missing component", []string{ctf + "//acme.org/missing:1.0.0"}, 2},
		{"bad ref", []string{"not-a-ref"}, 2},
		{"no args", nil, 2},
		{"tool error beats error finding", []string{ctf + "//acme.org/broken:1.0.0", ctf + "//acme.org/missing:1.0.0"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(context.Background(), tt.args, &out, &errOut); got != tt.want {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", got, tt.want, out.String(), errOut.String())
			}
		})
	}
}

func TestRecursiveJSON(t *testing.T) {
	dep := fixture.Descriptor(t, fixture.Component("acme.org/dep", "1.0.0", fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1"), ""))
	app := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", "", fixture.Ref("dep", "acme.org/dep", "1.0.0")))
	ctf := fixture.CTF(t, dep, app)

	var out bytes.Buffer
	run(context.Background(), []string{"--recursive", "-o", "json", ctf + "//acme.org/app:1.0.0"}, &out, &bytes.Buffer{})
	var got struct {
		Findings []struct {
			Component, Rule string
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(got.Findings) != 1 || got.Findings[0].Component != "acme.org/dep:1.0.0" || got.Findings[0].Rule != "core/digest-present" {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

func TestListRules(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--list-rules"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, id := range []string{"core/schema-valid", "air-gap/references-closed", "solar/values-template-present"} {
		if !strings.Contains(out.String(), id) {
			t.Fatalf("--list-rules misses %s:\n%s", id, out.String())
		}
	}
}
