package nginx

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Directive 表示一个最基本的Nginx指令
type Directive struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
}

// LocationBlock 描述 location {...} 结构
type LocationBlock struct {
	Path       string       `json:"path"`
	Directives []Directive  `json:"directives"`
	Proxy      *ProxyConfig `json:"proxy"`
}

// ProxyConfig 代理转发配置
type ProxyConfig struct {
	Enable       bool              `json:"enable"`
	Pass         string            `json:"pass"`
	Websocket    bool              `json:"websocket"`
	Headers      map[string]string `json:"headers"`
	PreserveHost bool              `json:"preserve_host"`
}

// PHPConfig PHP-FPM相关配置
type PHPConfig struct {
	FastCGIPass string   `json:"fastcgi_pass"`
	Index       []string `json:"index"`
}

// SSLConfig SSL证书相关配置
type SSLConfig struct {
	Enabled        bool     `json:"enabled"`
	Certificate    string   `json:"certificate"`
	CertificateKey string   `json:"certificate_key"`
	SessionCache   string   `json:"session_cache"`
	Protocols      []string `json:"protocols"`
	Ciphers        []string `json:"ciphers"`
}

// ServerBlock 表示 server {...}
type ServerBlock struct {
	Listen        []string        `json:"listen"`
	ServerNames   []string        `json:"server_names"`
	Root          string          `json:"root"`
	Index         []string        `json:"index"`
	AccessLog     string          `json:"access_log"`
	ErrorLog      string          `json:"error_log"`
	Proxy         *ProxyConfig    `json:"proxy"`
	PHP           *PHPConfig      `json:"php"`
	Locations     []LocationBlock `json:"locations"`
	SSL           *SSLConfig      `json:"ssl"`
	ForceSSL      bool            `json:"force_ssl"`
	ChallengeRoot string          `json:"challenge_root"`
	Extra         []Directive     `json:"extra"`
}

// NginxConfig 表示一个完整的nginx配置文件
type NginxConfig struct {
	FilePath string         `json:"file_path"`
	Servers  []*ServerBlock `json:"servers"`
}

// Render 将配置渲染为Nginx语法
func (cfg *NginxConfig) Render() (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置未初始化")
	}

	var buf bytes.Buffer
	for _, server := range cfg.Servers {
		if err := serverBlockTpl.Execute(&buf, server.templateData()); err != nil {
			return "", fmt.Errorf("渲染配置失败: %w", err)
		}
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

func (sb *ServerBlock) templateData() map[string]interface{} {
	data := map[string]interface{}{
		"Listen":        sb.Listen,
		"ServerNames":   sb.ServerNames,
		"Root":          sb.Root,
		"Index":         sb.Index,
		"AccessLog":     sb.AccessLog,
		"ErrorLog":      sb.ErrorLog,
		"Proxy":         sb.Proxy,
		"PHP":           sb.PHP,
		"Locations":     sb.Locations,
		"SSL":           sb.SSL,
		"ForceSSL":      sb.ForceSSL,
		"ChallengeRoot": sb.ChallengeRoot,
		"Extra":         sb.Extra,
	}
	return data
}

var serverBlockTpl = template.Must(
	template.New("server_block").Funcs(template.FuncMap{
		"join": func(items []string) string {
			return strings.Join(items, " ")
		},
		"hasListen": func(list []string, flag string) bool {
			for _, l := range list {
				if strings.Contains(l, flag) {
					return true
				}
			}
			return false
		},
	}).Parse(serverBlockTemplate),
)

const serverBlockTemplate = `
server {
	{{- range .Listen }}
	listen {{ . }};
	{{- end }}

	server_name {{ join .ServerNames }};

	{{- if .ForceSSL }}
	if ($scheme = http) {
		return 301 https://$host$request_uri;
	}
	{{- end }}

	{{- if .Root }}
	root {{ .Root }};
	{{- end }}

	{{- if .Index }}
	index {{ join .Index }};
	{{- end }}

	{{- if .AccessLog }}
	access_log {{ .AccessLog }};
	{{- end }}

	{{- if .ErrorLog }}
	error_log {{ .ErrorLog }};
	{{- end }}

	location ^~ /.well-known/acme-challenge/ {
		root {{ .ChallengeRoot }};
		default_type "text/plain";
		try_files $uri =404;
	}

	{{- if .Proxy }}
	location / {
		proxy_pass {{ .Proxy.Pass }};
		proxy_set_header Host $host;
		proxy_set_header X-Real-IP $remote_addr;
		proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
		proxy_set_header X-Forwarded-Proto $scheme;
		{{- if .Proxy.Websocket }}
		proxy_http_version 1.1;
		proxy_set_header Upgrade $http_upgrade;
		proxy_set_header Connection "upgrade";
		{{- end }}
		{{- range $key, $val := .Proxy.Headers }}
		proxy_set_header {{ $key }} {{ $val }};
		{{- end }}
	}
	{{- else }}
	location / {
		try_files $uri $uri/ /index.php$is_args$args;
	}
	{{- end }}

	{{- if .PHP }}
	location ~ \.php$ {
		include fastcgi_params;
		fastcgi_index index.php;
		fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
		fastcgi_pass {{ .PHP.FastCGIPass }};
	}
	{{- end }}

	{{- range .Locations }}
	location {{ .Path }} {
		{{- range .Directives }}
		{{ .Name }} {{ join .Params }};
		{{- end }}
		{{- if .Proxy }}
		proxy_pass {{ .Proxy.Pass }};
		{{- end }}
	}
	{{- end }}

	{{- if .SSL.Enabled }}
	ssl_certificate {{ .SSL.Certificate }};
	ssl_certificate_key {{ .SSL.CertificateKey }};
	ssl_session_cache {{ .SSL.SessionCache }};
	ssl_session_timeout 10m;
	ssl_protocols {{ join .SSL.Protocols }};
	ssl_ciphers {{ join .SSL.Ciphers }};
	ssl_prefer_server_ciphers on;
	{{- end }}

	{{- range .Extra }}
	{{ .Name }} {{ join .Params }};
	{{- end }}
}
`
