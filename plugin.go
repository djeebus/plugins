package plugins

import (
	"fmt"
	"path/filepath"
	"plugin"

	"github.com/hatchify/output"
)

func newPlugin(dir, key string, update bool) (pp *Plugin, err error) {
	var p Plugin
	p.importKey = key
	key, p.alias = parseKey(key)
	p.update = update

	switch {
	case filepath.Ext(key) != "":
		if len(p.alias) == 0 {
			p.alias = getPluginKey(key)
		}

		p.filename = key

	case isGitReference(key):
		p.gitURL = key
		if len(p.alias) == 0 {
			if p.alias, err = getGitPluginKey(key); err != nil {
				return
			}
		}

		// Set filename
		p.filename = filepath.Join(dir, p.alias+".so")

	default:
		err = fmt.Errorf("plugin type not supported: %s", key)
		return
	}

	p.out = output.NewWrapper(p.alias)
	pp = &p
	return
}

// Plugin represents a plugin entry
type Plugin struct {
	out *output.Wrapper
	p   *plugin.Plugin

	// Original import key
	importKey string
	// Alias given to plugin (e.g. github.com/user/myplugin would be myplugin)
	alias string
	// The git URL for the plugin
	gitURL string
	// The filename of the plugin's .so file
	filename string

	// Signals if the plugin was loaded with an active update state
	update bool
}

func (p *Plugin) retrieve() (err error) {
	if len(p.gitURL) == 0 {
		return
	}

	if doesPluginExist(p.filename) && !p.update {
		return
	}

	p.out.Notification("About to retrieve")
	var status string
	status, err = gitPull(p.gitURL)

	switch {
	case err == nil:
		if len(status) == 0 {
			p.out.Notification("Already up-to-date")
			return
		}

		p.out.Success("%s", status)

	case isDoesNotExistError(err):

	default:
		return
	}

	if err = goGet(p.gitURL, false); err != nil {
		return
	}

	p.out.Success("Download complete")
	return
}

func (p *Plugin) build() (err error) {
	if doesPluginExist(p.filename) && !p.update {
		return
	}

	if err = goBuild(p.gitURL, p.filename); err != nil {
		return
	}

	p.out.Success("Build complete")
	return
}

func (p *Plugin) init() (err error) {
	p.p, err = plugin.Open(p.filename)
	return
}
