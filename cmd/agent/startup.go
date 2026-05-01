package main

import (
	"crobot/internal/config"
	"crobot/internal/provider"
)

func createStartupProvider(cfg *config.AgentConfig, auth config.AuthConfig) (provider.Provider, string, error) {
	cfg.HasAuthorizedProvider = auth.HasAuthorizedProvider()
	if !cfg.HasAuthorizedProvider {
		// Credentials can be absent or temporarily unavailable. Keep the user's
		// selected provider/model visible and persisted until they explicitly
		// change or clear them.
		return nil, "warning: No provider added. Add credentials to ~/.crobot/auth.json or use /login.", nil
	}

	if cfg.Provider == "" {
		return nil, "", nil
	}

	apiKey := auth.APIKey(cfg.Provider)
	if apiKey == "" {
		// Another provider may be authorized, but the selected one is not. Keep the
		// selection visible so it does not appear to reset after startup.
		return nil, "", nil
	}

	prov, err := provider.Create(cfg.Provider, apiKey)
	if err != nil {
		return nil, "", err
	}
	return prov, "", nil
}
