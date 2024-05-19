server = true
bootstrap_expect = 1
ui = true
datacenter = "dc1"
node_name = "consul-server"
client_addr = "0.0.0.0"
ports {
  http = 8500
  dns = 8600
}

enable_script_checks = true

domain = "local"

telemetry {
  prometheus_retention_time = "24h"
}

leave_on_terminate = true
