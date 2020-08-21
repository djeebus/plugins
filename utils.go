package plugins

import (
	"bytes"
	"go/build"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/hatchify/errors"
)

// ParseKey returns stripped gitUrl and plugin alias
func ParseKey(key string) (newKey, alias string) {
	spl := strings.Split(key, " as ")
	// Set key as the first part of the split
	newKey = spl[0]
	// Check to see if an alias was provided
	if len(spl) > 1 {
		// Alias was provided, set the alias value
		alias = spl[1]
	} else {
		_, name := path.Split(newKey)
		alias = strings.Split(name, "-")[0]
		alias = strings.Split(alias, "@")[0]
		alias = strings.Split(alias, "#")[0]
	}
	return
}

func gitFetchTags(gitURL string) (err error) {
	gitfetch := exec.Command("git", "fetch", "--tags", "--force")
	gitfetch.Dir = getGitDir(gitURL)
	gitfetch.Stdin = os.Stdin

	outBuf := bytes.NewBuffer(nil)
	gitfetch.Stdout = outBuf

	errBuf := bytes.NewBuffer(nil)
	gitfetch.Stderr = errBuf

	if err = gitfetch.Run(); err == nil && errBuf.Len() == 0 {
		return
	}

	return
}

func gitCheckout(gitURL, branch string) (resp string, err error) {
	gitcheckout := exec.Command("git", "checkout", branch)
	gitcheckout.Dir = getGitDir(gitURL)
	gitcheckout.Stdin = os.Stdin

	outBuf := bytes.NewBuffer(nil)
	gitcheckout.Stdout = outBuf

	errBuf := bytes.NewBuffer(nil)
	gitcheckout.Stderr = errBuf

	if err = gitcheckout.Run(); err == nil && errBuf.Len() == 0 {
		resp = outBuf.String()
		return
	}

	errStr := errBuf.String()
	switch {
	case errStr == "":
		return
	case strings.Index(errStr, "Already on") > -1:
		return

	case strings.Index(errStr, "Switched to") > -1:
		resp = errBuf.String()
		return

	default:
		err = errors.Error(errBuf.String())
		return
	}
}

func gitPull(gitURL string) (resp string, err error) {
	gitpull := exec.Command("git", "pull", "origin")
	gitpull.Dir = getGitDir(gitURL)
	gitpull.Stdin = os.Stdin

	outBuf := bytes.NewBuffer(nil)
	gitpull.Stdout = outBuf

	errBuf := bytes.NewBuffer(nil)
	gitpull.Stderr = errBuf

	if err = gitpull.Run(); err != nil {
		if errBuf.Len() > 0 {
			err = errors.Error(errBuf.String())
		}

		return
	}

	outStr := outBuf.String()
	if strings.Index(outStr, "up to date") > -1 {
		return
	}

	resp = outStr
	return
}

func updatePluginDependencies(gitURL string) (err error) {
	args := []string{"mod", "download"}
	update := exec.Command("go", args...)
	update.Stdin = os.Stdin
	update.Stdout = os.Stdout
	update.Dir = getGoDir(gitURL)

	errBuf := bytes.NewBuffer(nil)
	update.Stderr = errBuf

	if err = update.Run(); err != nil {
		return errors.Error(errBuf.String())
	}

	return
}

func goGet(gitURL string) (err error) {
	args := []string{"get", "-v", "-d", "-buildmode=plugin", gitURL}
	goget := exec.Command("go", args...)
	goget.Stdin = os.Stdin
	goget.Stdout = os.Stdout

	errBuf := bytes.NewBuffer(nil)
	goget.Stderr = errBuf

	if err = goget.Run(); err != nil {
		return errors.Error(errBuf.String())
	}

	return
}

func goBuild(gitURL, filename string) (err error) {
	curDir, _ := os.Getwd()
	target := path.Join(curDir, filename)

	// Build in local directory with target filepath instead of target directory with build path.
	gobuild := exec.Command("go", "build", "-trimpath", "-buildmode=plugin", "-o", target)
	// Workaround for https://github.com/golang/go/issues/27751
	gobuild.Dir = getGoDir(gitURL)

	gobuild.Stdin = os.Stdin
	gobuild.Stdout = os.Stdout
	gobuild.Stderr = os.Stderr

	errBuf := bytes.NewBuffer(nil)
	gobuild.Stderr = errBuf

	if err = gobuild.Run(); err != nil {
		if errBuf.Len() > 0 {
			err = errors.Error(errBuf.String())
		}

		return
	}

	return
}

func goTest(gitURL string) (pass bool, err error) {
	// Test in local directory with target filepath instead of target directory with build path.
	goTest := exec.Command("go", "test")
	// Workaround for https://github.com/golang/go/issues/27751
	goTest.Dir = getGoDir(gitURL)

	goTest.Stdin = os.Stdin
	outBuf := bytes.NewBuffer(nil)
	goTest.Stdout = outBuf
	goTest.Stderr = os.Stderr

	errBuf := bytes.NewBuffer(nil)
	goTest.Stderr = errBuf

	if err = goTest.Run(); err != nil {
		err = errors.Error(errBuf.String())
		return
	}

	pass = strings.Contains(outBuf.String(), "PASS")
	return
}

