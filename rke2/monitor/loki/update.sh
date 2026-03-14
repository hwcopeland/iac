helm upgrade --install loki grafana/loki -f loki-values.yaml -n monitor
helm upgrade --install alloy grafana/alloy -f alloy-values.yaml -n monitor
