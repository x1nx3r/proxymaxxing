package the_bouncer

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"proxymaxxing/the_oracle"
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

					var targetURL *url.URL
					isLocal := false

					if isHijacked {
						targetURL, _ = url.Parse(svc.RerouteDestination)
						isLocal = true
						if svc.StripPrefix {
							req.URL.Path = remaining
						}
					} else {
						targetURL, _ = url.Parse(svc.RouteOrigin)
					}

					req.URL.Scheme = targetURL.Scheme
					req.URL.Host = targetURL.Host
					req.Host = targetURL.Host

					ctx := req.Context()
					req = req.WithContext(ctx)

					req.Header.Set("X-Proxy-Service", svc.Name)
					req.Header.Set("X-Proxy-IsLocal", fmt.Sprintf("%v", isLocal))
					req.Header.Set("X-Proxy-Dest", req.URL.String())
					return
				}
			}
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
