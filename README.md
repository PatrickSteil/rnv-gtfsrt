# rnv-gtfsrt

A lightweight service that converts occupancy data from the Rhein-Neckar-Verkehr (RNV) API into a GTFS-Realtime (GTFS-RT) feed.

## Overview

This project polls the RNV OpenData API for vehicle occupancy data and exposes it as a GTFS-RT protobuf stream. It is intended for use in transit applications that support GTFS-Realtime feeds.

## Features
- Fetches live occupancy data from RNV API
- Converts data into GTFS-Realtime format
- Exposes a simple HTTP endpoint
- Minimal dependencies and straightforward setup

## Requirements
- Go (1.20+ recommended)
- RNV API credentials:
  - OAuth URL
  - Client ID
  - Client Secret
  - Resource ID
  - API URL

## Setup
Set the environment variables:

```bash
export RNV_OAUTH_URL=<oauth-url>
export RNV_CLIENT_ID=<client-id>
export RNV_CLIENT_SECRET=<client-secret>
export RNV_RESOURCE_ID=<resource-id>
export RNV_API_URL=<api-url>
export RNV_POLL_INTERVAL=60 (optional)
export RNV_LISTEN_ADDR=8080 (optional)
```

Building the executables

```bash
make
```

and then running the server

```bash
./bin/server
```

Optional: enable debug logging


```bash
./bin/server --debug
```

## Output

The GTFS-Realtime feed will be available at:

```bash
http://localhost:8080/gtfs-rt
```

## Project Structure

```
cmd/
  server/     # Main service entrypoint
  inspect/    # Tool for inspecting GTFS-RT feeds

internal/
  config/     # Configuration loading
  poller/     # Polling logic
  rnvclient/  # API client
  gtfsrt/     # GTFS-RT encoding
  server/     # HTTP server
```

## Example

A sample GTFS-RT feed is available in:

```
example/feed.pb
example/feed.pretty
```
## Notes
- This project is not affiliated with RNV
- API access requires prior authorization
- Designed for experimentation
