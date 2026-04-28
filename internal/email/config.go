package email

import (
	"errors"
	"nwork/config"
)

var cfg *config.Config

// SetConfig should be called once at startup (api + worker).
func SetConfig(c *config.Config) {
	cfg = c
}

func GetConfig() (*config.Config, error) {
	if cfg == nil {
		return nil, errors.New("email config not initialized: call email.SetConfig")
	}
	return cfg, nil
}

// mustCfg returns the config or panics if not initialized.
func mustCfg() *config.Config {
	if cfg == nil {
		panic("email config not initialized: call email.SetConfig first")
	}
	return cfg
}
