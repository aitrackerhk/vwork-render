package handlers

import "nwork/config"

var appCfg *config.Config

// SetAppConfig should be called once at startup.
func SetAppConfig(c *config.Config) {
	appCfg = c
}

func mustAppConfig() *config.Config {
	if appCfg == nil {
		panic("handlers app config not initialized: call handlers.SetAppConfig")
	}
	return appCfg
}





