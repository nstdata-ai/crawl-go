package crawl_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func TestReadmeExamplesBuildAsModuleConsumers(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for _, name := range []string{"README.md", "README_zh.md"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			match := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindSubmatch(data)
			if len(match) != 2 {
				t.Fatal("missing Go quick start")
			}
			source := string(match[1])
			if first == "" {
				first = source
			} else if source != first {
				t.Fatal("bilingual quick starts differ")
			}
			dir := t.TempDir()
			module := "module example.com/crawl-consumer\n\ngo 1.20\n\nrequire github.com/nstdata-ai/crawl-go v1.0.0\nreplace github.com/nstdata-ai/crawl-go => " + strconv.Quote(root) + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-o", filepath.Join(dir, "consumer"), ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("installed consumer: %v\n%s", err, output)
			}
		})
	}
}
