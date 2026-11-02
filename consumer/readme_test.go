package consumer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

const (
	readmePath = "../README.md"
	docPath    = "../pkg/obcache/doc.go"
)

// featureTests maps each README Features bullet to the test proving it. A bullet
// with no entry fails, so the list cannot promise what nothing runs.
var featureTests = map[string]string{
	"Function wrapping":   "TestFeatureFunctionWrapping",
	"Stampede protection": "TestFeatureStampedeProtection",
	"TTL":                 "TestFeatureTTL",
	"Eviction":            "TestFeatureEviction",
	"Thread safety":       "TestFeatureThreadSafety",
	"Redis backend":       "TestFeatureRedisBackend",
	"Compression":         "TestFeatureCompression",
	"Prometheus metrics":  "TestFeaturePrometheusMetrics",
	"Statistics":          "TestFeatureStatistics",
	"Hooks":               "TestFeatureHooks",
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- paths are constants in this file
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type fence struct {
	lang string
	line int
	body string
}

func fences(text string) []fence {
	var found []fence
	var current *fence
	var body strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "```") {
			if current != nil {
				body.WriteString(line + "\n")
			}
			continue
		}
		if current == nil {
			current = &fence{lang: strings.TrimPrefix(line, "```"), line: i + 1}
			body.Reset()
			continue
		}
		current.body = body.String()
		found = append(found, *current)
		current = nil
	}
	return found
}

var (
	modulePath = regexp.MustCompile(`github\.com/1mb-dev/obcache-go(/[\w./-]*)?`)
	importLike = regexp.MustCompile(`go get |pkg\.go\.dev|"github\.com/1mb-dev/obcache-go|goreportcard`)
)

// TestDocsUseImportablePaths reads the prose as well as the fences: the install line
// that started this was in a bash fence, and a badge link is neither.
func TestDocsUseImportablePaths(t *testing.T) {
	for _, path := range []string{readmePath, docPath} {
		for i, line := range strings.Split(read(t, path), "\n") {
			if strings.Contains(line, "/internal/") {
				t.Errorf("%s:%d names an internal package: %s", path, i+1, line)
			}
			if !importLike.MatchString(line) {
				continue
			}
			for _, match := range modulePath.FindAllString(line, -1) {
				if !strings.HasPrefix(match, "github.com/1mb-dev/obcache-go/v2") {
					t.Errorf("%s:%d lacks /v2: %s", path, i+1, match)
				}
			}
		}
	}
}

func TestReadmeFenceLanguages(t *testing.T) {
	found := fences(read(t, readmePath))
	if len(found) == 0 {
		t.Fatal("no fences found; the parser or the README changed")
	}
	for _, f := range found {
		if f.lang != "go" && f.lang != "bash" {
			t.Errorf("README.md:%d has fence language %q, want go or bash", f.line, f.lang)
		}
		if f.lang == "go" && !strings.HasPrefix(f.body, "package main\n") {
			t.Errorf("README.md:%d is not a complete package main", f.line)
		}
	}
}

// TestReadmeFencesRun runs what a reader would paste. Compiling is not enough: on
// Redis a missing WithName compiles and panics at Wrap.
func TestReadmeFencesRun(t *testing.T) {
	mr := miniredis.RunT(t)
	var goFences []fence
	for _, f := range fences(read(t, readmePath)) {
		if f.lang == "go" {
			goFences = append(goFences, f)
		}
	}
	if len(goFences) == 0 {
		t.Fatal("no go fences found")
	}

	for n, f := range goFences {
		t.Run(fmt.Sprintf("line-%d", f.line), func(t *testing.T) {
			// Inside this module, so the fence resolves against its go.mod.
			dir, err := os.MkdirTemp(".", "fence")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })

			source := strings.ReplaceAll(f.body, "localhost:6379", mr.Addr())
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("go", "run", "./"+dir).CombinedOutput() // #nosec G204 -- dir is ours
			if err != nil {
				t.Fatalf("README.md:%d failed: %v\n%s", f.line, err, out)
			}
			if n == 0 && strings.TrimSpace(string(out)) != "result-123 result-123" {
				t.Errorf("Quick Start printed %q", out)
			}
		})
	}
}

// TestReadmeFeaturesAreTested fails on a bullet with no test, and on a mapped test
// that no longer exists.
func TestReadmeFeaturesAreTested(t *testing.T) {
	text := read(t, readmePath)
	section := text[strings.Index(text, "## Features"):]
	section = section[:strings.Index(section[1:], "\n## ")+1]

	bullets := regexp.MustCompile(`(?m)^- \*\*(.+?)\*\*`).FindAllStringSubmatch(section, -1)
	if len(bullets) == 0 {
		t.Fatal("no feature bullets found")
	}
	tests := read(t, "features_test.go")
	for _, b := range bullets {
		name, ok := featureTests[b[1]]
		if !ok {
			t.Errorf("README feature %q has no test", b[1])
			continue
		}
		if !strings.Contains(tests, "func "+name+"(") {
			t.Errorf("README feature %q maps to missing test %s", b[1], name)
		}
	}
}
