package main

import (
	"crobot/internal/config"
	"crobot/internal/provider"
)

func createStartupProvider(cfg *config.AgentConfig, auth config.AuthConfig) (provider.Provider, string, error) {
	cfg.HasAuthorizedProvider = auth.HasAuthorizedProvider()
	if !cfg.HasAuthorizedProvider {
		// Do not mutate the on-disk model/provider selection during startup.
		// Credentials can be absent or temporarily unavailable, but the user's
		// saved model settings should survive until they explicitly change them.
		cfg.Provider = ""
		cfg.Model = ""
		return nil, "warning: No provider added. Add credentials to ~/.crobot/auth.json or use /login.", nil
	}

	if cfg.Provider == "" {
		return nil, "", nil
	}

	apiKey := auth.APIKey(cfg.Provider)
	if apiKey == "" {
		// Keep the persisted selection intact; only disable it for this run.
		cfg.Provider = ""
		cfg.Model = ""
		return nil, "", nil
	}

	prov, err := provider.Create(cfg.Provider, apiKey)
	if err != nil {
		return nil, "", err
	}
	return prov, "", nil
}
