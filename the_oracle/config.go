package the_oracle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

type ServiceConfig struct {
	Name               string   `yaml:"name,omitempty"`
	SwaggerURL         string   `yaml:"swagger_url"`
	BasePath           string   `yaml:"base_path,omitempty"`
	RouteOrigin        string   `yaml:"route_origin,omitempty"`
	RerouteDestination string   `yaml:"reroute_destination,omitempty"`
	RerouteFlag        bool     `yaml:"reroute_flag"`
	StripPrefix        bool     `yaml:"strip_prefix"`
	HijackedRoutes     []string `yaml:"hijacked_routes,omitempty"`
}

type InfraConfig struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
}

type Config struct {
	Port           int             `yaml:"port"`
	VPNProfileName string          `yaml:"vpn_profile_name,omitempty"`
	Infrastructure []InfraConfig   `yaml:"infrastructure,omitempty"`
	Services       []ServiceConfig `yaml:"services"`
}

func ExtractRoutes(doc *openapi3.T) []string {
	routeMap := make(map[string]bool)
	if doc.Paths != nil {
		for path := range doc.Paths.Map() {
			parts := strings.Split(path, "/")
			var base string
			if len(parts) > 4 {
				base = strings.Join(parts[:4], "/")
			} else {
				base = path
			}
			routeMap[base] = true
		}
	}
	var routes []string
	for route := range routeMap {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

func Hydrate(cfg *Config, configPath string) bool {
	configChanged := false
	for i, svc := range cfg.Services {
		if svc.SwaggerURL == "" {
			continue
		}
		if len(svc.HijackedRoutes) > 0 && svc.BasePath != "" && svc.RouteOrigin != "" {
			continue
		}

		u, err := url.Parse(svc.SwaggerURL)
		if err != nil {
			continue
		}

		if svc.RouteOrigin == "" {
			svc.RouteOrigin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		}
		if svc.RerouteDestination == "" {
			svc.RerouteDestination = "http://localhost:8081"
		}

		resp, err := http.Get(svc.SwaggerURL)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if svc.BasePath == "" {
			var raw map[string]interface{}
			json.Unmarshal(body, &raw)
			if bp, ok := raw["basePath"].(string); ok {
				svc.BasePath = bp
			} else {
				idx := strings.Index(u.Path, "/swagger/")
				if idx != -1 {
					svc.BasePath = u.Path[:idx]
				}
			}
		}

		if svc.Name == "" {
			if svc.BasePath != "" {
				svc.Name = strings.Trim(svc.BasePath, "/")
			} else {
				svc.Name = u.Path
			}
		}

		if len(svc.HijackedRoutes) == 0 {
			loader := openapi3.NewLoader()
			doc, err := loader.LoadFromData(body)
			if err == nil {
				svc.HijackedRoutes = ExtractRoutes(doc)
				svc.RerouteFlag = true
				svc.StripPrefix = true
			}
		}

		cfg.Services[i] = svc
		configChanged = true
	}

	if configChanged {
		outData, _ := yaml.Marshal(cfg)
		os.WriteFile(configPath, outData, 0644)
	}
	return configChanged
}

func Read(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	return &cfg, nil
}
