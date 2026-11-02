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

// TestReadmeInstallsAsWritten pastes each Go fence into a fresh module, outside this
// one, and runs only the README's own go get lines before building it. -mod=readonly
// matters: -mod=mod or a tidy fills in the go.sum entries a reader would be missing.
func TestReadmeInstallsAsWritten(t *testing.T) {
	text := read(t, readmePath)
	install := installLines(t, text, "## Installation")
	for n, f := range goFencesBySection(text) {
		lines := append(append([]string(nil), install...), installLines(t, f.section, "")...)
		t.Run(fmt.Sprintf("line-%d", f.line), func(t *testing.T) {
			out, err := freshModule(t, f.body, lines, n == 0)
			if err != nil {
				t.Fatalf("README.md:%d does not build after %q: %v\n%s", f.line, lines, err, out)
			}
			if n == 0 && strings.TrimSpace(out) != "result-123 result-123" {
				t.Errorf("Quick Start printed %q", out)
			}
		})
	}

	// The control keeps the harness honest: installing the module root, which holds no
	// package, must still fail here, or something is quietly helping the build.
	quick := goFencesBySection(text)[0]
	if out, err := freshModule(t, quick.body, []string{"go get github.com/1mb-dev/obcache-go/v2"}, true); err == nil || !strings.Contains(out, "missing go.sum entry") {
		t.Errorf("module-root install passed (err=%v); the harness no longer runs what a reader runs\n%s", err, out)
	}
}

type sectionFence struct {
	fence
	section string
}

// goFencesBySection pairs each Go fence with the text of the section it sits in.
func goFencesBySection(text string) []sectionFence {
	lines := strings.Split(text, "\n")
	var found []sectionFence
	for _, f := range fences(text) {
		if f.lang != "go" {
			continue
		}
		start, end := f.line-1, f.line-1
		for start > 0 && !strings.HasPrefix(lines[start], "#") {
			start--
		}
		for end < len(lines) && !strings.HasPrefix(lines[end], "#") {
			end++
		}
		found = append(found, sectionFence{f, strings.Join(lines[start:end], "\n")})
	}
	return found
}

// installLines returns the commands in the bash fences under heading, or in text
// itself when heading is empty. Anything but go get fails: the harness runs nothing
// a reader would not.
func installLines(t *testing.T, text, heading string) []string {
	t.Helper()
	if heading != "" {
		i := strings.Index(text, heading+"\n")
		if i < 0 {
			t.Fatalf("README has no %q section", heading)
		}
		text = text[i:]
		text = text[:strings.Index(text[len(heading):], "\n## ")+len(heading)]
	}
	var lines []string
	for _, f := range fences(text) {
		if f.lang != "bash" {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(f.body), "\n") {
			if !strings.HasPrefix(line, "go get ") {
				t.Fatalf("install fence runs %q; only go get is allowed", line)
			}
			lines = append(lines, line)
		}
	}
	if heading != "" && len(lines) == 0 {
		t.Fatalf("%q has no go get line", heading)
	}
	return lines
}

// freshModule builds source in a new module whose only steps are init, a replace onto
// this checkout, and the given go get lines. With run set it runs the program instead.
func freshModule(t *testing.T, source string, gets []string, run bool) (string, error) {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	step := func(args ...string) (string, error) {
		cmd := exec.Command("go", args...) // #nosec G204 -- README go get lines, checked above
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := step("mod", "init", "example.com/readme"); err != nil {
		return out, err
	}
	if out, err := step("mod", "edit", "-replace", "github.com/1mb-dev/obcache-go/v2="+repo); err != nil {
		return out, err
	}
	for _, line := range gets {
		if out, err := step(strings.Fields(line)[1:]...); err != nil {
			return out, err
		}
	}
	env = append(env, "GOFLAGS=-mod=readonly")
	if run {
		return step("run", ".")
	}
	return step("build", "-o", os.DevNull, ".")
}
