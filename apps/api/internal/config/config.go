package config

import (
	"encoding/json"
	"os"
	"strconv"
)

type Config struct {
	Addr               string `json:"addr"`
	Data               string `json:"data"`
	DB                 string `json:"db"`
	Uploads            string `json:"uploads"`
	PublicURL          string `json:"public_url"`
	DevBootstrap       bool   `json:"dev_bootstrap"`
	GitHubClientID     string `json:"github_client_id"`
	GitHubClientSecret string `json:"github_client_secret"`
	GitHubAllowedOrg   string `json:"github_allowed_org"`
	GitHubModeratorOrg string `json:"github_moderator_org"`
	GoogleClientID     string `json:"google_client_id"`
	GoogleClientSecret string `json:"google_client_secret"`
	GoogleAllowedDomain string `json:"google_allowed_domain"`
	OIDCClientID        string `json:"oidc_client_id"`
	OIDCClientSecret    string `json:"oidc_client_secret"`
	OIDCIssuer          string `json:"oidc_issuer"`
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramBotUsername string `json:"telegram_bot_username"`
	PushoverAPIToken   string `json:"pushover_api_token"`
	R2AccountID        string `json:"r2_account_id"`
	R2AccessKeyID      string `json:"r2_access_key_id"`
	R2SecretAccessKey  string `json:"r2_secret_access_key"`
	R2Endpoint         string `json:"r2_endpoint"`
}

func Defaults() Config {
	return Config{Addr: ":8080", Data: "./data", DevBootstrap: false}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	var fileBody []byte
	fileHasDevBootstrap := false
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return Config{}, err
		}
		_, fileHasDevBootstrap = fields["dev_bootstrap"]
		fileBody = body
	}
	if env := os.Getenv("CLICKCLACK_ADDR"); env != "" {
		cfg.Addr = env
	}
	if env := os.Getenv("CLICKCLACK_DATA"); env != "" {
		cfg.Data = env
	}
	if env := os.Getenv("CLICKCLACK_DB"); env != "" {
		cfg.DB = env
	}
	if env := os.Getenv("CLICKCLACK_UPLOADS"); env != "" {
		cfg.Uploads = env
	}
	if env := os.Getenv("CLICKCLACK_PUBLIC_URL"); env != "" {
		cfg.PublicURL = env
	}
	if env := os.Getenv("CLICKCLACK_DEV_BOOTSTRAP"); env != "" && !fileHasDevBootstrap {
		value, err := strconv.ParseBool(env)
		if err != nil {
			return Config{}, err
		}
		cfg.DevBootstrap = value
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_CLIENT_ID"); env != "" {
		cfg.GitHubClientID = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_CLIENT_SECRET"); env != "" {
		cfg.GitHubClientSecret = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_ALLOWED_ORG"); env != "" {
		cfg.GitHubAllowedOrg = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_MODERATOR_ORG"); env != "" {
		cfg.GitHubModeratorOrg = env
	}
	if env := os.Getenv("CLICKCLACK_GOOGLE_CLIENT_ID"); env != "" {
		cfg.GoogleClientID = env
	}
	if env := os.Getenv("CLICKCLACK_GOOGLE_CLIENT_SECRET"); env != "" {
		cfg.GoogleClientSecret = env
	}
	if env := os.Getenv("CLICKCLACK_GOOGLE_ALLOWED_DOMAIN"); env != "" {
		cfg.GoogleAllowedDomain = env
	}
	if env := os.Getenv("CLICKCLACK_OIDC_CLIENT_ID"); env != "" {
		cfg.OIDCClientID = env
	}
	if env := os.Getenv("CLICKCLACK_OIDC_CLIENT_SECRET"); env != "" {
		cfg.OIDCClientSecret = env
	}
	if env := os.Getenv("CLICKCLACK_OIDC_ISSUER"); env != "" {
		cfg.OIDCIssuer = env
	}
	if env := os.Getenv("CLICKCLACK_TELEGRAM_BOT_TOKEN"); env != "" {
		cfg.TelegramBotToken = env
	}
	if env := os.Getenv("CLICKCLACK_TELEGRAM_BOT_USERNAME"); env != "" {
		cfg.TelegramBotUsername = env
	}
	if env := os.Getenv("CLICKCLACK_PUSHOVER_API_TOKEN"); env != "" {
		cfg.PushoverAPIToken = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ACCOUNT_ID"); env != "" {
		cfg.R2AccountID = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ACCESS_KEY_ID"); env != "" {
		cfg.R2AccessKeyID = env
	}
	if env := os.Getenv("CLICKCLACK_R2_SECRET_ACCESS_KEY"); env != "" {
		cfg.R2SecretAccessKey = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ENDPOINT"); env != "" {
		cfg.R2Endpoint = env
	}
	if path == "" {
		return cfg, nil
	}
	if err := json.Unmarshal(fileBody, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Data == "" {
		cfg.Data = "./data"
	}
	return cfg, nil
}
