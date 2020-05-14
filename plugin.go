package plugins

import (
	"fmt"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/hatchify/scribe"
)

func newPlugin(dir, key string, update bool) (pp *Plugin, err error) {
	var p Plugin
	p.importKey = key
	key, p.alias = parseKey(key)
	p.update = update

	if isGitReference(key) {
		// Vpm case?
		var repoName string
		if _, repoName, p.branch, err = getGitURLParts(key); err != nil {
			return
		}

		if len(p.alias) == 0 {
			p.alias = repoName
		}

		// Set gitURL
		p.gitURL = removeBranchHash(key)

		// Set filename
		p.filename = filepath.Join(dir, p.alias+".so")

	} else {
		// Vroomy case?
		switch filepath.Ext(key) {
		case ".so":
			// Handle plugin binary
			if len(p.alias) == 0 {
				p.alias = getPluginKey(key)
			}

			p.filename = key
		default:
			err = fmt.Errorf("plugin type not supported: %s", key)
			return
		}
	}

	p.out = scribe.New(p.alias)
	pp = &p
	return
}

// Plugin represents a plugin entry
type Plugin struct {
	out *scribe.Scribe
	p   *plugin.Plugin

	// Original import key
	importKey string
	// Alias given to plugin (e.g. github.com/user/myplugin would be myplugin)
	alias string
	// The git URL for the plugin
	gitURL string
	// The filename of the plugin's .so file
	filename string
	// The target branch of the plugin
	branch string

	// Signals if the plugin was loaded with an active update state
	update bool
}

func (p *Plugin) updatePlugin() (err error) {
	if len(p.gitURL) == 0 {
		return
	}

	if !doesPluginSourceExist(p.gitURL) {
		if err = goGet(p.gitURL); err != nil {
			return
		}
	}

	if len(p.branch) > 0 {
		var shouldPull bool
		shouldPull, err = p.setTargetBranch()
		if !shouldPull || err != nil {
			// Ignore pull for explicit versions and checkouts with errors
			return
		}
	}

	// Ensure we're up to date with the given branch
	var status string
	if status, err = gitPull(p.gitURL); err == nil {
		if len(status) != 0 {
			p.out.Notification("Updated to latest ref.")
		} else {
			// Already had these refs
			p.out.Success("Already up to date.")
			return
		}
	}

	// Grab latest deps
	// TODO: only download changed deps?
	return p.updateDependencies()
}

func (p *Plugin) setTargetBranch() (shouldPull bool, err error) {
	gitFetch(p.gitURL)

	if err = p.checkout(); err != nil {
		// Err is expected when setting an explicit version
		if !strings.Contains(err.Error(), "HEAD is now at") {
			p.out.Errorf("Error checking out %s: %+v", p.branch, err)
			return
		}

		p.out.Notificationf("Set version: %s", p.branch)

		// No need to pull
		return false, p.updateDependencies()
	}

	// Currently tracking release channel or current branch, needs pull
	return true, nil
}

func (p *Plugin) checkout() (err error) {
	var status string
	if status, err = gitCheckout(p.gitURL, p.branch); err != nil {
		return
	} else if len(status) != 0 {
		p.out.Notificationf("Switched to \"%s\" branch", p.branch)
	}

	return
}

func (p *Plugin) updateDependencies() (err error) {
	p.out.Notification("Updating dependencies...")

	// Ensure we have all the current dependencies
	if err = updatePluginDependencies(p.gitURL); err != nil {
		p.out.Errorf("Failed to update dependencies %v", err)
		return
	}

	p.out.Success("Dependencies downloaded!")
	return
}

func (p *Plugin) build() (err error) {
	if err = goBuild(p.gitURL, p.filename); err != nil {
		return
	}

	p.out.Success("Build complete!")
	return
}

func (p *Plugin) test() (err error) {
	if doesPluginExist(p.filename) && !p.update {
		return
	}

	var pass bool
	if pass, err = goTest(p.gitURL); err != nil {
		p.out.Error("Test failed :(")
		return fmt.Errorf("%s failed test", p.alias)
	}

	if pass {
		p.out.Success("Test passed!")
	} else {
		p.out.Warning("No test files")
	}

	return
}

func (p *Plugin) init() (err error) {
	p.p, err = plugin.Open(p.filename)
	return
}
