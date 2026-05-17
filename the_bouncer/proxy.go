package the_bouncer

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"proxymaxxing/the_oracle"
	"sort"
	"strings"
	"time"
)

type LogEvent struct {
	Service string
	Method  string
	Path    string
	Dest    string
	Local   bool
	Status  int
	Time    time.Time
}

func Setup(cfg *the_oracle.Config, logChan chan LogEvent) *httputil.ReverseProxy {
	// Sort services by prefix length descending to ensure most specific match wins
	sort.Slice(cfg.Services, func(i, j int) bool {
		return len(cfg.Services[i].BasePath) > len(cfg.Services[j].BasePath)
	})

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			path := req.URL.Path

			var fallbackSvc *the_oracle.ServiceConfig

			for _, svc := range cfg.Services {
				if svc.BasePath != "" && strings.HasPrefix(path, svc.BasePath) {
					isHijacked := false
					var hijackedPath string

					if svc.RerouteFlag {
						for _, r := range svc.HijackedRoutes {
							idx := strings.Index(path, r)
							if idx != -1 {
								isHijacked = true
								hijackedPath = path[idx:]
								break
							}
						}
					}

					if isHijacked {
						targetStr := svc.RerouteDestination
						if svc.StripPrefix {
							req.URL.Path = hijackedPath
						}

						targetURL, err := url.Parse(targetStr)
						if err != nil || targetURL.Scheme == "" {
							logChan <- LogEvent{
								Service: svc.Name,
								Method:  req.Method,
								Path:    path,
								Dest:    "INVALID CONFIG: " + targetStr,
								Local:   true,
								Status:  500,
								Time:    time.Now(),
							}
							return
						}

						req.URL.Scheme = targetURL.Scheme
						req.URL.Host = targetURL.Host
						req.Host = targetURL.Host

						req.Header.Set("X-Proxy-Service", svc.Name)
						req.Header.Set("X-Proxy-IsLocal", "true")
						req.Header.Set("X-Proxy-Dest", req.URL.String())
						return
					}

					if fallbackSvc == nil {
						s := svc
						fallbackSvc = &s
					}
				}
			}

			if fallbackSvc != nil {
				targetStr := fallbackSvc.RouteOrigin
				targetURL, err := url.Parse(targetStr)
				if err != nil || targetURL.Scheme == "" {
					logChan <- LogEvent{
						Service: fallbackSvc.Name,
						Method:  req.Method,
						Path:    path,
						Dest:    "INVALID CONFIG: " + targetStr,
						Local:   false,
						Status:  500,
						Time:    time.Now(),
					}
					return
				}

				req.URL.Scheme = targetURL.Scheme
				req.URL.Host = targetURL.Host
				req.Host = targetURL.Host

				req.Header.Set("X-Proxy-Service", fallbackSvc.Name)
				req.Header.Set("X-Proxy-IsLocal", "false")
				req.Header.Set("X-Proxy-Dest", req.URL.String())
				return
			}

			// If we got here, no service matched the prefix.
			logChan <- LogEvent{
				Service: "UNKNOWN",
				Method:  req.Method,
				Path:    path,
				Dest:    "NO MATCHING SERVICE",
				Local:   false,
				Status:  404,
				Time:    time.Now(),
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Wipe out all upstream CORS headers to avoid duplicates
			for k := range resp.Header {
				if strings.HasPrefix(strings.ToLower(k), "access-control-") {
					delete(resp.Header, k)
				}
			}

			// Force clean CORS headers to match the requester's origin
			if origin := resp.Request.Header.Get("Origin"); origin != "" {
				resp.Header.Set("Access-Control-Allow-Origin", origin)
				resp.Header.Set("Access-Control-Allow-Credentials", "true")
				resp.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, traceparent")
				resp.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}

			return nil
		},
		Transport: &interceptorTransport{
			roundTripper: http.DefaultTransport,
			logChan:      logChan,
		},
	}
	return proxy
}

type interceptorTransport struct {
	roundTripper http.RoundTripper
	logChan      chan LogEvent
}

func (i *interceptorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	svcName := req.Header.Get("X-Proxy-Service")
	if svcName == "" {
		return i.roundTripper.RoundTrip(req)
	}

	isLocal := req.Header.Get("X-Proxy-IsLocal") == "true"
	dest := req.Header.Get("X-Proxy-Dest")
	path := req.URL.Path
	method := req.Method

	req.Header.Del("X-Proxy-Service")
	req.Header.Del("X-Proxy-IsLocal")
	req.Header.Del("X-Proxy-Dest")

	resp, err := i.roundTripper.RoundTrip(req)

	status := 502
	if resp != nil {
		status = resp.StatusCode
	}

	i.logChan <- LogEvent{
		Service: svcName,
		Method:  method,
		Path:    path,
		Dest:    dest,
		Local:   isLocal,
		Status:  status,
		Time:    time.Now(),
	}

	return resp, err
}
