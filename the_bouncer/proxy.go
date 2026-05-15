package the_bouncer

import (
	"fmt"
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

			for _, svc := range cfg.Services {
				if svc.BasePath != "" && strings.HasPrefix(path, svc.BasePath) {
					remaining := strings.TrimPrefix(path, svc.BasePath)

					isHijacked := false
					if svc.RerouteFlag {
						for _, r := range svc.HijackedRoutes {
							if strings.HasPrefix(remaining, r) {
								isHijacked = true
								break
							}
						}
					}

					var targetStr string
					isLocal := false

					if isHijacked {
						targetStr = svc.RerouteDestination
						isLocal = true
						if svc.StripPrefix {
							req.URL.Path = remaining
						}
					} else {
						targetStr = svc.RouteOrigin
					}

					targetURL, err := url.Parse(targetStr)
					if err != nil || targetURL.Scheme == "" {
						// This is what caused your error. We need a scheme!
						logChan <- LogEvent{
							Service: svc.Name,
							Method:  req.Method,
							Path:    path,
							Dest:    "INVALID CONFIG: " + targetStr,
							Local:   isLocal,
							Status:  500,
							Time:    time.Now(),
						}
						return
					}

					req.URL.Scheme = targetURL.Scheme
					req.URL.Host = targetURL.Host
					req.Host = targetURL.Host

					req.Header.Set("X-Proxy-Service", svc.Name)
					req.Header.Set("X-Proxy-IsLocal", fmt.Sprintf("%v", isLocal))
					req.Header.Set("X-Proxy-Dest", req.URL.String())
					return
				}
			}

			// If we got here, no service matched the prefix.
			// Instead of letting it fail with "" scheme, we log it.
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
			fmt.Printf("ModifyResponse IN: %+v\n", resp.Header)
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
			fmt.Printf("ModifyResponse OUT: %+v\n", resp.Header)
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
