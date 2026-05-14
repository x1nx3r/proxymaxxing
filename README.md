# proxymaxxing

You are a fullstack developer. You wrote a feature. It spans the frontend and one of the fourteen microservices that make up your backend architecture. You opened a pull request. It was approved. It was merged. It was deployed. 

It is broken. 

You investigate. The fix requires exactly one line of code in the backend microservice. You change the line locally. But to verify if the frontend will actually play nice with this fix, you have a choice. You can either spin up all fourteen microservices and their databases on your laptop until it melts, or you can commit the fix, open another pull request, beg for another review, wait forty-five minutes for the CI/CD pipeline, wait for the deployment gatekeeper to return from his coffee break, and finally test the frontend against the staging environment. 

You are effectively paid to sit in your chair and wait for pipelines to finish.

Welcome to proxymaxxing. It solves this problem. 

## What it does

It sits on your machine, binds to a port, and pretends to be your staging environment. 

You give it a list of Swagger JSON URLs. It scrapes them, reads their base paths, and generates a routing table. If a request comes in for a route you are actively fixing, it strips the cloud prefix and throws it at the single local binary you are running. If a request comes in for anything else, it throws it over the fence to the actual staging environment. 

You no longer have to mock authentication. You no longer have to run a convoluted docker-compose file that consumes all your RAM. You no longer have to wait for your own backend fix to be merged just so you can see if the frontend actually works. You just run this, spin up your one local backend binary alongside your local frontend, and pretend the cloud is local.

## Architecture

The codebase has been meticulously engineered into three highly logical (debatable, fight me) domains because putting everything in main.go was making us look bad.

### the_oracle
Reads your configuration file. If the file is incomplete, it goes out to the internet, scrapes the Swagger definitions, infers the namespace routing, and overwrites your configuration file with the correct answers. It knows what your API looks like better than you do. It handles the tedious work so you don't have to.

### the_bouncer
A custom reverse proxy. It intercepts every HTTP request. If the request matches a hijacked route, the bouncer strips the prefix and forces it into your local port. If it doesn't, the bouncer sends it to the cloud. It also logs everything, because trust is earned, not given.

### the_stage
A 60fps terminal user interface. It runs in the foreground so you look busy. You can use it to toggle routing hijacks with the spacebar, or press 'i' to change the destination port because you forgot what port your own code binds to. It also has a live request inspector so you can watch exactly how your frontend is malforming the payload before it even hits the server.

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
