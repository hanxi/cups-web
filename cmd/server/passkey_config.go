package main

import (
	"errors"
	"net/url"
	"strings"
)

// passkeyConfig 是 passkey（WebAuthn）功能的运行配置，全部来自环境变量。
type passkeyConfig struct {
	Enabled bool
	RPID    string
	RPName  string
	Origins []string
}

func envBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitOrigins(v string) []string {
	var out []string
	for _, o := range strings.Split(v, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, strings.TrimRight(o, "/"))
		}
	}
	return out
}

// parsePasskeyConfig 读取并校验配置。未开启时不做任何校验，保证旧部署零影响。
// PASSKEY_RP_ORIGINS 缺省时回退 TRUSTED_ORIGINS。
func parsePasskeyConfig(getenv func(string) string) (passkeyConfig, error) {
	cfg := passkeyConfig{
		Enabled: envBool(getenv("PASSKEY_ENABLED")),
		RPID:    strings.TrimSpace(getenv("PASSKEY_RP_ID")),
		RPName:  strings.TrimSpace(getenv("PASSKEY_RP_NAME")),
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	if cfg.RPName == "" {
		cfg.RPName = "CUPS Web"
	}
	if cfg.RPID == "" {
		return cfg, errors.New("PASSKEY_ENABLED=true 需要设置 PASSKEY_RP_ID（域名，不能是 IP）")
	}
	if strings.ContainsAny(cfg.RPID, "/:") {
		return cfg, errors.New("PASSKEY_RP_ID 只能是域名，不含协议、端口或路径")
	}
	if isIPLiteral(cfg.RPID) {
		return cfg, errors.New("PASSKEY_RP_ID 不能是 IP 地址，WebAuthn 要求使用域名")
	}
	cfg.Origins = splitOrigins(getenv("PASSKEY_RP_ORIGINS"))
	if len(cfg.Origins) == 0 {
		cfg.Origins = splitOrigins(getenv("TRUSTED_ORIGINS"))
	}
	if len(cfg.Origins) == 0 {
		return cfg, errors.New("需要设置 PASSKEY_RP_ORIGINS（或 TRUSTED_ORIGINS），如 https://print.example.com")
	}
	for _, o := range cfg.Origins {
		u, err := url.Parse(o)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return cfg, errors.New("无效的 origin：" + o)
		}
	}
	return cfg, nil
}

func isIPLiteral(host string) bool {
	h := strings.Trim(host, "[]")
	if strings.Contains(h, ":") {
		return true
	}
	parts := strings.Split(h, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
