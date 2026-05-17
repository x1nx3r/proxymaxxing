# proxymaxxing

You are a fullstack developer. You wrote a feature. It spans the frontend and one of the fourteen microservices that make up your backend architecture. You opened a pull request. It was approved. It was merged. It was deployed. 

It is broken. 

You investigate. The fix requires exactly one line of code in the backend microservice. You change the line locally. But to verify if the frontend will actually play nice with this fix, you have a choice. You can either spin up all fourteen microservices and their databases on your laptop until it melts, or you can commit the fix, open another pull request, beg for another review, wait forty-five minutes for the CI/CD pipeline, wait for the deployment gatekeeper to return from his coffee break, and finally test the frontend against the staging environment. 

You are effectively paid to sit in your chair and wait for pipelines to finish.

Welcome to proxymaxxing. It solves this problem. 

## What it does

It sits on your machine, binds to a port, and pretends to be your staging environment. 

You give it a list of Swagger JSON URLs. It scrapes them, reads their base paths, and generates a routing table. It also dynamically resolves those API hostnames and surgically injects them into your OS network stack as a split-tunnel VPN, forcing only the traffic you care about to pass through without destroying your ability to watch YouTube. If a request comes in for a route you are actively fixing, it strips the cloud prefix and throws it at the single local binary you are running. If a request comes in for anything else, it throws it over the fence to the actual staging environment. 

You no longer have to mock authentication. You no longer have to manually wrestle with OpenVPN configurations just to hit an internal database. You no longer have to run a convoluted docker-compose file that consumes all your RAM. You no longer have to wait for your own backend fix to be merged just so you can see if the frontend actually works. You just run this, spin up your one local backend binary alongside your local frontend, and pretend the cloud is local.

## Architecture

The codebase has been meticulously engineered into four highly logical (debatable, fight me) domains because putting everything in main.go was making us look bad.

### the_oracle
Executing first on boot, this domain reads your configuration file. If the file is incomplete, it goes out to the internet, scrapes the Swagger definitions via HTTP GET requests, traverses the OpenAPI schema to infer the namespace routing, and rewrites your configuration file with the correct answers to cache the route list. It knows what your API looks like better than you do. It handles the tedious work so you don't have to, ultimately passing the fully populated configuration struct to the rest of the application.

### the_conduit
Executing second, this domain automatically configures a split-tunnel VPN using Linux NetworkManager. It loops through the hostnames provided by `the_oracle`, performs DNS queries to resolve all their IPv4 addresses, and merges them with any explicit internal infrastructure IPs you need (like databases or Redis). It then issues precise `nmcli` commands to flip the target VPN profile into "ignore-auto-routes" mode, injects exactly those IPs with `/32` CIDR masks, and cycles the connection to forcefully limit the VPN's scope strictly to those endpoints. It also registers a signal listener in `main.go` to gracefully clean up after itself on exit so your OS networking isn't permanently hijacked.

### the_bouncer
Executing third and running continuously in a background goroutine, this is a custom reverse proxy bound to `0.0.0.0`. It intercepts every incoming HTTP request and checks the URL path against the hijacked route list. If the request matches a hijacked route, the bouncer mutates the request destination, rewrites the CORS headers so your frontend browser doesn't panic, strips the prefix, and forces it into your local port. If it doesn't match, the bouncer sends it to the cloud. It also pushes a telemetry record of every request into a Go channel for the UI, logging everything because trust is earned, not given.

### the_stage
Executing last and blocking the main thread, this is a 60fps terminal user interface built on the Bubble Tea framework. It consumes the configuration struct, the `the_conduit` status payload, and listens to the log channel emitted by `the_bouncer`. It runs in the foreground in alternate-screen mode so you look busy. You can use it to dynamically mutate the configuration struct in memory by toggling routing hijacks with the spacebar, or pressing 'i' to change the destination port, which implicitly alters how `the_bouncer` routes traffic in real-time. It writes those changes back to `config.yaml` and features three tabs, including a live request inspector so you can watch exactly how your frontend is malforming the payload, and a Conduit status tab to track your split-tunneling.

## Tech Stack

The application is written in Go, because nobody wants to write a reverse proxy in JavaScript. 

- **Go (Golang)**: The language of choice. It handles concurrency well, meaning it can proxy your failing requests very quickly.
- **net/http/httputil**: The standard library workhorse that acts as the actual reverse proxy.
- **Bubble Tea**: A framework for terminal applications. It makes the UI look significantly better than the underlying logic deserves.
- **kin-openapi**: A parser for OpenAPI 3. It parses the Swagger files that your backend team (which is you) automatically generated and never bothered to read.
- **yaml.v3**: Because JSON configuration files are for people who enjoy counting curly braces.

## Usage

Create a configuration file. The application will yell at you if you don't.

```yaml
port: 8080
vpn_profile_name: "Office-Staging-VPN" # Optional: Target NetworkManager connection

# Optional: Explicit internal network targets (Databases, Redis, etc.)
infrastructure:
  - name: "Staging Postgres"
    ip: "10.0.4.15"
  - name: "Internal Redis"
    ip: "redis.internal.company.com"

services:
  - swagger_url: "https://api-dev.company.com/service-a/swagger.json"
    reroute_destination: "http://localhost:8081"
  - swagger_url: "https://api-dev.company.com/service-b/swagger.json"
    reroute_destination: "http://localhost:8082"
```

Run the binary.

```bash
./proxymaxxing
```

The application will read your minimal effort, scrape the rest of the required data from the remote servers, rewrite your yaml file to be perfectly comprehensive, and then drop you into the terminal UI. 

Navigate with the arrow keys. Press space to hijack a route. Press tab to watch the requests fail in real time. 

Now fix your code. 

## License

This software is licensed under the WTFPL. Do whatever you want with it. We are not responsible for what happens when you accidentally proxy production traffic to your laptop.