func getGoDir(gitURL string) (goDir string) {
	homeDir := os.Getenv("HOME")
	return path.Join(homeDir, "go", "src", gitURL)
}

func getGitDir(gitURL string) (goDir string) {
	homeDir := os.Getenv("HOME")
	spl := strings.Split(gitURL, "/")

	var parts []string
	parts = append(parts, homeDir)
	parts = append(parts, "go")
	parts = append(parts, "src")

	if len(spl) > 0 {
		// Append host
		parts = append(parts, spl[0])
	}

	if len(spl) > 1 {
		// Append git user
		parts = append(parts, spl[1])
	}

	if len(spl) > 2 {
		// Append repo name
		parts = append(parts, spl[2])
	}

	return path.Join(parts...)
}

func trimSlash(in string) (out string) {
	if len(in) == 0 {
		return
	}

	if in[len(in)-1] != '/' {
		return in
	}

	return in[:len(in)-1]
}

func doesPluginExist(filename string) (exists bool) {
	info, err := os.Stat(filename)
	if err != nil {
		return
	}

	// Something exists at the provided filename, if it's not a directory - we're good!
	return !info.IsDir()
}

func getGitPluginKey(gitURL string) (key, branch string, err error) {
	_, key, branch, err = getGitURLParts(gitURL)
	return
}

func getGitURLParts(gitURL string) (gitUser, repoName, branch string, err error) {
	// Attempt to parse version first
	comps := strings.Split(gitURL, "@")

	// Parse url/branch
	var u *url.URL
	if u, err = url.Parse("http://" + comps[0]); err != nil {
		return
	}

	// Split parts
	parts := stripEmpty(strings.Split(u.Path, "/"))
	gitUser = parts[0]
	repoName = parts[1]

	if len(comps) > 1 {
		// Optional Version
		branch = comps[1]
	} else {
		// Optional Branch
		branch = u.Fragment
	}

	return
}

func stripEmpty(ss []string) (out []string) {
	for _, str := range ss {
		if len(str) == 0 {
			continue
		}

		out = append(out, str)
	}

	return
}

func getPluginKey(filename string) (key string) {
	base := filepath.Base(filename)
	spl := strings.Split(base, ".")
	key = spl[0]
	return
}

func getKeyFromGitURL(gitURL string) (key string, err error) {
	var u *url.URL
	if u, err = url.Parse("http://" + gitURL); err != nil {
		return
	}

	key = filepath.Base(u.Path)
	return
}

func getHandlerParts(handlerKey string) (key, handler string, args []string, err error) {
	spl := strings.Split(handlerKey, ".")
	key = spl[0]
	handler = spl[1]

	spl = strings.Split(handler, "(")
	if len(spl) == 1 {
		return
	}

	handler = spl[0]
	argsStr := spl[1]
	if argsStr[len(argsStr)-1] != ')' {
		err = ErrExpectedEndParen
		return
	}

	argsStr = argsStr[:len(argsStr)-1]
	args = strings.Split(argsStr, ",")
	return
}

func isGitReference(handlerKey string) (ok bool) {
	var err error
	_, err = url.Parse("http://" + handlerKey)
	return err == nil
}

func closePlugin(p *plugin.Plugin) (err error) {
	var sym plugin.Symbol
	if sym, err = p.Lookup("Close"); err != nil {
		err = nil
		return
	}

	fn, ok := sym.(func() error)
	if !ok {
		return
	}

	return fn()
}

func wrapProcess(fn func() error, ch chan error) {
	ch <- fn()
}

func waitForProcesses(ch chan error, count int) (err error) {
	var n int
	for err = range ch {
		if err != nil {
			return
		}

		if n++; n == count {
			break
		}
	}

	return
}

func isDoesNotExistError(err error) (ok bool) {
	if err == nil {
		return
	}

	str := strings.ToLower(err.Error())
	return strings.Index(str, "no such file or directory") > -1
}

func removeBranchHash(gitURL string) (out string) {
	spl := strings.Split(gitURL, "#")
	spl = strings.Split(spl[0], "@")
	out = spl[0]
	return
}

func doesPluginSourceExist(gitURL string) (exists bool) {
	gopath := path.Clean(getGoPath())
	dir := path.Join(gopath, "src", gitURL)
	info, err := os.Stat(dir)
	if err != nil {
		return
	}

	return info.IsDir()
}

func getGoPath() (gopath string) {
	if gopath = os.Getenv("GOPATH"); len(gopath) > 0 {
		return
	}

	return build.Default.GOPATH
}
