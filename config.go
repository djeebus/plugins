package plugins

// Config is the configuration needed to initialize a new instance of Service
type Config struct {
	Plugins []string `toml:"plugins"`
}
