// Command release validates a reviewed commit and publishes its Go module tag.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const repository = "nstdata-ai/crawl-go"
const modulePath = "github.com/" + repository

type command func(context.Context, string, string, []string, []string) ([]byte, error)

type release struct {
	root, version, commit, token string
	env                          map[string]string
	client                       *http.Client
	run                          command
}

func execute(ctx context.Context, dir, name string, args, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, env
	output, err := cmd.Output()
	if err != nil {
		operation := filepath.Base(name)
		if len(args) > 0 {
			operation += " " + args[0]
		}
		if len(args) > 1 && args[0] == "mod" {
			operation += " " + args[1]
		}
		return nil, fmt.Errorf("%s failed; inspect this stage before another run", operation)
	}
	return output, nil
}

func hostEnvironment() []string {
	var env []string
	for _, name := range []string{"PATH", "TMPDIR", "SystemRoot"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func (r release) source(ctx context.Context) error {
	if r.env["GITHUB_EVENT_NAME"] != "workflow_dispatch" ||
		r.env["GITHUB_REPOSITORY"] != repository || r.env["GITHUB_REF"] != "refs/heads/main" {
		return errors.New("release requires manual dispatch from this repository's main branch")
	}
	if !regexp.MustCompile(`^v1\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`).MatchString(r.version) ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(r.commit) || r.env["GITHUB_SHA"] != r.commit {
		return errors.New("stable v1 version and full matching reviewed commit required")
	}
	for _, name := range []string{"README.md", "README_zh.md"} {
		data, err := os.ReadFile(filepath.Join(r.root, name))
		if err != nil || !strings.Contains(string(data), "\ngo get "+modulePath+"@"+r.version+"\n") {
			return errors.New("README installation version differs")
		}
	}
	data, err := os.ReadFile(filepath.Join(r.root, "go.mod"))
	fields := strings.Fields(string(data))
	if err != nil || len(fields) < 2 || fields[0] != "module" || fields[1] != modulePath {
		return errors.New("module path differs")
	}
	data, err = os.ReadFile(filepath.Join(r.root, "CHANGELOG.md"))
	if err != nil || !strings.Contains(string(data), "\n## "+r.version+"\n") {
		return errors.New("stable changelog version differs")
	}
	for _, args := range [][]string{{"rev-parse", "HEAD"}, {"status", "--porcelain", "--untracked-files=no"}} {
		output, err := r.run(ctx, r.root, "git", args, hostEnvironment())
		if err != nil {
			return err
		}
		if (args[0] == "rev-parse" && strings.TrimSpace(string(output)) != r.commit) ||
			(args[0] == "status" && len(bytes.TrimSpace(output)) != 0) {
			return errors.New("reviewed source changed")
		}
	}
	return nil
}

func (r release) api(ctx context.Context, method, path string, input, output any) (int, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return 0, errors.New("invalid release request")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com/repos/"+repository+path, body)
	if err != nil {
		return 0, errors.New("invalid release request")
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := r.client.Do(req)
	if err != nil {
		return 0, errors.New("GitHub release request failed; inspect tag state before another run")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return response.StatusCode, nil
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return response.StatusCode, fmt.Errorf("GitHub release API returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
	if err != nil || len(data) > 1048576 || json.Unmarshal(data, output) != nil {
		return response.StatusCode, errors.New("invalid GitHub release response")
	}
	return response.StatusCode, nil
}

func (r release) environment(ctx context.Context) error {
	if r.token == "" {
		return errors.New("GitHub workflow token required")
	}
	var settings struct {
		CanAdminsBypass *bool `json:"can_admins_bypass"`
		ProtectionRules []struct {
			Type      string            `json:"type"`
			Reviewers []json.RawMessage `json:"reviewers"`
		} `json:"protection_rules"`
		DeploymentBranchPolicy struct {
			Custom    *bool `json:"custom_branch_policies"`
			Protected *bool `json:"protected_branches"`
		} `json:"deployment_branch_policy"`
	}
	status, err := r.api(ctx, http.MethodGet, "/environments/go-production", nil, &settings)
	if err != nil {
		return err
	}
	reviewer := false
	for _, rule := range settings.ProtectionRules {
		reviewer = reviewer || (rule.Type == "required_reviewers" && len(rule.Reviewers) > 0)
	}
	policy := settings.DeploymentBranchPolicy
	if status != 200 || settings.CanAdminsBypass == nil || *settings.CanAdminsBypass || !reviewer ||
		policy.Custom == nil || !*policy.Custom || policy.Protected == nil || *policy.Protected {
		return errors.New("configure go-production reviewers, no administrator bypass and selected branches")
	}
	var policies struct {
		Total int                           `json:"total_count"`
		Rules []struct{ Name, Type string } `json:"branch_policies"`
	}
	status, err = r.api(ctx, http.MethodGet, "/environments/go-production/deployment-branch-policies?per_page=100", nil, &policies)
	if err != nil {
		return err
	}
	if status != 200 || policies.Total != 1 || len(policies.Rules) != 1 ||
		policies.Rules[0].Name != "main" || policies.Rules[0].Type != "branch" {
		return errors.New("allow only the main branch in go-production")
	}
	return nil
}

type reference struct {
	Ref    string
	Object struct{ SHA, Type string }
}

func (r release) ref(ctx context.Context, path, want string, absent bool) error {
	var ref reference
	status, err := r.api(ctx, http.MethodGet, "/git/ref/"+path, nil, &ref)
	if err != nil {
		return err
	}
	if absent {
		if status != 404 {
			return errors.New("version tag already exists; do not overwrite or repeat publication")
		}
	} else if status != 200 || ref.Object.Type != "commit" || ref.Object.SHA != want {
		return errors.New("remote reference differs from the reviewed commit")
	}
	return nil
}

type downloaded struct {
	Path, Version, Dir, Sum, GoModSum string
	Origin                            struct{ VCS, URL, Hash string }
}

func goEnvironment(folder, config, proxy string) []string {
	return append(hostEnvironment(), "GOENV=off", "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=",
		"GOPATH="+filepath.Join(folder, "gopath"), "GOMODCACHE="+filepath.Join(folder, "modcache"),
		"GOCACHE="+filepath.Join(folder, "buildcache"), "GOPROXY="+proxy, "GOSUMDB=sum.golang.org",
		"GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=", "GOINSECURE=", "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+config, "NETRC="+os.DevNull)
}

func (r release) download(ctx context.Context, folder string, env []string) (downloaded, error) {
	var result downloaded
	output, err := r.run(ctx, folder, filepath.Join(runtime.GOROOT(), "bin", "go"),
		[]string{"mod", "download", "-json", modulePath + "@" + r.version}, env)
	if err != nil {
		return result, err
	}
	if json.Unmarshal(output, &result) != nil || result.Path != modulePath ||
		result.Version != r.version || result.Dir == "" || !strings.HasPrefix(result.Sum, "h1:") ||
		!strings.HasPrefix(result.GoModSum, "h1:") {
		return result, errors.New("invalid public module download metadata")
	}
	relative, err := filepath.Rel(filepath.Join(folder, "modcache"), result.Dir)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return result, errors.New("module download escaped its isolated cache")
	}
	return result, nil
}

func (r release) verify(ctx context.Context) error {
	if err := r.ref(ctx, "tags/"+r.version, r.commit, false); err != nil {
		return err
	}
	folder, err := os.MkdirTemp("", "nstdata-go-public-install-")
	if err != nil {
		return errors.New("cannot create isolated installation directory")
	}
	defer os.RemoveAll(folder)
	config := filepath.Join(folder, "gitconfig")
	if err := os.WriteFile(config, []byte("[credential]\n helper =\n"), 0600); err != nil {
		return errors.New("cannot create isolated Git configuration")
	}
	directDir, proxyDir := filepath.Join(folder, "direct"), filepath.Join(folder, "proxy")
	for _, dir := range []string{directDir, proxyDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			return errors.New("cannot create isolated consumer directory")
		}
	}
	direct, err := r.download(ctx, directDir, goEnvironment(directDir, config, "direct"))
	if err != nil {
		return err
	}
	if direct.Origin.VCS != "git" || direct.Origin.Hash != r.commit ||
		(direct.Origin.URL != "https://github.com/"+repository && direct.Origin.URL != "https://github.com/"+repository+".git") {
		return errors.New("direct module origin does not identify the reviewed commit")
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	env := goEnvironment(proxyDir, config, "https://proxy.golang.org,direct")
	if _, err := r.run(ctx, proxyDir, goBin, []string{"mod", "init", "example.com/crawl-release-check"}, env); err != nil {
		return err
	}
	// source has already matched this literal command against both READMEs.
	if _, err := r.run(ctx, proxyDir, goBin, []string{"get", modulePath + "@" + r.version}, env); err != nil {
		return err
	}
	installed, err := r.download(ctx, proxyDir, env)
	if err != nil {
		return err
	}
	if direct.Sum != installed.Sum || direct.GoModSum != installed.GoModSum {
		return errors.New("public module checksums differ between source and default installation")
	}
	tracked, err := r.run(ctx, r.root, "git", []string{"ls-files", "-z"}, hostEnvironment())
	if err != nil {
		return err
	}
	for _, name := range strings.Split(strings.TrimSuffix(string(tracked), "\x00"), "\x00") {
		if name != "go.mod" && name != "LICENSE" && !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".md") {
			continue
		}
		expected, err := os.ReadFile(filepath.Join(r.root, name))
		if err != nil {
			return errors.New("cannot read reviewed source")
		}
		for _, dir := range []string{direct.Dir, installed.Dir} {
			actual, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || !bytes.Equal(expected, actual) {
				return errors.New("installed module source differs from the reviewed commit")
			}
		}
	}
	readme, err := os.ReadFile(filepath.Join(r.root, "README.md"))
	if err != nil {
		return errors.New("cannot read the documented program")
	}
	program := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindSubmatch(readme)
	if len(program) != 2 || os.WriteFile(filepath.Join(proxyDir, "main.go"), program[1], 0600) != nil {
		return errors.New("cannot prepare the documented consumer program")
	}
	_, err = r.run(ctx, proxyDir, goBin,
		[]string{"build", "-mod=readonly", "-o", filepath.Join(proxyDir, "consumer"), "."}, env)
	return err
}

func (r release) perform(ctx context.Context, mode string) error {
	if mode != "check" && mode != "tag" && mode != "verify" {
		return errors.New("use check, tag or verify")
	}
	if err := r.source(ctx); err != nil {
		return err
	}
	if mode == "verify" {
		return r.verify(ctx)
	}
	if err := r.environment(ctx); err != nil {
		return err
	}
	if err := r.ref(ctx, "heads/main", r.commit, false); err != nil {
		return err
	}
	if err := r.ref(ctx, "tags/"+r.version, "", true); err != nil {
		return err
	}
	if mode == "tag" {
		var ref reference
		status, err := r.api(ctx, http.MethodPost, "/git/refs",
			map[string]string{"ref": "refs/tags/" + r.version, "sha": r.commit}, &ref)
		if err != nil {
			return err
		}
		if status != 201 || ref.Ref != "refs/tags/"+r.version || ref.Object.SHA != r.commit || ref.Object.Type != "commit" {
			return errors.New("tag creation was not confirmed; inspect GitHub before another run")
		}
		return r.ref(ctx, "tags/"+r.version, r.commit, false)
	}
	return nil
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot locate release checkout")
		os.Exit(1)
	}
	env := map[string]string{}
	for _, name := range []string{"GITHUB_EVENT_NAME", "GITHUB_REPOSITORY", "GITHUB_REF", "GITHUB_SHA"} {
		env[name] = os.Getenv(name)
	}
	r := release{root: root, version: os.Getenv("RELEASE_VERSION"), commit: os.Getenv("RELEASE_COMMIT"),
		token: os.Getenv("GH_TOKEN"), env: env, run: execute,
		client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}
	mode := ""
	if len(os.Args) == 2 {
		mode = os.Args[1]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := r.perform(ctx, mode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Release stopped. Inspect the tag and module before another run.")
		os.Exit(1)
	}
	fmt.Println("PASS: Go module release " + mode)
}
