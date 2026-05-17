package the_conduit

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"

	"proxymaxxing/the_oracle"
)

type ConduitInfo struct {
	VPNProfile string
	RoutedIPs  []string
	Message    string
}

// Setup configures the split-tunnel VPN using NetworkManager based on the provided configuration.
// Returns a ConduitInfo struct describing the setup, or an error.
func Setup(cfg *the_oracle.Config) (ConduitInfo, error) {
	if cfg.VPNProfileName == "" {
		return ConduitInfo{Message: "Skipped: No VPN profile specified"}, nil
	}

	ipSet := make(map[string]bool)

	// Phase A: State Calculation
	// Extract Hostnames and Resolve IPs
	for _, svc := range cfg.Services {
		if svc.SwaggerURL != "" {
			u, err := url.Parse(svc.SwaggerURL)
			if err == nil {
				host := u.Hostname()
				ips, err := net.LookupHost(host)
				if err == nil {
					for _, ip := range ips {
						parsedIP := net.ParseIP(ip)
						if parsedIP != nil && parsedIP.To4() != nil {
							ipSet[ip] = true
						}
					}
				}
			}
		}
	}

	// Merge with infrastructure IPs or Hostnames
	for _, infra := range cfg.Infrastructure {
		parsedIP := net.ParseIP(infra.IP)
		if parsedIP != nil && parsedIP.To4() != nil {
			ipSet[infra.IP] = true
		} else {
			// It's not a valid IP, assume it's a hostname and try to resolve it
			ips, err := net.LookupHost(infra.IP)
			if err == nil {
				for _, ip := range ips {
					pIP := net.ParseIP(ip)
					if pIP != nil && pIP.To4() != nil {
						ipSet[ip] = true
					}
				}
			}
		}
	}

	if len(ipSet) == 0 {
		return ConduitInfo{Message: "Skipped: No valid IPv4 addresses to route"}, nil
	}

	var routeStrings []string
	var displayIPs []string
	for ip := range ipSet {
		routeStrings = append(routeStrings, fmt.Sprintf("%s/32", ip))
		displayIPs = append(displayIPs, ip)
	}

	formattedRouteString := strings.Join(routeStrings, ",")

	// Phase B: The nmcli Execution
	// 1. Prevent Global Hijacking
	cmd := exec.Command("nmcli", "connection", "modify", cfg.VPNProfileName, "ipv4.never-default", "yes")
	if err := cmd.Run(); err != nil {
		return ConduitInfo{}, fmt.Errorf("failed to set never-default: %w", err)
	}

	cmd = exec.Command("nmcli", "connection", "modify", cfg.VPNProfileName, "ipv4.ignore-auto-routes", "yes")
	if err := cmd.Run(); err != nil {
		return ConduitInfo{}, fmt.Errorf("failed to set ignore-auto-routes: %w", err)
	}

	// 2. Inject Specific Routes
	cmd = exec.Command("nmcli", "connection", "modify", cfg.VPNProfileName, "ipv4.routes", formattedRouteString)
	if err := cmd.Run(); err != nil {
		return ConduitInfo{}, fmt.Errorf("failed to inject routes: %w", err)
	}

	// 3. Apply Changes (Cycle Connection)
	// We need to bring it down first to ensure settings apply cleanly if it's already active
	exec.Command("nmcli", "connection", "down", cfg.VPNProfileName).Run()
	cmd = exec.Command("nmcli", "connection", "up", cfg.VPNProfileName)
	if err := cmd.Run(); err != nil {
		return ConduitInfo{}, fmt.Errorf("failed to cycle connection: %w", err)
	}

	msg := fmt.Sprintf("VPN %s activated with %d split-tunnel routes.", cfg.VPNProfileName, len(displayIPs))
	return ConduitInfo{VPNProfile: cfg.VPNProfileName, RoutedIPs: displayIPs, Message: msg}, nil
}

// Teardown cleans up the injected routes.
func Teardown(vpnProfileName string) error {
	if vpnProfileName == "" {
		return nil
	}

	// 1. Flush the specific proxymaxxing routes
	cmd := exec.Command("nmcli", "connection", "modify", vpnProfileName, "ipv4.routes", "")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush routes: %w", err)
	}

	cmd = exec.Command("nmcli", "connection", "modify", vpnProfileName, "ipv4.ignore-auto-routes", "no")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to revert ignore-auto-routes: %w", err)
	}

	cmd = exec.Command("nmcli", "connection", "modify", vpnProfileName, "ipv4.never-default", "no")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to revert never-default: %w", err)
	}

	// 2. Cycle the connection to apply the flush
	exec.Command("nmcli", "connection", "down", vpnProfileName).Run()
	cmd = exec.Command("nmcli", "connection", "up", vpnProfileName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to cycle connection: %w", err)
	}

	return nil
}
