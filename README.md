# Scraping data from an OPCUA endpoint
## Using a public OPCUA endpoint
The publicly avaialable OPCUA endpoints are listed here:     
https://github.com/node-opcua/node-opcua/wiki/publicly-available-OPC-UA-Servers-and-Clients  

## Monitoring a public OPCUA endpoint

## Creating an OTEL exporter - with out any metrics

Reference - https://uptrace.dev/opentelemetry/go-metrics.html#histogram
```bash
go get go.opentelemetry.io/otel/exporters/prometheus
go get github.com/gopcua/opcua/errors@v0.3.5
```
## Configure Promethues to scrape from the endpoint
Reference: https://medium.com/javarevisited/monitoring-setup-with-docker-compose-part-1-prometheus-3d2c9089ee82   

prometheus/prometheus.yml:
```yaml
global:
  scrape_interval: 30s
  scrape_timeout: 10s

rule_files:
  - alert.yml

scrape_configs:
  - job_name: services
    metrics_path: /metrics
    static_configs:
      - targets:
          - 'prometheus:9090'
          - 'idonotexists:564'
```


prometheus/alert.yml   
```yaml
groups:
  - name: DemoAlerts
    rules:
      - alert: InstanceDown 
        expr: up{job="services"} < 1 
        for: 5m
```

docker-compose.yaml
```yaml
version: '3'

services:
  prometheus:
    image: prom/prometheus:v2.30.3
    ports:
      - 9000:9090
    volumes:
      - ./prometheus:/etc/prometheus
      - prometheus-data:/prometheus
    command: --web.enable-lifecycle  --config.file=/etc/prometheus/prometheus.yml


volumes:
  prometheus-data:
```

Run 
```bash 
docker-compose up
```
## Pushing the metrics from the OPCUA endpoint to the exporter



----
## Starting the Simulator

```bash
docker pull mcr.microsoft.com/iotedge/opc-plc:2.5.0
docker run --rm -it -p 50000:50000 -p 8080:8080 --name opcplc mcr.microsoft.com/iotedge/opc-plc:latest --pn=50000 --autoaccept --ut --dca --sph --sn=5 --sr=10 --st=uint --fn=5 --fr=1 --ft=uint --ctb --scn --lid --lsn --ref --gn=5
```
