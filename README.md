# tick
Simple app that heartbeats itself with a registry

## Usage
Push and start app
```
cf push tick --no-start
cf set-env tick REGISTRY_ADDR "10.244.12.10"
cf start tick
```
